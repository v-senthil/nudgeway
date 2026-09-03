// Package metrics wires the Prometheus registry and exposes the canonical
// fullWA metric families (HTTP, providers, workers, webhooks, Kafka,
// WebSocket). All metric names follow the convention
// `fullwa_<subsystem>_<name>_<unit>` and are registered on a dedicated
// [prometheus.Registry] so tests and the /metrics endpoint stay isolated
// from the default global.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns the fullWA Prometheus registry and every metric family the
// platform emits. Construct one with [New] at boot, share the value with
// every subsystem that needs to record, and mount [Metrics.Handler] on the
// `/metrics` route.
type Metrics struct {
	// Registry is the Prometheus registry all fullWA metrics register onto.
	// Exposed so external collectors (runtime, process, custom) can also be
	// registered by the caller.
	Registry *prometheus.Registry

	// HTTPRequestsTotal counts every HTTP request served, labelled by
	// method, path (route template), and status code.
	HTTPRequestsTotal *prometheus.CounterVec
	// HTTPRequestDurationSeconds is the request latency histogram.
	HTTPRequestDurationSeconds *prometheus.HistogramVec

	// ProviderCallsTotal counts calls made to third-party providers.
	// Labels: provider, operation, outcome (ok|error|rate_limited|auth).
	ProviderCallsTotal *prometheus.CounterVec
	// ProviderCallDurationSeconds is provider-call latency.
	ProviderCallDurationSeconds *prometheus.HistogramVec

	// WorkerJobsTotal counts worker jobs processed.
	// Labels: lane, group, outcome (ok|error|dead_letter).
	WorkerJobsTotal *prometheus.CounterVec
	// WorkerJobDurationSeconds is per-job processing latency.
	WorkerJobDurationSeconds *prometheus.HistogramVec
	// WorkerJobRetriesTotal counts retry attempts for worker jobs.
	WorkerJobRetriesTotal *prometheus.CounterVec

	// WebhookEventsReceivedTotal counts inbound webhook events by provider
	// and integration.
	WebhookEventsReceivedTotal *prometheus.CounterVec

	// KafkaProducerBatchBytesTotal counts bytes shipped by the Kafka
	// producer, labelled by topic.
	KafkaProducerBatchBytesTotal *prometheus.CounterVec
	// KafkaConsumerLagRecords is the current per-topic/partition/group
	// consumer lag in records.
	KafkaConsumerLagRecords *prometheus.GaugeVec

	// WebSocketConnections is the current number of active WebSocket
	// connections labelled by a short org id prefix (avoids high cardinality
	// while still preserving per-tenant signal).
	WebSocketConnections *prometheus.GaugeVec
}

// New constructs a Metrics with a fresh registry, registers every fullWA
// metric family, and adds the standard Go runtime + process collectors.
// It panics if any metric fails to register — this only happens on
// programmer error (duplicate name), never at runtime.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,

		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fullwa_http_requests_total",
			Help: "Total HTTP requests served, labelled by method, route template, and status code.",
		}, []string{"method", "path", "status"}),

		HTTPRequestDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "fullwa_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds, labelled by method, route template, and status code.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),

		ProviderCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fullwa_provider_calls_total",
			Help: "Calls to third-party providers, labelled by provider, operation, and outcome.",
		}, []string{"provider", "operation", "outcome"}),

		ProviderCallDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "fullwa_provider_call_duration_seconds",
			Help:    "Provider call latency in seconds, labelled by provider, operation, and outcome.",
			Buckets: prometheus.DefBuckets,
		}, []string{"provider", "operation", "outcome"}),

		WorkerJobsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fullwa_worker_jobs_total",
			Help: "Worker jobs processed, labelled by lane, consumer group, and outcome.",
		}, []string{"lane", "group", "outcome"}),

		WorkerJobDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "fullwa_worker_job_duration_seconds",
			Help:    "Worker job processing latency in seconds, labelled by lane, consumer group, and outcome.",
			Buckets: prometheus.DefBuckets,
		}, []string{"lane", "group", "outcome"}),

		WorkerJobRetriesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fullwa_worker_job_retries_total",
			Help: "Worker job retry attempts, labelled by lane and consumer group.",
		}, []string{"lane", "group"}),

		WebhookEventsReceivedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fullwa_webhook_events_received_total",
			Help: "Inbound webhook events received, labelled by provider and integration id.",
		}, []string{"provider", "integration_id"}),

		KafkaProducerBatchBytesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fullwa_kafka_producer_batch_bytes_total",
			Help: "Total bytes shipped by the Kafka producer, labelled by topic.",
		}, []string{"topic"}),

		KafkaConsumerLagRecords: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fullwa_kafka_consumer_lag_records",
			Help: "Current Kafka consumer lag in records, labelled by topic, partition, and consumer group.",
		}, []string{"topic", "partition", "group"}),

		WebSocketConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fullwa_websocket_connections",
			Help: "Currently open WebSocket connections, labelled by a short org id prefix.",
		}, []string{"org_id_short"}),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDurationSeconds,
		m.ProviderCallsTotal,
		m.ProviderCallDurationSeconds,
		m.WorkerJobsTotal,
		m.WorkerJobDurationSeconds,
		m.WorkerJobRetriesTotal,
		m.WebhookEventsReceivedTotal,
		m.KafkaProducerBatchBytesTotal,
		m.KafkaConsumerLagRecords,
		m.WebSocketConnections,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return m
}

// Handler returns an [http.Handler] that serves the Prometheus exposition
// format for this Metrics' registry. Mount on `/metrics`.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{
		Registry:          m.Registry,
		EnableOpenMetrics: true,
	})
}
