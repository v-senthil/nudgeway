// Package attachments is the port for media/attachment storage.
// Implemented on HBase (content-addressed by SHA-256).
package attachments

import (
	"context"
	"io"
)

// Store persists media blobs by content hash and returns a storage key
// downstream code can use to fetch them again.
type Store interface {
	Put(ctx context.Context, contentType string, r io.Reader) (key string, size int64, sha256 string, err error)
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
