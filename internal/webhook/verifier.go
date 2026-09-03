package webhook

import (
	"errors"
	"net/http"
)

// SignatureVerifier is implemented by any provider adapter that can verify
// the authenticity of a raw webhook body against a provider-specific shared
// secret. Verification MUST be constant-time and reject on any missing
// header, malformed signature, or mismatch.
//
// This interface is the sole surface the generic ingress layer uses to
// authenticate deliveries — nothing about Meta / Twilio / Zoho vocabulary
// leaks past it.
type SignatureVerifier interface {
	// VerifySignature validates the request-level signature carried in
	// headers against body using appSecret. Returns nil on success and a
	// non-nil error (which the ingress translates into a 401) otherwise.
	VerifySignature(headers http.Header, body []byte, appSecret string) error
}

// SignatureVerifierFunc adapts a bare function to the SignatureVerifier
// interface. cmd/server uses this to wrap free functions exported by
// provider adapters (e.g. whatsapp.VerifySignature) without those adapters
// having to know about this package.
type SignatureVerifierFunc func(headers http.Header, body []byte, appSecret string) error

// VerifySignature implements SignatureVerifier.
func (f SignatureVerifierFunc) VerifySignature(headers http.Header, body []byte, appSecret string) error {
	return f(headers, body, appSecret)
}

// ErrNoVerifier is returned by VerifierLookup when the caller asked for a
// provider whose adapter has no signature verifier registered. The ingress
// converts this into a 401 rather than a 500 because it means the caller
// tried to speak to a provider we cannot authenticate.
var ErrNoVerifier = errors.New("webhook: no signature verifier registered for provider")

// VerifierLookup returns the SignatureVerifier for the given provider key,
// or a non-nil error (typically ErrNoVerifier) if no adapter is registered.
//
// A VerifierLookup is provided at wire-up time so the ingress does not need
// to import any provider package directly.
type VerifierLookup func(providerKey string) (SignatureVerifier, error)

// StaticVerifierLookup builds a VerifierLookup from an in-memory map. It is
// the simplest form of registry — cmd/server wires one of these using the
// adapters it has imported. Adapters supply the mapping via their
// registration helpers (e.g. whatsapp.Verifier()).
func StaticVerifierLookup(m map[string]SignatureVerifier) VerifierLookup {
	// Copy to insulate the returned closure from later mutation.
	cp := make(map[string]SignatureVerifier, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return func(providerKey string) (SignatureVerifier, error) {
		v, ok := cp[providerKey]
		if !ok {
			return nil, ErrNoVerifier
		}
		return v, nil
	}
}
