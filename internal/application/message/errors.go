package message

import "errors"

// ErrIntegrationNotFound is returned when the inbound webhook references an
// integration_id that does not exist (or was deleted). This is treated as a
// permanent failure — retrying will never succeed.
var ErrIntegrationNotFound = errors.New("inbound: integration not found")

// ErrProviderNotRegistered is returned when the integration's provider key is
// not present in the runtime channel-provider registry. Permanent failure.
var ErrProviderNotRegistered = errors.New("inbound: provider not registered")

// ErrEndpointNotProvisioned is returned when the webhook targets a
// business_endpoint that has not been created for this org yet. Permanent
// for this envelope (skipped and logged); the caller must provision the
// endpoint before Meta can deliver messages for it.
var ErrEndpointNotProvisioned = errors.New("inbound: business endpoint not provisioned")

// ErrUnknownEnvelope is returned when the parser emits an event type this
// service does not know how to handle. Permanent.
var ErrUnknownEnvelope = errors.New("inbound: unknown envelope type")

// permanent wraps another error and marks it non-retryable. Workers unwrap
// with IsPermanent to decide whether to swallow or requeue.
type permanent struct{ err error }

// Error implements error.
func (p permanent) Error() string { return p.err.Error() }

// Unwrap exposes the wrapped cause.
func (p permanent) Unwrap() error { return p.err }

// Permanent marks err as a non-retryable failure. The worker will log,
// mark the webhook_event failed, and ACK the job so the queue does not
// redeliver it forever.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanent{err: err}
}

// IsPermanent reports whether err (or anything it wraps) has been tagged
// with Permanent.
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	var p permanent
	return errors.As(err, &p)
}

// IsDuplicateMessage reports whether err represents a UNIQUE(org,
// provider_message_id) collision on messages. Implementations of MessageRepo
// return an error that the InboundService can classify via this predicate;
// duplicates are absorbed as success by the caller. We match by string to
// avoid taking a hard dependency on the mysql package from the application
// layer.
func IsDuplicateMessage(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, needle := range duplicateNeedles {
		if contains(msg, needle) {
			return true
		}
	}
	return false
}

// duplicateNeedles lists substrings that MySQL / infrastructure surfaces for
// duplicate-key errors. Kept broad so we absorb both raw driver errors and
// wrapped ones without importing the driver here.
var duplicateNeedles = []string{
	"Duplicate entry",
	"duplicate key",
	"1062",
}

// contains is a tiny substring check — avoids importing strings just for
// error classification and keeps this file dependency-free.
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
