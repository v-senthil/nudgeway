package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params configures argon2id hashing.
//
// Defaults follow the OWASP 2023 password-storage guidance for a memory-bound
// hash on a modern web server. Tune per deployment and record in an ADR.
type Argon2Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltBytes   uint32
	KeyBytes    uint32
}

// DefaultArgon2Params returns the Nudgeway defaults.
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		MemoryKiB:   64 * 1024,
		Iterations:  3,
		Parallelism: 2,
		SaltBytes:   16,
		KeyBytes:    32,
	}
}

// HashPassword returns an encoded argon2id hash of pw using p.
//
// Format: $argon2id$v=19$m=<mem>,t=<iters>,p=<par>$<b64salt>$<b64key>.
// This is the standard PHC-compatible encoding.
func HashPassword(pw string, p Argon2Params) (string, error) {
	if pw == "" {
		return "", errors.New("empty password")
	}
	salt := make([]byte, p.SaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, p.Iterations, p.MemoryKiB, p.Parallelism, p.KeyBytes)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword returns true when pw hashes to encoded.
//
// Timing is constant with respect to the correct key. Any parse failure
// returns (false, error) — callers should log the error and treat it as an
// invalid credential.
func VerifyPassword(pw, encoded string) (bool, error) {
	p, salt, want, err := parseEncoded(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(pw), salt, p.Iterations, p.MemoryKiB, p.Parallelism, p.KeyBytes)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

//nolint:funlen // linear parser, easier to audit as one function.
func parseEncoded(s string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(s, "$")
	// parts[0]="" parts[1]="argon2id" parts[2]="v=19" parts[3]="m=..,t=..,p=.." parts[4]=salt parts[5]=key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, errors.New("bad argon2id encoding")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("bad version: %w", err)
	}
	if version != argon2.Version {
		return Argon2Params{}, nil, nil, fmt.Errorf("unsupported argon2 version: %d", version)
	}
	var mem, iters uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iters, &par); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("bad params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("bad salt: %w", err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("bad key: %w", err)
	}
	return Argon2Params{
		MemoryKiB: mem, Iterations: iters, Parallelism: par,
		SaltBytes: uint32(len(salt)), KeyBytes: uint32(len(key)),
	}, salt, key, nil
}
