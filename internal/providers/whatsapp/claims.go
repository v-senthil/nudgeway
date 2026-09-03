package whatsapp

import (
	"encoding/json"
	"fmt"
)

// VerifyClaims parses a Meta webhook body and asserts that the
// entry[].id (WABA id) and every changes[].value.metadata.phone_number_id
// value match the integration's configured `waba_id` and
// `phone_number_id`. Returns nil on match.
//
// Used as a dev-only fallback when HMAC verification is disabled — see
// webhook.Ingress.RequireSignature. Production must keep the HMAC path.
func VerifyClaims(body []byte, config map[string]any) error {
	wantWABA, _ := config["waba_id"].(string)
	wantPhone, _ := config["phone_number_id"].(string)
	if wantWABA == "" && wantPhone == "" {
		return fmt.Errorf("whatsapp claims: integration config missing waba_id + phone_number_id")
	}

	var env struct {
		Object string `json:"object"`
		Entry  []struct {
			ID      string `json:"id"`
			Changes []struct {
				Value struct {
					Metadata struct {
						PhoneNumberID string `json:"phone_number_id"`
					} `json:"metadata"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("whatsapp claims: parse body: %w", err)
	}
	if env.Object != "whatsapp_business_account" {
		return fmt.Errorf("whatsapp claims: unexpected object %q", env.Object)
	}
	if len(env.Entry) == 0 {
		return fmt.Errorf("whatsapp claims: no entry in body")
	}
	for _, e := range env.Entry {
		if wantWABA != "" && e.ID != wantWABA {
			return fmt.Errorf("whatsapp claims: waba mismatch (got %s, want %s)", e.ID, wantWABA)
		}
		for _, c := range e.Changes {
			pn := c.Value.Metadata.PhoneNumberID
			if wantPhone != "" && pn != "" && pn != wantPhone {
				return fmt.Errorf("whatsapp claims: phone_number_id mismatch (got %s, want %s)", pn, wantPhone)
			}
		}
	}
	return nil
}
