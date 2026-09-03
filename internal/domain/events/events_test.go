package events

import (
	"testing"
	"time"
)

func TestType_String(t *testing.T) {
	t.Parallel()
	if string(MessageReceived) != "message.received" {
		t.Errorf("MessageReceived = %q", MessageReceived)
	}
	if string(TicketCreated) != "ticket.created" {
		t.Errorf("TicketCreated = %q", TicketCreated)
	}
}

func TestEnvelope_FieldsRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	e := Envelope{
		Type:           MessageReceived,
		OrganizationID: "org_1",
		OccurredAt:     now,
		CorrelationID:  "corr_1",
		CausationID:    "cause_1",
		Payload:        map[string]any{"body": "hi"},
	}
	if e.Type != MessageReceived {
		t.Errorf("Type mismatch")
	}
	if e.OccurredAt != now {
		t.Errorf("OccurredAt mismatch")
	}
	if e.OrganizationID != "org_1" {
		t.Errorf("OrganizationID mismatch")
	}
}
