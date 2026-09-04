package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/v-senthil/nudgeway/internal/domain/events"
	"github.com/v-senthil/nudgeway/internal/ports/queue"
)

// tuning constants shared across producer + consumer.
const (
	producerLinger     = 5 * time.Millisecond
	maxBufferedRecords = 10_000
	pollTimeout        = 500 * time.Millisecond
)

// Producer implements queue.Enqueuer and eventbus.Publisher on top of a
// franz-go client. It performs synchronous produces so callers see broker
// errors on the return path — idempotency is enabled at the client so
// duplicate deliveries under retry never yield duplicate log entries.
type Producer struct {
	client *kgo.Client
	prefix string
}

// NewProducer wraps an existing franz-go client. Topic names are namespaced
// by prefix (see TopicName).
func NewProducer(client *kgo.Client, prefix string) *Producer {
	return &Producer{client: client, prefix: prefix}
}

// Enqueue publishes a job to <prefix>.jobs.<lane>. Partition key is
// j.ID so all jobs for the same conversation (whose Job.ID is derived
// from conversation_id upstream) hit the same partition and are consumed
// in order.
func (p *Producer) Enqueue(ctx context.Context, j queue.Job) (string, error) {
	if j.Lane == "" {
		return "", fmt.Errorf("kafka: enqueue: lane is required")
	}
	if j.ID == "" {
		// Callers that don't care about partition affinity (webhook ingress —
		// each delivery is independent) leave ID empty. Mint one so the
		// record still has a stable partition key.
		j.ID = ulid.Make().String()
	}
	body, err := EncodeJob(j)
	if err != nil {
		return "", err
	}
	rec := &kgo.Record{
		Topic: TopicName(p.prefix, KindJob, j.Lane),
		Key:   []byte(j.ID),
		Value: body,
	}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return "", fmt.Errorf("kafka: produce job: %w", err)
	}
	return j.ID, nil
}

// Publish sends a canonical event to <prefix>.events.<type>. Partition key
// is Envelope.CorrelationID (which upstream sets to conversation_id for
// message-flow events) so per-conversation ordering is preserved.
//
// Events without a CorrelationID fall back to a partitioner-picked
// partition — acceptable for cross-cutting events (integration.*,
// template.*) where ordering is not per-conversation.
func (p *Producer) Publish(ctx context.Context, evt events.Envelope) error {
	if evt.Type == "" {
		return fmt.Errorf("kafka: publish: event type is required")
	}
	body, err := EncodeEnvelope(evt)
	if err != nil {
		return err
	}
	rec := &kgo.Record{
		Topic: TopicName(p.prefix, KindEvent, string(evt.Type)),
		Value: body,
	}
	if evt.CorrelationID != "" {
		rec.Key = []byte(evt.CorrelationID)
	}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("kafka: produce event: %w", err)
	}
	return nil
}
