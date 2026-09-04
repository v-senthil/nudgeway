package analytics

import (
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// SeriesKind names one of the read-side time-series a dashboard chart
// can request. The Service switches on the value; unknown values return
// ErrUnknownSeries.
type SeriesKind string

// Well-known series kinds. Add new values alongside the code that reads
// them; the string is treated as an API and appears in query strings.
const (
	// SeriesMessagesDaily plots the total messages/day (all providers,
	// all directions, all types).
	SeriesMessagesDaily SeriesKind = "messages_daily"
	// SeriesDeliveryRate plots the delivery rate (delivered / sent) over
	// time as a percentage in the 0..100 range (Value is *100 to keep
	// the wire shape integer-friendly).
	SeriesDeliveryRate SeriesKind = "delivery_rate"
	// SeriesConversationsOpened plots conversations opened per day.
	SeriesConversationsOpened SeriesKind = "conversations_opened"
	// SeriesCallsDaily plots calls per day (pan-direction "all" row).
	SeriesCallsDaily SeriesKind = "calls_daily"
)

// Call direction sentinels used in the analytics_calls_daily rollup.
const (
	// CallDirectionAll is the pan-direction aggregate row.
	CallDirectionAll = "all"
	// CallDirectionInbound narrows to inbound calls.
	CallDirectionInbound = "inbound"
	// CallDirectionOutbound narrows to outbound calls.
	CallDirectionOutbound = "outbound"
)

// MessagesDaily is one row of the analytics_messages_daily rollup table.
// Provider and MessageType default to the sentinel "all" for rows that
// aggregate across every provider or every message type respectively.
type MessagesDaily struct {
	// OrgID is the tenant boundary. Every read is scoped by it.
	OrgID organization.ID
	// Day is a UTC calendar date. Time-of-day fields are zero.
	Day time.Time
	// Provider is the provider registry key or the sentinel "all" for
	// the pan-provider aggregate. Treated as an opaque tenant-scoped
	// string; the domain package never enumerates concrete providers.
	Provider string
	// Direction is "inbound" or "outbound".
	Direction string
	// MessageType is the domain message type (e.g. "text", "image") or
	// the sentinel "all" for the pan-type aggregate.
	MessageType string
	// Total is the count of messages matching the row's dimensions.
	Total int64
	// Delivered is the subset that reached "delivered" state or later.
	Delivered int64
	// ReadCount is the subset that reached "read" state.
	ReadCount int64
	// Failed is the subset that terminated in the "failed" state.
	Failed int64
}

// ConversationsDaily is one row of the analytics_conversations_daily
// rollup table.
type ConversationsDaily struct {
	// OrgID is the tenant boundary.
	OrgID organization.ID
	// Day is a UTC calendar date.
	Day time.Time
	// Opened is the number of conversations that transitioned into an
	// open state on the given day.
	Opened int64
	// Resolved is the number of conversations that transitioned into
	// the resolved state on the given day.
	Resolved int64
	// AvgResponseTimeSeconds is a coarse average of the seconds between
	// an inbound message and the next outbound reply on that day.
	AvgResponseTimeSeconds int64
}

// DeliveryRateDaily is one row of the analytics_delivery_rate_daily
// rollup table. Provider "all" carries the pan-provider aggregate.
type DeliveryRateDaily struct {
	// OrgID is the tenant boundary.
	OrgID organization.ID
	// Day is a UTC calendar date.
	Day time.Time
	// Provider is the registry key or "all" sentinel.
	Provider string
	// Sent is the count of outbound messages that entered a
	// "sent"-or-later state.
	Sent int64
	// Delivered is the subset delivered.
	Delivered int64
	// ReadCount is the subset read.
	ReadCount int64
	// Failed is the subset that failed.
	Failed int64
}

// CallsDaily is one row of the analytics_calls_daily rollup table.
// Direction defaults to the sentinel "all" for rows that aggregate across
// both inbound and outbound.
type CallsDaily struct {
	// OrgID is the tenant boundary.
	OrgID organization.ID
	// Day is a UTC calendar date.
	Day time.Time
	// Direction is "inbound", "outbound", or the sentinel "all" for the
	// pan-direction aggregate row.
	Direction string
	// Total is the count of matching calls on the day.
	Total int
	// Answered is the subset that reached the "answered" state or later.
	Answered int
	// Completed is the subset that reached the "completed" terminal state.
	Completed int
	// Failed is the subset that terminated in the "failed" state.
	Failed int
	// Missed is the subset that terminated as "missed" or "no_answer".
	Missed int
	// DurationSecondsTotal is the sum of duration_seconds over the row.
	DurationSecondsTotal int
}

// Point is one (day, value) pair inside a Series.
type Point struct {
	// Day is a UTC calendar date. Time-of-day fields are zero.
	Day time.Time
	// Value is the integer aggregate for the day.
	Value int64
}

// Series is a labelled sequence of Points suitable for a line chart.
type Series struct {
	// Name is a human-readable label for the series (e.g. "messages",
	// "delivery_rate_pct"). Used verbatim as a chart legend entry.
	Name string
	// Points are ordered by ascending Day.
	Points []Point
}

// Overview is the compact aggregate the dashboard cards render at the
// top of the page.
type Overview struct {
	// MessagesTotal is the total messages in the range across every
	// provider and direction.
	MessagesTotal int64
	// DeliveryRatePct is the delivered / sent ratio in the range,
	// expressed as a percentage in the 0..100 range. Zero when Sent is
	// zero (no divide-by-zero).
	DeliveryRatePct int64
	// ResponseTimeSecondsP50 is a coarse p50 of the daily average
	// response time in the range. It is derived from the per-day
	// averages already persisted in analytics_conversations_daily.
	ResponseTimeSecondsP50 int64
	// ConversationsOpened is the total conversations opened in range.
	ConversationsOpened int64
	// CallsTotal is the total calls in range across both directions.
	CallsTotal int64
	// CallsAnswered is the subset that reached "answered" or later.
	CallsAnswered int64
	// CallsAvgDurationSeconds is the average duration_seconds across calls
	// in range. Zero when no calls occurred (no divide-by-zero).
	CallsAvgDurationSeconds int64
}
