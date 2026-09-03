package crypto

import (
	"encoding/hex"
	"fmt"
)

// ParseKEKHex decodes a 64-character hex string into a 32-byte KEK. Used
// by the config loader to turn `auth.credential_kek_hex` into raw bytes.
func ParseKEKHex(s string) ([]byte, error) {
	if len(s) != 64 {
		return nil, fmt.Errorf("crypto: KEK hex must be 64 chars, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("crypto: KEK hex decode: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("crypto: KEK decoded to %d bytes, want 32", len(b))
	}
	return b, nil
}
