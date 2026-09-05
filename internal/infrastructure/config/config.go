// Package config loads runtime configuration from YAML, applies NUDGEWAY_*
// environment-variable overrides, and validates the result.
//
// Precedence: env vars > config file > defaults.
//
// The Phase 0 loader intentionally uses only stdlib + a tiny hand-rolled YAML
// parser scoped to the subset used by config/example.yaml. When the tree grows
// (nested overrides, hot reload) we swap in a proper library behind this
// package's boundary without changing callers.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the root configuration object.
type Config struct {
	Env       string
	HTTP      HTTPConfig
	Auth      AuthConfig
	MySQL     MySQLConfig
	Redis     RedisConfig
	HBase     HBaseConfig
	Log       LogConfig
	Metrics   MetricsConfig
	Tracing   TracingConfig
	WebSocket WebSocketConfig
	Frontend  FrontendConfig
	Kafka     KafkaConfig
	Workers   WorkersConfig
	// Attachments configures the media/attachment blob store. In dev this
	// is a local filesystem directory; production deployments swap the
	// impl behind attachments.Store.
	Attachments AttachmentsConfig
}

// AttachmentsConfig configures the attachment blob store.
type AttachmentsConfig struct {
	// Root is the on-disk directory used by the local filesystem store.
	// Defaults to "./attachments" relative to the process working
	// directory when unset.
	Root string
}

// WorkersConfig groups the tunables for every background worker pool. Each
// lane has its own concurrency + retry policy so operators can dial hot
// paths independently. Zero values fall back to a sensible default
// (concurrency=8) so a partial config never silently disables a worker.
type WorkersConfig struct {
	// MessageSend drives the outbound send worker (queue lane "message.send").
	MessageSend WorkerConfig
	// WebhookProcess drives the inbound webhook worker (queue lane
	// "webhook.process").
	WebhookProcess WorkerConfig
	// TicketSync is reserved for the ticketing sync worker landing later.
	TicketSync WorkerConfig
	// CampaignJob is reserved for the campaign dispatcher landing in Phase 2.
	CampaignJob WorkerConfig
	// AIInvoke is reserved for the AI orchestrator worker landing in Phase 3.
	AIInvoke WorkerConfig
}

// WorkerConfig configures a single worker pool.
type WorkerConfig struct {
	// Concurrency is the number of goroutines the pool spawns. Zero or
	// negative means "use the default" (8).
	Concurrency int
	// MaxRetries caps the redelivery attempts before the job is deadlettered.
	MaxRetries int
	// InitialBackoff is the initial retry delay used by the queue consumer.
	InitialBackoff time.Duration
}

// EffectiveConcurrency returns Concurrency if positive, else the default
// worker concurrency (8). Kept as a method so wire-up code does not scatter
// the default constant across the tree.
func (w WorkerConfig) EffectiveConcurrency() int {
	if w.Concurrency > 0 {
		return w.Concurrency
	}
	return 8
}

// KafkaConfig configures the durable event log + job queue backend.
// Brokers list is the bootstrap set; TopicsPrefix namespaces every topic
// (default "nudgeway") so multiple deploys can share a cluster without
// colliding. ReplicationFactor and DefaultPartitions are applied when
// EnsureTopics creates a missing topic.
type KafkaConfig struct {
	Brokers           []string
	ClientID          string
	TopicsPrefix      string
	ReplicationFactor int16
	DefaultPartitions int32
}

// HTTPConfig configures the public HTTP server.
type HTTPConfig struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// AuthConfig configures session cookies and password hashing.
type AuthConfig struct {
	SessionCookieName string
	SessionTTL        time.Duration
	CSRFCookieName    string
	CredentialKEKHex  string
}

// MySQLConfig configures the transactional store.
type MySQLConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// RedisConfig configures the coordination store.
type RedisConfig struct {
	Addr     string
	DB       int
	Password string
	PoolSize int
}

// HBaseConfig configures the high-volume event store.
type HBaseConfig struct {
	ZookeeperQuorum []string
	ZNodeParent     string
	Namespace       string
	ThriftAddr      string
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level     string
	Format    string
	AddSource bool
}

// MetricsConfig configures the Prometheus exporter.
type MetricsConfig struct {
	Enabled bool
	Path    string
}

// TracingConfig configures OpenTelemetry export.
type TracingConfig struct {
	Enabled      bool
	ServiceName  string
	OTLPEndpoint string
}

// WebSocketConfig configures the real-time hub.
type WebSocketConfig struct {
	PingInterval   time.Duration
	WriteTimeout   time.Duration
	MaxMessageSize int
}

// FrontendConfig controls how the embedded frontend is served in dev vs prod.
type FrontendConfig struct {
	ServeEmbedded bool
	ViteDevProxy  string
}

