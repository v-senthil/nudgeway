// Package queue is the port for durable background job enqueuing.
// Implemented on Redis Streams with per-lane consumer groups.
package queue

import (
	"context"
	"time"
)

// Job is the unit of work enqueued to a lane.
type Job struct {
	ID          string
	Lane        string
	Payload     []byte
	MaxAttempts int
	NotBefore   time.Time
	Attempt     int
}

// Enqueuer places a job on a lane and returns its ID.
type Enqueuer interface {
	Enqueue(ctx context.Context, j Job) (string, error)
}

// Consumer reads jobs from a lane. Ack marks a job done; Nack requeues with
// backoff up to MaxAttempts.
type Consumer interface {
	Consume(ctx context.Context, lane string, group string, handler func(context.Context, Job) error) error
}
