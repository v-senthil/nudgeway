package kafka

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/v-senthil/nudgeway/internal/infrastructure/health"
)

// Probe returns a readiness Probe that checks broker connectivity by
// asking the client to load its metadata. Suitable for /readyz.
func Probe(client *kgo.Client) health.Probe {
	return health.Probe{
		Name: "kafka",
		Check: func(ctx context.Context) error {
			if client == nil {
				return fmt.Errorf("kafka: client is nil")
			}
			if err := client.Ping(ctx); err != nil {
				return fmt.Errorf("kafka: ping: %w", err)
			}
			return nil
		},
	}
}
