// Package openapi embeds the OpenAPI 3.1 specification as a byte slice so
// other tools (notably cmd/mcp) can generate protocol tools from it without
// re-reading the file from disk at runtime.
package openapi

import _ "embed"

//go:embed openapi.yaml
var specYAML []byte

// SpecYAML returns the raw bytes of internal/api/openapi/openapi.yaml as
// embedded at build time.
func SpecYAML() []byte {
	// Return a defensive copy so callers cannot mutate the embedded bytes.
	out := make([]byte, len(specYAML))
	copy(out, specYAML)
	return out
}
