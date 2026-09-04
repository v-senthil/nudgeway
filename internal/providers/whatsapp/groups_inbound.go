package whatsapp

// Groups inbound helper — additive surface for the wire-up commit that
// threads a `group_id` field off Meta's inbound-message envelope through to
// canonical MessageReceived events.
//
// The main mapper (`mapper.go`) already extracts most of a Meta message
// today; extending it to surface group_id is a mechanical change but lives
// in a file that is on this task's deny list. This helper file declares the
// carrier type + extractor so a follow-up commit can plumb it without any
// coordination with the current author.
//
// Reference: ~/Documents/whatsapp_doc_tracker/docs/groups/groups-messaging.md
// section "Receive group messages".

// MessageReceivedGroupPayload is the small enrichment carried alongside a
// MessageReceived event when the source message came from a group. It is
// deliberately provider-agnostic — the group id is opaque here; the
// application layer resolves it to a domain.group.ID via GroupRepo before
// fanning out to real-time subscribers.
type MessageReceivedGroupPayload struct {
	// ProviderGroupID is Meta's opaque group id string from the inbound
	// message envelope's `group_id` field.
	ProviderGroupID string
}

// ExtractGroupID picks the `group_id` field off a raw Meta inbound-message
// map when present. It accepts the loose map[string]any shape mapper.go
// already builds during parseInboundMessage so the wire-up commit only has
// to call this at the right place — no re-parsing of the raw envelope.
//
// TODO(wire-up): thread the returned string onto MessageReceivedPayload as
// GroupProviderID, then extend mapper.go's parseInboundMessage to include
// the raw group_id in the map that ends up being passed here. The current
// mapper.go emits an events.Envelope.Payload of type
// events.MessageReceivedPayload; adding a GroupProviderID string field to
// that payload struct (in internal/domain/events) is the natural extension
// point. Two-commit sequence keeps churn bounded.
func ExtractGroupID(inbound map[string]any) string {
	if inbound == nil {
		return ""
	}
	v, ok := inbound["group_id"].(string)
	if !ok {
		return ""
	}
	return v
}
