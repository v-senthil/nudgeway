// Package workers hosts the background consumers that drain the Redis
// Streams job lanes: message send, webhook process, campaign job, ticket
// sync, ai invoke. Each worker uses a bounded pool.
package workers
