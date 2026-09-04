package hbase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/tsuna/gohbase"
	"github.com/tsuna/gohbase/hrpc"
)

// MaxAttachmentBytes caps a single blob at 16 MiB. Beyond this Meta
// requires the resumable upload API — surfaced as TODO in the summary.
const MaxAttachmentBytes int64 = 16 << 20

const (
	// cfData is the column family holding the raw blob bytes.
	cfData = "d"
	// cfMeta is the column family holding metadata (content type, size,
	// sha256, filename, per-integration provider media handles).
	cfMeta = "m"

	// colBytes is the column under cfData that stores the blob.
	colBytes = "bytes"

	// colContentType holds the client-declared or sniffed MIME type.
	colContentType = "content_type"
	// colSize holds the decimal byte count.
	colSize = "size"
	// colSHA256 holds the lowercase hex sha256 of the blob (== row key).
	colSHA256 = "sha256"
	// colFilename holds the original client filename when known.
	colFilename = "filename"
)

// Attachments implements the attachments.Store port against HBase.
//
// Row-key is the lowercase hex SHA-256 of the blob so writes are
// content-addressed: identical bytes always land on the same row.
// This makes uploads naturally deduplicated across tenants (which is a
// feature for popular templates and a non-issue for private blobs
// because operators can only reach a key they already know).
//
// Two column families:
//
//	d:bytes           — the raw blob
//	m:content_type    — MIME type (from client or sniffed)
//	m:size            — decimal byte count
//	m:sha256          — hex digest (redundant with the key, kept for
//	                    replication / cross-check)
//	m:filename        — original client filename (optional)
//	m:media_id_<providerKey>_<integrationID>
//	                  — opaque provider media handle returned by a Meta
//	                    Media Upload. One column per (provider, integ).
//
// Attachments is safe for concurrent use.
type Attachments struct {
	client gohbase.Client
	table  string
}

// NewAttachments builds an Attachments backed by client against the
// fully-qualified table name (e.g. "nudgeway:attachments").
func NewAttachments(client gohbase.Client, table string) *Attachments {
	return &Attachments{client: client, table: table}
}

// Put reads r fully into memory (bounded by MaxAttachmentBytes), computes
// SHA-256, and upserts a row keyed by the hex digest. Returns the digest,
// the byte count actually copied, and the digest again in the third
// tuple position for API parity with the localfs implementation.
//
// Caller MUST close r themselves.
func (a *Attachments) Put(ctx context.Context, contentType string, r io.Reader) (string, int64, string, error) {
	return a.PutWithMetadata(ctx, contentType, "", r)
}

// PutWithMetadata is Put plus a filename that is persisted under
// m:filename. Used by the /api/v1/attachments handler which knows the
// client-side name from the multipart FileHeader.
func (a *Attachments) PutWithMetadata(ctx context.Context, contentType, filename string, r io.Reader) (string, int64, string, error) {
	if a.client == nil {
		return "", 0, "", fmt.Errorf("hbase: Attachments.Put: nil client")
	}
	// Cap the read at MaxAttachmentBytes+1 so an oversized upload trips
	// the "too large" branch instead of silently truncating.
	limited := io.LimitReader(r, MaxAttachmentBytes+1)
	buf := &bytes.Buffer{}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(buf, h), limited)
	if err != nil {
		return "", 0, "", fmt.Errorf("hbase: read blob: %w", err)
	}
	if n > MaxAttachmentBytes {
		return "", 0, "", fmt.Errorf("hbase: blob exceeds %d bytes", MaxAttachmentBytes)
	}
	digest := hex.EncodeToString(h.Sum(nil))
	values := map[string]map[string][]byte{
		cfData: {
			colBytes: buf.Bytes(),
		},
		cfMeta: {
			colContentType: []byte(contentType),
			colSize:        []byte(strconv.FormatInt(n, 10)),
			colSHA256:      []byte(digest),
		},
	}
	if filename != "" {
		values[cfMeta][colFilename] = []byte(filename)
	}
	put, err := hrpc.NewPutStr(ctx, a.table, digest, values)
	if err != nil {
		return "", 0, "", fmt.Errorf("hbase: build put %s: %w", digest, err)
	}
	if _, err := a.client.Put(put); err != nil {
		return "", 0, "", fmt.Errorf("hbase: put row %s: %w", digest, err)
	}
	return digest, n, digest, nil
}

// Get returns an io.ReadCloser streaming the blob for key. Returns a
// wrapped os.ErrNotExist when the row is unknown so REST handlers can
// 404 without string-matching.
func (a *Attachments) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if a.client == nil {
		return nil, fmt.Errorf("hbase: Attachments.Get: nil client")
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}
	get, err := hrpc.NewGetStr(ctx, a.table, key,
		hrpc.Families(map[string][]string{cfData: {colBytes}}))
	if err != nil {
		return nil, fmt.Errorf("hbase: build get %s: %w", key, err)
	}
	res, err := a.client.Get(get)
	if err != nil {
		return nil, fmt.Errorf("hbase: get row %s: %w", key, err)
	}
	blob, ok := findCell(res, cfData, colBytes)
	if !ok {
		return nil, fmt.Errorf("hbase: blob %s: %w", key, os.ErrNotExist)
	}
	return io.NopCloser(bytes.NewReader(blob)), nil
}

