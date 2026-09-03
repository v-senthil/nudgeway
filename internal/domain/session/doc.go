// Package session models the communication relationship between one
// business endpoint (e.g. a WhatsApp Business phone number) and one
// end-user identity. A Contact may hold multiple Sessions across different
// business endpoints; at most one ACTIVE Session exists per
// (org, business_endpoint, contact).
package session
