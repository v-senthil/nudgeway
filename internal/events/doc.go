// Package events implements the internal event bus: in-process fan-out for
// same-node handlers and Redis Streams for cross-node fan-out. Ordering
// per conversation_id is preserved by the streams consumer group.
package events