// Load reads YAML from path, applies NUDGEWAY_* overrides, and validates.
func Load(path string) (Config, error) {
	cfg := defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return cfg, fmt.Errorf("read %s: %w", path, err)
			}
			// Missing local.yaml is fine — env vars + defaults may cover it.
		} else if err := applyYAML(&cfg, string(b)); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	applyEnv(&cfg)
	if err := validate(cfg); err != nil {
		return cfg, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func defaults() Config {
	return Config{
		Env: "local",
		HTTP: HTTPConfig{
			Addr:            ":8080",
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 20 * time.Second,
		},
		Auth: AuthConfig{
			SessionCookieName: "nudgeway_session",
			SessionTTL:        720 * time.Hour,
			CSRFCookieName:    "nudgeway_csrf",
		},
		MySQL: MySQLConfig{MaxOpenConns: 32, MaxIdleConns: 8, ConnMaxLifetime: 30 * time.Minute},
		Redis: RedisConfig{Addr: "127.0.0.1:6379", PoolSize: 32},
		HBase: HBaseConfig{ZNodeParent: "/hbase", Namespace: "nudgeway"},
		Log:   LogConfig{Level: "info", Format: "json"},
		Metrics: MetricsConfig{Enabled: true, Path: "/metrics"},
		WebSocket: WebSocketConfig{
			PingInterval:   20 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxMessageSize: 262144,
		},
		Frontend:    FrontendConfig{ViteDevProxy: "http://127.0.0.1:5173"},
		Attachments: AttachmentsConfig{Root: "./attachments"},
		Kafka: KafkaConfig{
			Brokers:           []string{"127.0.0.1:9092"},
			ClientID:          "nudgeway",
			TopicsPrefix:      "nudgeway",
			ReplicationFactor: 1,
			DefaultPartitions: 6,
		},
	}
}

// applyYAML parses the subset of YAML used by config/example.yaml. It supports
// two-level nesting (section: key: value), inline `[a, b]` lists, and quoted
// strings. It is intentionally strict — anything else fails validation early.
//
//nolint:funlen,gocyclo // small hand-rolled parser is easier to audit than a dependency.
func applyYAML(cfg *Config, src string) error {
	lines := strings.Split(src, "\n")
	section := ""
	for i, raw := range lines {
		line := stripComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingSpaces(line)
		trimmed := strings.TrimSpace(line)
		if indent == 0 && strings.HasSuffix(trimmed, ":") {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if indent == 0 {
			// top-level scalar: "key: value"
			k, v := splitKV(trimmed)
			if k == "" {
				return fmt.Errorf("line %d: %q", i+1, raw)
			}
			if err := setScalar(cfg, "", k, v); err != nil {
				return fmt.Errorf("line %d: %w", i+1, err)
			}
			continue
		}
		// nested "key: value"
		k, v := splitKV(trimmed)
		if k == "" {
			continue
		}
		if err := setScalar(cfg, section, k, v); err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
	}
	return nil
}

func stripComment(s string) string {
	// naive: strip everything after " #" that isn't inside quotes.
	inQ := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQ = !inQ
		}
		if !inQ && c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
			return s[:i]
		}
	}
	return s
}

func leadingSpaces(s string) int {
	n := 0
	for _, c := range s {
		if c != ' ' && c != '\t' {
			break
		}
		n++
	}
	return n
}

func splitKV(s string) (string, string) {
	i := strings.Index(s, ":")
	if i < 0 {
		return "", ""
	}
	k := strings.TrimSpace(s[:i])
	v := strings.TrimSpace(s[i+1:])
	v = strings.Trim(v, `"`)
	return k, v
}

