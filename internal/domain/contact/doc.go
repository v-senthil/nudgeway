// Package contact models the customer identity. A Contact may carry multiple
// ContactIdentity rows (phone, email, WhatsApp, BSUID, external CRM ID). The
// merge key is (org_id, provider, normalized_value).
package contact
