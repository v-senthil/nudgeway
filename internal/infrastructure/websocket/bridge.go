package websocket

import (
	"encoding/json"
	"log/slog"

	devents "github.com/fullwa/fullwa/internal/domain/events"
	"github.com/fullwa/fullwa/internal/ports/eventbus"
)

// bridgedTypes is the list of canonical event types the WebSocket bridge
// mirrors from the in-proc bus onto the org room. Keeping the list explicit
// (rather than subscribing to everything) means we never accidentally leak
// events that shouldn't reach the browser.
var bridgedTypes = []devents.Type{
	devents.MessageReceived,
	devents.MessageSent,
	devents.MessageDelivered,
	devents.MessageRead,
	devents.MessageFailed,
	devents.ConversationCreated,
	devents.ConversationUpdated,
	devents.ConversationAssigned,
	devents.ConversationResolved,
	devents.CallInitiated,
	devents.CallRinging,
	devents.CallAnswered,
	devents.CallEnded,
	devents.CallEndedDetailed,
	devents.CallFailed,
	devents.CallRecordingCreated,
}

// wireFrame is the JSON envelope shipped to browsers. It intentionally
// mirrors the canonical event envelope's shape without including internal
// correlation metadata that has no meaning to the UI.
type wireFrame struct {
	Type          string `json:"type"`
	OrgID         string `json:"org_id"`
	OccurredAt    string `json:"occurred_at,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Payload       any    `json:"payload,omitempty"`
}

// RegisterEventBridge wires the in-proc event bus into the WebSocket hub.
// For every bridged event type, the handler serialises the envelope into a
// wireFrame and calls hub.Broadcast on the event's OrganizationID.
//
// Handlers registered here return nil unconditionally: the bus should not
// treat a client-side serialisation failure as a domain error.
func RegisterEventBridge(bus eventbus.Subscriber, hub *Hub, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	for _, t := range bridgedTypes {
		t := t // capture
		bus.Subscribe(t, func(evt devents.Envelope) error {
			frame := wireFrame{
				Type:          string(evt.Type),
				OrgID:         evt.OrganizationID,
				CorrelationID: evt.CorrelationID,
				Payload:       evt.Payload,
			}
			if !evt.OccurredAt.IsZero() {
				frame.OccurredAt = evt.OccurredAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")
			}
			payload, err := json.Marshal(frame)
			if err != nil {
				logger.Warn("ws bridge: marshal event",
					slog.String("type", string(evt.Type)),
					slog.String("org_id", evt.OrganizationID),
					slog.Any("err", err),
				)
				return nil
			}
			hub.Broadcast(evt.OrganizationID, payload)
			return nil
		})
	}
}