//nolint:gocyclo,funlen,gocritic // exhaustive but flat switch.
func setScalar(cfg *Config, section, key, val string) error {
	switch section {
	case "":
		switch key {
		case "env":
			cfg.Env = val
		}
	case "http":
		switch key {
		case "addr":
			cfg.HTTP.Addr = val
		case "read_timeout":
			return dur(&cfg.HTTP.ReadTimeout, val)
		case "write_timeout":
			return dur(&cfg.HTTP.WriteTimeout, val)
		case "idle_timeout":
			return dur(&cfg.HTTP.IdleTimeout, val)
		case "shutdown_timeout":
			return dur(&cfg.HTTP.ShutdownTimeout, val)
		}
	case "auth":
		switch key {
		case "session_cookie_name":
			cfg.Auth.SessionCookieName = val
		case "csrf_cookie_name":
			cfg.Auth.CSRFCookieName = val
		case "session_ttl":
			return dur(&cfg.Auth.SessionTTL, val)
		case "credential_kek_hex":
			cfg.Auth.CredentialKEKHex = val
		}
	case "mysql":
		switch key {
		case "dsn":
			cfg.MySQL.DSN = val
		case "max_open_conns":
			return intp(&cfg.MySQL.MaxOpenConns, val)
		case "max_idle_conns":
			return intp(&cfg.MySQL.MaxIdleConns, val)
		case "conn_max_lifetime":
			return dur(&cfg.MySQL.ConnMaxLifetime, val)
		}
	case "redis":
		switch key {
		case "addr":
			cfg.Redis.Addr = val
		case "db":
			return intp(&cfg.Redis.DB, val)
		case "password":
			cfg.Redis.Password = val
		case "pool_size":
			return intp(&cfg.Redis.PoolSize, val)
		}
	case "hbase":
		switch key {
		case "zookeeper_quorum":
			cfg.HBase.ZookeeperQuorum = parseList(val)
		case "zookeeper_znode_parent":
			cfg.HBase.ZNodeParent = val
		case "namespace":
			cfg.HBase.Namespace = val
		case "thrift_addr":
			cfg.HBase.ThriftAddr = val
		}
	case "log":
		switch key {
		case "level":
			cfg.Log.Level = val
		case "format":
			cfg.Log.Format = val
		case "add_source":
			cfg.Log.AddSource = val == "true"
		}
	case "metrics":
		switch key {
		case "enabled":
			cfg.Metrics.Enabled = val == "true"
		case "path":
			cfg.Metrics.Path = val
		}
	case "tracing":
		switch key {
		case "enabled":
			cfg.Tracing.Enabled = val == "true"
		case "service_name":
			cfg.Tracing.ServiceName = val
		case "otlp_endpoint":
			cfg.Tracing.OTLPEndpoint = val
		}
	case "websocket":
		switch key {
		case "ping_interval":
			return dur(&cfg.WebSocket.PingInterval, val)
		case "write_timeout":
			return dur(&cfg.WebSocket.WriteTimeout, val)
		case "max_message_size":
			return intp(&cfg.WebSocket.MaxMessageSize, val)
		}
	case "frontend":
		switch key {
		case "serve_embedded":
			cfg.Frontend.ServeEmbedded = val == "true"
		case "vite_dev_proxy":
			cfg.Frontend.ViteDevProxy = val
		}
	case "kafka":
		switch key {
		case "brokers":
			cfg.Kafka.Brokers = parseList(val)
		case "client_id":
			cfg.Kafka.ClientID = val
		case "topics_prefix":
			cfg.Kafka.TopicsPrefix = val
		case "replication_factor":
			n, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("bad int %q: %w", val, err)
			}
			cfg.Kafka.ReplicationFactor = int16(n)
		case "default_partitions":
			n, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("bad int %q: %w", val, err)
			}
			cfg.Kafka.DefaultPartitions = int32(n)
		}
	case "workers":
		// Flat schema to stay compatible with the two-level parser:
		// each per-lane setting is a single key with underscore-joined
		// components, e.g. `message_send_concurrency: 16`.
		switch key {
		case "message_send_concurrency":
			return intp(&cfg.Workers.MessageSend.Concurrency, val)
		case "message_send_max_retries":
			return intp(&cfg.Workers.MessageSend.MaxRetries, val)
		case "message_send_initial_backoff":
			return dur(&cfg.Workers.MessageSend.InitialBackoff, val)
		case "webhook_process_concurrency":
			return intp(&cfg.Workers.WebhookProcess.Concurrency, val)
		case "webhook_process_max_retries":
			return intp(&cfg.Workers.WebhookProcess.MaxRetries, val)
		case "webhook_process_initial_backoff":
			return dur(&cfg.Workers.WebhookProcess.InitialBackoff, val)
		case "ticket_sync_concurrency":
			return intp(&cfg.Workers.TicketSync.Concurrency, val)
		case "campaign_job_concurrency":
			return intp(&cfg.Workers.CampaignJob.Concurrency, val)
		case "ai_invoke_concurrency":
			return intp(&cfg.Workers.AIInvoke.Concurrency, val)
		}
	case "attachments":
		switch key {
		case "root":
			cfg.Attachments.Root = val
		}
	}
	return nil
}

func dur(dst *time.Duration, s string) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("bad duration %q: %w", s, err)
	}
	*dst = d
	return nil
}

func intp(dst *int, s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("bad int %q: %w", s, err)
	}
	*dst = n
	return nil
}

func parseList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("NUDGEWAY_HTTP_ADDR"); v != "" {
		cfg.HTTP.Addr = v
	}
	if v := os.Getenv("NUDGEWAY_MYSQL_DSN"); v != "" {
		cfg.MySQL.DSN = v
	}
	if v := os.Getenv("NUDGEWAY_REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("NUDGEWAY_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("NUDGEWAY_ENV"); v != "" {
		cfg.Env = v
	}
	if v := os.Getenv("NUDGEWAY_KAFKA_BROKERS"); v != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			cfg.Kafka.Brokers = out
		}
	}
}

func validate(cfg Config) error {
	if cfg.HTTP.Addr == "" {
		return errors.New("http.addr is required")
	}
	if cfg.HTTP.ShutdownTimeout <= 0 {
		return errors.New("http.shutdown_timeout must be > 0")
	}
	return nil
}
