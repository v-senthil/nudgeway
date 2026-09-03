// Package kafka wires fullWA's durable event log and job queue on top of
// Apache Kafka via the franz-go client. It implements the queue and eventbus
// ports so callers stay unaware of the underlying broker.
//
// Topic layout is namespaced by cfg.TopicsPrefix:
//   - <prefix>.jobs.<lane>     — background job queues
//   - <prefix>.events.<type>   — canonical domain events
//
// Ordering: partition key is derived from conversation_id (via Job.ID for
// jobs, Envelope.CorrelationID for events) so all traffic for one
// conversation lands on the same partition and is processed in order.
package kafka

import (
	"fmt"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/fullwa/fullwa/internal/infrastructure/config"
)

// NewClient builds a producer-capable franz-go client with sensible defaults
// for the fullWA workload: idempotent producer, snappy compression, small
// batches with a short linger so end-to-end latency stays low.
//
// The returned client is safe for concurrent use. Callers own the lifecycle
// and must call Close when done.
func NewClient(cfg config.KafkaConfig) (*kgo.Client, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka: at least one broker required")
	}
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ClientID(clientID(cfg)),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
		kgo.ProducerLinger(producerLinger),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		// franz-go enables idempotency by default when acks=all and no
		// non-idempotent options are set; making the intent explicit here
		// via DisableIdempotentWrite would be wrong.
		kgo.MaxBufferedRecords(maxBufferedRecords),
	}
	c, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka: new client: %w", err)
	}
	return c, nil
}

// NewAdmin returns an admin client for topic + consumer-group management.
// It reuses the same broker list and client-id as NewClient.
func NewAdmin(cfg config.KafkaConfig) (*kadm.Client, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka: at least one broker required")
	}
	c, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ClientID(clientID(cfg)+"-admin"),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka: new admin client: %w", err)
	}
	return kadm.NewClient(c), nil
}

// Close shuts down a franz-go client, waiting for in-flight produces to
// flush. Safe to call on a nil client.
func Close(c *kgo.Client) {
	if c == nil {
		return
	}
	c.Close()
}

func clientID(cfg config.KafkaConfig) string {
	if cfg.ClientID == "" {
		return "fullwa"
	}
	return cfg.ClientID
}
