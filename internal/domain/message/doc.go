// Package message models canonical messages. Every Message belongs to a
// Session and a Conversation, and carries provider-agnostic status, type,
// and metadata. Message payloads live in HBase, referenced by payload_ref.
package message
