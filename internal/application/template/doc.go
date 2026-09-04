// Package template contains the tenant-facing template management
// use-cases: create, submit-for-review, sync, list, get, delete.
//
// The service never imports a provider package directly. Provider-specific
// calls go through the TemplateProvider port injected by cmd/server, which
// closes over the concrete adapter (e.g. whatsapp.Provider).
package template
