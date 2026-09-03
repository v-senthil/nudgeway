package health

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// kafkaProbeTimeout bounds the TCP dial for each broker. Deliberately
// short — /readyz has its own overall timeout and Phase 1 only needs to
// know the broker socket answers. A deeper "metadata fetch" probe is
// Phase 2.
const kafkaProbeTimeout = 500 * time.Millisecond

// KafkaProbe returns a Probe that attempts a TCP connect to each broker
// with a 500ms per-broker timeout. The probe passes as soon as any one
// broker answers, which matches Kafka client bootstrap semantics — you
// need at least one reachable broker to discover the rest of the cluster.
// If brokers is empty the probe fails immediately with a clear error.
func KafkaProbe(brokers []string) Probe {
	return Probe{
		Name: "kafka",
		Check: func(ctx context.Context) error {
			if len(brokers) == 0 {
				return errors.New("kafka: no brokers configured")
			}
			var lastErr error
			for _, b := range brokers {
				dctx, cancel := context.WithTimeout(ctx, kafkaProbeTimeout)
				d := net.Dialer{Timeout: kafkaProbeTimeout}
				conn, err := d.DialContext(dctx, "tcp", b)
				cancel()
				if err != nil {
					lastErr = fmt.Errorf("kafka: dial %s: %w", b, err)
					continue
				}
				_ = conn.Close()
				return nil
			}
			if lastErr == nil {
				lastErr = errors.New("kafka: no broker reachable")
			}
			return lastErr
		},
	}
}
