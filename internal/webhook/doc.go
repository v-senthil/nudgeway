// Package webhook is the provider-agnostic webhook ingress. Every provider
// mounts its endpoint under /webhooks/<provider>/<integration_id>; this
// package validates signatures via the adapter, persists the raw event,
// ACKs quickly, and enqueues for async processing.
package webhook
