// Package hbase implements the MessageStore, EventStore, and
// AttachmentStore ports against HBase. Row-key design per docs/architecture.md
// — tenant-prefixed and time-bucketed to avoid hotspots.
package hbase
