// Package crypto contains envelope-encryption helpers for tenant secrets.
//
// The key hierarchy is intentionally simple for Phase 1: a single 32-byte
// KEK ("Key Encryption Key") is loaded from configuration and used to
// AES-256-GCM every integration_credentials.ciphertext blob directly.
// A later phase introduces per-integration DEKs and KMS-backed rotation
// without changing the on-disk framing.
//
// Ciphertext framing:
//
//	byte 0        : version (currently 1)
//	bytes 1..12   : 12-byte GCM nonce
//	bytes 13..end : AES-GCM ciphertext with the 16-byte auth tag appended
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// envelopeVersion is the on-disk framing version.
const envelopeVersion byte = 1

// gcmNonceSize is the standard 96-bit GCM nonce.
const gcmNonceSize = 12

// Envelope is a stateless helper for AES-GCM envelope encryption with a
// fixed 32-byte KEK. It is safe for concurrent use.
type Envelope struct {
	// KEK is the 32-byte key encryption key. Callers should populate this
	// from a source appropriate to the deployment (config file, KMS, etc.).
	KEK []byte
}

// ErrKEKInvalid is returned when the KEK is not 32 bytes.
var ErrKEKInvalid = errors.New("crypto: KEK must be 32 bytes")

// ErrCiphertextTooShort is returned when a ciphertext blob cannot possibly
// contain a valid version+nonce header.
var ErrCiphertextTooShort = errors.New("crypto: ciphertext too short")

// ErrUnknownVersion is returned when the framing version byte is not one
// the current build understands.
var ErrUnknownVersion = errors.New("crypto: unknown ciphertext version")

// NewEnvelope constructs an Envelope with the given KEK. The KEK must be
// exactly 32 bytes.
func NewEnvelope(kek []byte) (*Envelope, error) {
	if len(kek) != 32 {
		return nil, ErrKEKInvalid
	}
	dup := make([]byte, 32)
	copy(dup, kek)
	return &Envelope{KEK: dup}, nil
}

// Encrypt seals plaintext under the KEK using AES-256-GCM and returns the
// framed blob described in the package doc. The nonce is drawn from
// crypto/rand.
func (e *Envelope) Encrypt(plaintext []byte) ([]byte, error) {
	if len(e.KEK) != 32 {
		return nil, ErrKEKInvalid
	}
	block, err := aes.NewCipher(e.KEK)
	if err != nil {
		return nil, fmt.Errorf("crypto envelope new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto envelope new gcm: %w", err)
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto envelope nonce: %w", err)
	}
	ct := aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, 1+gcmNonceSize+len(ct))
	out = append(out, envelopeVersion)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// Decrypt opens a framed blob and returns the plaintext. Rejects unknown
// framing versions and truncated inputs.
func (e *Envelope) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(e.KEK) != 32 {
		return nil, ErrKEKInvalid
	}
	if len(ciphertext) < 1+gcmNonceSize+16 {
		return nil, ErrCiphertextTooShort
	}
	if ciphertext[0] != envelopeVersion {
		return nil, ErrUnknownVersion
	}
	nonce := ciphertext[1 : 1+gcmNonceSize]
	ct := ciphertext[1+gcmNonceSize:]
	block, err := aes.NewCipher(e.KEK)
	if err != nil {
		return nil, fmt.Errorf("crypto envelope new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto envelope new gcm: %w", err)
	}
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto envelope open: %w", err)
	}
	return pt, nil
}
