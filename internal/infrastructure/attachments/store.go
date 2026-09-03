// Package attachments provides concrete implementations of the
// attachments.Store port. The dev-time implementation is the local
// filesystem store defined in localfs.go; production deployments swap it
// for HBase / S3 without touching the application layer.
package attachments

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config configures the local filesystem attachment store. It is the only
// tunable a Phase 1 dev deployment needs — content is content-addressed by
// SHA-256 so no bucket/prefix knobs are surfaced yet.
type Config struct {
	// Root is the directory under which attachment blobs are written. It
	// is created (with parents) on New if missing. Relative paths are
	// resolved against the process working directory.
	Root string
}

// New constructs a *LocalFS with the given config, ensuring the root
// directory exists. Returns an error when Root is empty or the mkdir fails.
func New(cfg Config) (*LocalFS, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("attachments: Root is required")
	}
	abs, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("attachments: resolve root %q: %w", cfg.Root, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("attachments: mkdir root %q: %w", abs, err)
	}
	return &LocalFS{root: abs}, nil
}
