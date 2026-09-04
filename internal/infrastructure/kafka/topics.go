package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
)

// TopicKind selects the naming scheme used by TopicName.
type TopicKind string

// Recognised topic kinds.
const (
	// KindJob is a background job lane — <prefix>.jobs.<name>.
	KindJob TopicKind = "jobs"
	// KindEvent is a canonical domain event — <prefix>.events.<name>.
	KindEvent TopicKind = "events"
)

// TopicName returns the fully qualified Kafka topic name for a kind + name
// under the given prefix.
func TopicName(prefix string, kind TopicKind, name string) string {
	if prefix == "" {
		prefix = "nudgeway"
	}
	return fmt.Sprintf("%s.%s.%s", prefix, string(kind), name)
}

// EnsureTopics creates every requested topic that does not yet exist. It is
// idempotent — topics that already exist are left alone and are not treated
// as errors. topics is the list of already-qualified topic names (as
// returned by TopicName).
//
// Callers should keep partitions and factor identical across restarts to
// avoid surprising the broker; changing them here has no effect on an
// existing topic — resize via kafka-topics --alter.
func EnsureTopics(
	ctx context.Context,
	adm *kadm.Client,
	factor int16,
	partitions int32,
	topics []string,
) error {
	if len(topics) == 0 {
		return nil
	}
	if partitions <= 0 {
		partitions = 1
	}
	if factor <= 0 {
		factor = 1
	}
	resp, err := adm.CreateTopics(ctx, partitions, factor, nil, topics...)
	if err != nil {
		return fmt.Errorf("kafka: create topics: %w", err)
	}
	for _, r := range resp {
		if r.Err == nil {
			continue
		}
		// Already-exists is fine.
		if strings.Contains(strings.ToLower(r.Err.Error()), "already exists") {
			continue
		}
		// Any other broker-level error is unexpected.
		return fmt.Errorf("kafka: ensure topic %s: %w", r.Topic, r.Err)
	}
	return nil
}