// Delete removes the row from HBase. Missing rows are treated as
// success so Delete is idempotent.
func (a *Attachments) Delete(ctx context.Context, key string) error {
	if a.client == nil {
		return fmt.Errorf("hbase: Attachments.Delete: nil client")
	}
	if err := validateKey(key); err != nil {
		return err
	}
	del, err := hrpc.NewDelStr(ctx, a.table, key, nil)
	if err != nil {
		return fmt.Errorf("hbase: build delete %s: %w", key, err)
	}
	if _, err := a.client.Delete(del); err != nil {
		return fmt.Errorf("hbase: delete row %s: %w", key, err)
	}
	return nil
}

// ContentType reads m:content_type for key. Returns "" when the row
// exists but no MIME was recorded. Used by the REST /media handler to
// set the response Content-Type without re-sniffing. Name matches the
// contentTypeReader interface in internal/api/rest/v1/media.go.
func (a *Attachments) ContentType(ctx context.Context, key string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("hbase: Attachments.ContentType: nil client")
	}
	if err := validateKey(key); err != nil {
		return "", err
	}
	get, err := hrpc.NewGetStr(ctx, a.table, key,
		hrpc.Families(map[string][]string{cfMeta: {colContentType}}))
	if err != nil {
		return "", fmt.Errorf("hbase: build get %s: %w", key, err)
	}
	res, err := a.client.Get(get)
	if err != nil {
		return "", fmt.Errorf("hbase: get row %s: %w", key, err)
	}
	v, _ := findCell(res, cfMeta, colContentType)
	return string(v), nil
}

// SetMediaID persists the opaque provider handle returned by a Meta
// Media Upload for the given integration under
// m:media_id_<providerKey>_<integrationID>. Overwrite is fine — a fresh
// upload for the same blob against the same integration yields the same
// (or newer) handle.
func (a *Attachments) SetMediaID(ctx context.Context, key, providerKey, integrationID, mediaID string) error {
	if a.client == nil {
		return fmt.Errorf("hbase: Attachments.SetMediaID: nil client")
	}
	if err := validateKey(key); err != nil {
		return err
	}
	if providerKey == "" || integrationID == "" {
		return fmt.Errorf("hbase: SetMediaID: providerKey and integrationID required")
	}
	col := mediaIDColumn(providerKey, integrationID)
	values := map[string]map[string][]byte{
		cfMeta: {col: []byte(mediaID)},
	}
	put, err := hrpc.NewPutStr(ctx, a.table, key, values)
	if err != nil {
		return fmt.Errorf("hbase: build put media_id %s: %w", key, err)
	}
	if _, err := a.client.Put(put); err != nil {
		return fmt.Errorf("hbase: put media_id %s: %w", key, err)
	}
	return nil
}

// GetMediaID returns the cached provider media handle for (providerKey,
// integrationID) on the row, or ("", nil) when none has been recorded.
func (a *Attachments) GetMediaID(ctx context.Context, key, providerKey, integrationID string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("hbase: Attachments.GetMediaID: nil client")
	}
	if err := validateKey(key); err != nil {
		return "", err
	}
	col := mediaIDColumn(providerKey, integrationID)
	get, err := hrpc.NewGetStr(ctx, a.table, key,
		hrpc.Families(map[string][]string{cfMeta: {col}}))
	if err != nil {
		return "", fmt.Errorf("hbase: build get media_id %s: %w", key, err)
	}
	res, err := a.client.Get(get)
	if err != nil {
		return "", fmt.Errorf("hbase: get media_id %s: %w", key, err)
	}
	v, _ := findCell(res, cfMeta, col)
	return string(v), nil
}

// mediaIDColumn returns the metadata qualifier name that stores the
// media_id for a given (providerKey, integrationID). Kept in one place
// so writers and readers agree.
func mediaIDColumn(providerKey, integrationID string) string {
	return "media_id_" + providerKey + "_" + integrationID
}

// findCell returns the value cell for (family, qualifier) if present in
// res. Handles nil results defensively.
func findCell(res *hrpc.Result, family, qualifier string) ([]byte, bool) {
	if res == nil {
		return nil, false
	}
	for _, c := range res.Cells {
		if c == nil {
			continue
		}
		if string(c.Family) == family && string(c.Qualifier) == qualifier {
			return c.Value, true
		}
	}
	return nil, false
}

// validateKey guards Get/Delete against injected keys. The store only
// accepts 64-char lowercase hex digests (a SHA-256 output).
func validateKey(key string) error {
	if len(key) != 64 {
		return fmt.Errorf("hbase: invalid key length %d", len(key))
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("hbase: invalid key character at %d", i)
		}
	}
	return nil
}

// ErrNotFound is a sentinel returned by callers that want a stable
// "no such row" signal without importing os.ErrNotExist. Kept exported
// so packages that don't want to depend on os can errors.Is on it.
var ErrNotFound = errors.New("hbase: row not found")
