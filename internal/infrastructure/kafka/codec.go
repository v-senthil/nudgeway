package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fullwa/fullwa/internal/domain/events"
	"github.com/fullwa/fullwa/internal/ports/queue"
)

// jobRecord is the wire form of queue.Job. Kept JSON to keep the round
// green today; protobuf can supersede once we have a proto schema for jobs.
type jobRecord struct {
	ID          string    `json:"id"`
	Lane        string    `json:"lane"`
	Payload     []byte    `json:"payload"`
	MaxAttempts int       `json:"max_attempts"`
	NotBefore   time.Time `json:"not_before"`
	Attempt     int       `json:"attempt"`
}

// envelopeRecord is the wire form of events.Envelope. Payload is round-
// tripped through json.RawMessage so the concrete payload type does not
// need to be visible to this package.
type envelopeRecord struct {
	Type           events.Type     `json:"type"`
	OrganizationID string          `json:"organization_id"`
	OccurredAt     time.Time       `json:"occurred_at"`
	CorrelationID  string          `json:"correlation_id"`
	CausationID    string          `json:"causation_id"`
	Payload        json.RawMessage `json:"payload"`
}

// EncodeJob serialises a queue.Job to bytes for Kafka.
func EncodeJob(j queue.Job) ([]byte, error) {
	b, err := json.Marshal(jobRecord{
		ID:          j.ID,
		Lane:        j.Lane,
		Payload:     j.Payload,
		MaxAttempts: j.MaxAttempts,
		NotBefore:   j.NotBefore,
		Attempt:     j.Attempt,
	})
	if err != nil {
		return nil, fmt.Errorf("kafka: encode job: %w", err)
	}
	return b, nil
}

// DecodeJob reads a queue.Job back off the wire.
func DecodeJob(b []byte) (queue.Job, error) {
	var r jobRecord
	if err := json.Unmarshal(b, &r); err != nil {
		return queue.Job{}, fmt.Errorf("kafka: decode job: %w", err)
	}
	return queue.Job{
		ID:          r.ID,
		Lane:        r.Lane,
		Payload:     r.Payload,
		MaxAttempts: r.MaxAttempts,
		NotBefore:   r.NotBefore,
		Attempt:     r.Attempt,
	}, nil
}

// EncodeEnvelope serialises an events.Envelope to bytes for Kafka.
func EncodeEnvelope(e events.Envelope) ([]byte, error) {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("kafka: encode envelope payload: %w", err)
	}
	b, err := json.Marshal(envelopeRecord{
		Type:           e.Type,
		OrganizationID: e.OrganizationID,
		OccurredAt:     e.OccurredAt,
		CorrelationID:  e.CorrelationID,
		CausationID:    e.CausationID,
		Payload:        payload,
	})
	if err != nil {
		return nil, fmt.Errorf("kafka: encode envelope: %w", err)
	}
	return b, nil
}

// DecodeEnvelope reads an events.Envelope back off the wire. Payload is
// left as json.RawMessage — callers decode into the concrete type keyed
// off Envelope.Type.
func DecodeEnvelope(b []byte) (events.Envelope, error) {
	var r envelopeRecord
	if err := json.Unmarshal(b, &r); err != nil {
		return events.Envelope{}, fmt.Errorf("kafka: decode envelope: %w", err)
	}
	return events.Envelope{
		Type:           r.Type,
		OrganizationID: r.OrganizationID,
		OccurredAt:     r.OccurredAt,
		CorrelationID:  r.CorrelationID,
		CausationID:    r.CausationID,
		Payload:        r.Payload,
	}, nil
}
