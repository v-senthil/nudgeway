package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/v-senthil/nudgeway/internal/infrastructure/config"
	"github.com/v-senthil/nudgeway/internal/ports/queue"
)

// Consumer implements queue.Consumer. Each call to Consume spins up a
// dedicated franz-go client in consumer-group mode subscribed to one lane
// topic. The client is closed when the supplied ctx is cancelled.
//
// Consumer intentionally does NOT share the Producer's franz-go client:
// franz-go clients pin themselves to a single consumer group per instance
// and mixing produce + consume on the same client complicates lifecycle.
type Consumer struct {
	cfg    config.KafkaConfig
	prefix string
	logger *slog.Logger
}

// NewConsumer builds a Consumer using cfg for broker discovery + client id.
// If logger is nil, slog.Default() is used.
func NewConsumer(cfg config.KafkaConfig, logger *slog.Logger) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Consumer{cfg: cfg, prefix: cfg.TopicsPrefix, logger: logger}
}

// Consume subscribes to <prefix>.jobs.<lane> under the given consumer
// group and dispatches records to handler until ctx is cancelled.
//
// Commit policy: records are committed only after the handler returns
// nil. A handler error leaves the record uncommitted so the next poll
// re-delivers it — retry policy (backoff, max attempts, DLQ) is the
// caller's responsibility via the queue.Job envelope.
func (c *Consumer) Consume(
	ctx context.Context,
	lane string,
	group string,
	handler func(context.Context, queue.Job) error,
) error {
	if lane == "" {
		return fmt.Errorf("kafka: consume: lane is required")
	}
	if group == "" {
		return fmt.Errorf("kafka: consume: group is required")
	}
	topic := TopicName(c.prefix, KindJob, lane)
	client, err := kgo.NewClient(
		kgo.SeedBrokers(c.cfg.Brokers...),
		kgo.ClientID(clientID(c.cfg)+"-"+group),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.SessionTimeout(30*time.Second),
	)
	if err != nil {
		return fmt.Errorf("kafka: consume: new client: %w", err)
	}
	defer client.Close()

	log := c.logger.With(
		slog.String("component", "kafka.consumer"),
		slog.String("topic", topic),
		slog.String("group", group),
	)
	log.Info("consumer started")
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		start := time.Now()
		fetches := client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			// Only fail out on non-context, non-transient client errors.
			for _, fe := range errs {
				if errors.Is(fe.Err, context.Canceled) || errors.Is(fe.Err, context.DeadlineExceeded) {
					return fe.Err
				}
				log.Warn("fetch error", slog.String("topic", fe.Topic), slog.Any("err", fe.Err))
			}
		}
		batchCount := 0
		var toCommit []*kgo.Record
		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()
			job, decErr := DecodeJob(rec.Value)
			if decErr != nil {
				// Poison-pill record — commit to move past it and keep going.
				log.Error("decode job failed; skipping",
					slog.String("key", string(rec.Key)),
					slog.Any("err", decErr))
				toCommit = append(toCommit, rec)
				continue
			}
			if hErr := handler(ctx, job); hErr != nil {
				log.Warn("handler returned error; will re-consume",
					slog.String("job_id", job.ID),
					slog.Any("err", hErr))
				// Stop committing further records so ordering is preserved
				// on retry: everything from this partition offset onward
				// will be redelivered.
				break
			}
			toCommit = append(toCommit, rec)
			batchCount++
		}
		if len(toCommit) > 0 {
			if err := client.CommitRecords(ctx, toCommit...); err != nil {
				log.Error("commit failed", slog.Any("err", err))
			}
		}
		if batchCount > 0 {
			log.Debug("batch processed",
				slog.Int("count", batchCount),
				slog.Duration("latency", time.Since(start)))
		}
	}
}
