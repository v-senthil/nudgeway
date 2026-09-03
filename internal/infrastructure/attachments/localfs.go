package attachments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalFS implements the attachments.Store port on top of the local
// filesystem. It is content-addressed by SHA-256: the storage key IS the
// hex-encoded digest of the bytes. Content is sharded across two levels
// (2 hex chars each) so a single directory never accumulates thousands of
// siblings.
//
// Layout under Root:
//
//	<root>/aa/bb/aabbcc…                — blob bytes
//	<root>/aa/bb/aabbcc….contenttype    — one-line UTF-8 MIME string
//
// Writes are atomic: bytes stream to a temp file first, then rename onto
// the final path once the digest is known. The sidecar is written after
// the blob so partial failures never leave a content-type without bytes.
//
// LocalFS is safe for concurrent Put/Get/Delete.
type LocalFS struct {
	root string
}

// Put streams r to disk while computing its SHA-256, then renames the temp
// file onto <root>/<aa>/<bb>/<digest>. The returned key is the lowercase
// hex digest and the returned size is the number of bytes copied. The
// content type is persisted alongside as a `.contenttype` sidecar so Get
// can surface it without re-sniffing.
//
// Callers MUST close r themselves.
func (l *LocalFS) Put(ctx context.Context, contentType string, r io.Reader) (key string, size int64, digest string, err error) {
	// Buffer to a temp file in the root so the eventual rename is same-fs.
	tmp, err := os.CreateTemp(l.root, ".upload-*")
	if err != nil {
		return "", 0, "", fmt.Errorf("attachments: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), ctxReader{ctx: ctx, r: r})
	if closeErr := tmp.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return "", 0, "", fmt.Errorf("attachments: buffer to temp: %w", copyErr)
	}

	digest = hex.EncodeToString(h.Sum(nil))
	blobPath := l.pathFor(digest)
	if err = os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		return "", 0, "", fmt.Errorf("attachments: mkdir shard: %w", err)
	}

	// Rename onto final path. If a blob with the same digest already
	// exists we honour it (content-addressed store: identical bytes ⇒
	// identical file) and drop the temp copy.
	if _, statErr := os.Stat(blobPath); statErr == nil {
		cleanup()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", 0, "", fmt.Errorf("attachments: stat blob: %w", statErr)
	} else {
		if err = os.Rename(tmpPath, blobPath); err != nil {
			return "", 0, "", fmt.Errorf("attachments: rename blob: %w", err)
		}
	}

	// Best-effort content-type sidecar. Overwrite is fine — same digest
	// with a different reported type is a caller bug, but honouring the
	// latest write matches the "last writer wins" content-addressed rule.
	if contentType != "" {
		if err = os.WriteFile(blobPath+".contenttype", []byte(contentType), 0o644); err != nil {
			return "", 0, "", fmt.Errorf("attachments: write content-type sidecar: %w", err)
		}
	}
	return digest, n, digest, nil
}

// Get returns an io.ReadCloser streaming the bytes for key. Callers MUST
// close it. Returns os.ErrNotExist (wrapped) when the key is unknown so
// handlers can 404 without string-matching.
func (l *LocalFS) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	_ = ctx // filesystem reads are non-blocking; ctx kept for interface parity.
	if err := validateKey(key); err != nil {
		return nil, err
	}
	f, err := os.Open(l.pathFor(key))
	if err != nil {
		return nil, fmt.Errorf("attachments: open blob %s: %w", key, err)
	}
	return f, nil
}

// Delete removes the blob and its content-type sidecar. Missing files are
// treated as success so Delete is idempotent.
func (l *LocalFS) Delete(ctx context.Context, key string) error {
	_ = ctx
	if err := validateKey(key); err != nil {
		return err
	}
	p := l.pathFor(key)
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("attachments: remove blob %s: %w", key, err)
	}
	if err := os.Remove(p + ".contenttype"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("attachments: remove sidecar %s: %w", key, err)
	}
	return nil
}

// ContentType returns the stored MIME type for key or "" when no sidecar
// was recorded. Filesystem errors other than not-exist are returned as-is.
// It is exposed alongside Get so REST handlers can set the response
// Content-Type without re-sniffing bytes.
func (l *LocalFS) ContentType(ctx context.Context, key string) (string, error) {
	_ = ctx
	if err := validateKey(key); err != nil {
		return "", err
	}
	b, err := os.ReadFile(l.pathFor(key) + ".contenttype")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("attachments: read content-type sidecar: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// pathFor returns the on-disk path for a given key. Sharded 2/2 so
// millions of blobs distribute evenly.
func (l *LocalFS) pathFor(key string) string {
	// key is validated to be a 64-char lowercase hex string, so [0:2] and
	// [2:4] are always safe.
	return filepath.Join(l.root, key[0:2], key[2:4], key)
}

// validateKey guards Get/Delete against path traversal. The store only
// accepts 64-char lowercase hex digests (a SHA-256 output).
func validateKey(key string) error {
	if len(key) != 64 {
		return fmt.Errorf("attachments: invalid key length %d", len(key))
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("attachments: invalid key character at %d", i)
		}
	}
	return nil
}

// ctxReader wraps an io.Reader with an early-exit on ctx cancellation so a
// stuck upstream (Meta media host) cannot block Put indefinitely.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

// Read forwards to the wrapped reader after checking ctx.Err().
func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
