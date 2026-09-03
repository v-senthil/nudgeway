package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/fullwa/fullwa/internal/domain/events"
	"github.com/fullwa/fullwa/internal/infrastructure/config"
)

// EventBus implements eventbus.Subscriber for durable cross-node event
// delivery. Each Subscribe call starts a managed background goroutine that
// consumes <prefix>.events.<type> under a consumer group derived from the
// group name passed to NewEventBus + the event type.
//
// Close cancels every managed goroutine and waits for them to exit — no
// unbounded goroutines escape this type.
type EventBus struct {
	cfg    config.KafkaConfig
	prefix string
	group  string
	logger *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	handlers map[events.Type][]events.Handler
	started  map[events.Type]bool
}

// NewEventBus builds an EventBus using cfg for broker discovery. group is
// the consumer-group name prefix — each subscribed event type appends its
// name so different event types are consumed independently.
func NewEventBus(cfg config.KafkaConfig, group string, logger *slog.Logger) *EventBus {
	if logger == nil {
		logger = slog.Default()
	}
	if group == "" {
		group = "fullwa"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &EventBus{
		cfg:      cfg,
		prefix:   cfg.TopicsPrefix,
		group:    group,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		handlers: map[events.Type][]events.Handler{},
		started:  map[events.Type]bool{},
	}
}

// Subscribe registers a Handler for the given event Type. The first
// subscribe for a Type starts a background consumer; subsequent
// subscribes for the same Type share the same consumer and fan out
// synchronously.
func (b *EventBus) Subscribe(t events.Type, h events.Handler) {
	b.mu.Lock()
	b.handlers[t] = append(b.handlers[t], h)
	needStart := !b.started[t]
	if needStart {
		b.started[t] = true
	}
	b.mu.Unlock()
	if needStart {
		b.wg.Add(1)
		go b.consume(t)
	}
}

// Close cancels every background consumer and waits for them to exit.
// After Close no further Subscribe calls should be made.
func (b *EventBus) Close() {
	b.cancel()
	b.wg.Wait()
}

func (b *EventBus) consume(t events.Type) {
	defer b.wg.Done()
	topic := TopicName(b.prefix, KindEvent, string(t))
	group := b.group + ".events." + string(t)
	log := b.logger.With(
		slog.String("component", "kafka.eventbus"),
		slog.String("topic", topic),
		slog.String("group", group),
	)
	client, err := kgo.NewClient(
		kgo.SeedBrokers(b.cfg.Brokers...),
		kgo.ClientID(clientID(b.cfg)+"-"+group),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.SessionTimeout(30*time.Second),
	)
	if err != nil {
		log.Error("consumer init failed", slog.Any("err", err))
		return
	}
	defer client.Close()
	log.Info("event consumer started")

	for {
		if err := b.ctx.Err(); err != nil {
			return
		}
		start := time.Now()
		fetches := client.PollFetches(b.ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, fe := range errs {
				if errors.Is(fe.Err, context.Canceled) || errors.Is(fe.Err, context.DeadlineExceeded) {
					return
				}
				log.Warn("fetch error", slog.String("topic", fe.Topic), slog.Any("err", fe.Err))
			}
		}
		var toCommit []*kgo.Record
		batchCount := 0
		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()
			env, decErr := DecodeEnvelope(rec.Value)
			if decErr != nil {
				log.Error("decode envelope failed; skipping",
					slog.String("key", string(rec.Key)),
					slog.Any("err", decErr))
				toCommit = append(toCommit, rec)
				continue
			}
			if err := b.dispatch(env); err != nil {
				log.Warn("handler returned error; will re-consume",
					slog.String("event_type", string(env.Type)),
					slog.Any("err", err))
				break
			}
			toCommit = append(toCommit, rec)
			batchCount++
		}
		if len(toCommit) > 0 {
			if err := client.CommitRecords(b.ctx, toCommit...); err != nil {
				log.Error("commit failed", slog.Any("err", err))
			}
		}
		if batchCount > 0 {
			log.Debug("event batch processed",
				slog.Int("count", batchCount),
				slog.Duration("latency", time.Since(start)))
		}
	}
}

func (b *EventBus) dispatch(env events.Envelope) error {
	b.mu.Lock()
	hs := append([]events.Handler(nil), b.handlers[env.Type]...)
	b.mu.Unlock()
	var firstErr error
	for _, h := range hs {
		if err := h(env); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("event handler for %s: %w", env.Type, err)
		}
	}
	return firstErr
}
