// Package integrationsettings is the application-layer entry point for
// the operator UI's "integration settings" drawer.
//
// It exposes provider-agnostic methods (business profile, calling
// settings, official-business-account status) that resolve an
// integration + its decrypted secrets and dispatch through a local
// ProviderClient interface. The concrete adapter is wired by cmd/server
// so this package NEVER imports internal/providers/whatsapp.
package integrationsettings
