// Command fullwa is the single-binary entrypoint for the fullWA platform.
//
// It boots the HTTP server, wires MySQL + Redis + Kafka, builds every
// application service, mounts the v1 REST API + webhook ingress + WebSocket
// hub, starts background worker pools, registers Prometheus metrics, and
// shuts everything down gracefully on SIGINT / SIGTERM.
//
// Configuration is loaded from config/local.yaml (or FULLWA_CONFIG) with
// FULLWA_* env vars overriding individual keys.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oklog/ulid/v2"

	v1 "github.com/fullwa/fullwa/internal/api/rest/v1"
	appauth "github.com/fullwa/fullwa/internal/application/auth"
	appintegration "github.com/fullwa/fullwa/internal/application/integration"
	appmsg "github.com/fullwa/fullwa/internal/application/message"
	devents "github.com/fullwa/fullwa/internal/domain/events"
	dintegration "github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/events"
	infauth "github.com/fullwa/fullwa/internal/infrastructure/auth"
	"github.com/fullwa/fullwa/internal/infrastructure/config"
	"github.com/fullwa/fullwa/internal/infrastructure/crypto"
	"github.com/fullwa/fullwa/internal/infrastructure/health"
	fhttp "github.com/fullwa/fullwa/internal/infrastructure/http"
	fkafka "github.com/fullwa/fullwa/internal/infrastructure/kafka"
	fmetrics "github.com/fullwa/fullwa/internal/infrastructure/metrics"
	fmysql "github.com/fullwa/fullwa/internal/infrastructure/mysql"
	fredis "github.com/fullwa/fullwa/internal/infrastructure/redis"
	fws "github.com/fullwa/fullwa/internal/infrastructure/websocket"
	"github.com/fullwa/fullwa/internal/ports/channel"
	"github.com/fullwa/fullwa/internal/ports/queue"
	"github.com/fullwa/fullwa/internal/providers/whatsapp"
	"github.com/fullwa/fullwa/internal/webhook"
	"github.com/fullwa/fullwa/internal/workers"
)

// version is stamped at build time via `-ldflags "-X main.version=..."`.
var version = "0.0.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

//nolint:funlen,gocyclo // wire-up is deliberately linear so the boot order is auditable at a glance.
func run() error {
	cfgPath := flag.String("config", envOr("FULLWA_CONFIG", "config/local.yaml"), "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Log)
	slog.SetDefault(logger)
	logger.Info("fullwa starting",
		slog.String("version", version),
		slog.String("env", cfg.Env),
		slog.String("config", *cfgPath),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- MySQL --------------------------------------------------------------
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := fmysql.Open(dbCtx, cfg.MySQL)
	dbCancel()
	if err != nil {
		return fmt.Errorf("mysql: %w", err)
	}
	logger.Info("mysql connected")

	// --- Redis --------------------------------------------------------------
	rdb := fredis.New(cfg.Redis)
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	if err := fredis.Ping(pingCtx, rdb); err != nil {
		logger.Warn("redis ping failed at boot", slog.Any("err", err))
	} else {
		logger.Info("redis connected")
	}
	pingCancel()

	// --- Credential envelope crypto -----------------------------------------
	kek, err := crypto.ParseKEKHex(cfg.Auth.CredentialKEKHex)
	if err != nil {
		return fmt.Errorf("credential_kek_hex: %w", err)
	}
	envelope, err := crypto.NewEnvelope(kek)
	if err != nil {
		return fmt.Errorf("envelope: %w", err)
	}

	// --- Phase 0 repos ------------------------------------------------------
	users := fmysql.NewUsers(db)
	sessionStore := fmysql.NewSessions(db)
	perms := fmysql.NewRBAC(db)
	orgs := fmysql.NewOrgs(db)

	// --- Phase 1 repos ------------------------------------------------------
	contacts := fmysql.NewContacts(db)
	identities := fmysql.NewIdentities(db)
	businessEndpoints := fmysql.NewBusinessEndpoints(db)
	integrations := fmysql.NewIntegrations(db, envelope)
	commSessions := fmysql.NewSessionsComm(db)
	conversations := fmysql.NewConversations(db)
	messages := fmysql.NewMessages(db)
	webhookEvents := fmysql.NewWebhookEvents(db)

	// --- Kafka (best-effort at boot; server still runs without it) ---------
	var kProducer *fkafka.Producer
	var kConsumer *fkafka.Consumer
	kClient, kErr := fkafka.NewClient(cfg.Kafka)
	if kErr != nil {
		logger.Warn("kafka client failed; workers + async pipelines disabled",
			slog.Any("err", kErr))
	} else {
		adm, aErr := fkafka.NewAdmin(cfg.Kafka)
		if aErr != nil {
			logger.Warn("kafka admin failed", slog.Any("err", aErr))
		} else {
			topicCtx, topicCancel := context.WithTimeout(ctx, 10*time.Second)
			// EnsureTopics wants fully-qualified names; TopicName is the
			// canonical builder — matches what the producer computes on send.
			prefix := cfg.Kafka.TopicsPrefix
			required := []string{
				fkafka.TopicName(prefix, fkafka.KindJob, "webhook.process"),
				fkafka.TopicName(prefix, fkafka.KindJob, "message.send"),
			}
			if err := fkafka.EnsureTopics(topicCtx, adm,
				cfg.Kafka.ReplicationFactor, cfg.Kafka.DefaultPartitions,
				required); err != nil {
				logger.Warn("kafka ensure topics failed", slog.Any("err", err))
			} else {
				logger.Info("kafka topics ready")
			}
			topicCancel()
			adm.Close()
		}
		kProducer = fkafka.NewProducer(kClient, cfg.Kafka.TopicsPrefix)
		kConsumer = fkafka.NewConsumer(cfg.Kafka, logger)
		logger.Info("kafka connected")
	}

	// --- Metrics ------------------------------------------------------------
	m := fmetrics.New()

	// --- In-proc event bus + WebSocket hub ---------------------------------
	inproc := events.NewInProc()
	hub := fws.NewHub(logger)
	fws.RegisterEventBridge(inproc, hub, logger)

	// --- Providers ----------------------------------------------------------
	// A single default WhatsApp adapter serves the STATELESS operations
	// (ParseWebhook + verify signature). SendMessage requires per-integration
	// credentials — those go through providerRegistry below, which
	// constructs a fresh Provider bound to the integration's secrets.
	defaultWA := whatsapp.New(whatsapp.Config{})
	webhook.RegisterProvider("whatsapp", defaultWA)

	// providerLookup is the ChannelProviderLookup consumed by InboundService.
	providerLookup := func(key string) (channel.Provider, bool) {
		return webhook.ProviderLookup(key)
	}

	// providerRegistry builds a per-integration Provider using decrypted
	// secrets. Owned here in cmd/server because it is the only package
	// allowed to import concrete provider adapters.
	provRegistry := providerRegistryFunc(func(_ context.Context, key string, secrets map[string]string) (channel.Provider, error) {
		if key != "whatsapp" {
			return nil, fmt.Errorf("provider %q not supported yet", key)
		}
		return whatsapp.New(whatsapp.Config{
			PhoneNumberID: secrets["phone_number_id"],
			WABAID:        secrets["waba_id"],
			AccessToken:   secrets["access_token"],
			AppSecret:     secrets["app_secret"],
		}), nil
	})

	// providerResolver dispatches integration.Service.Test to the adapter
	// via HealthCheck.
	provResolver := providerResolverFunc(func(pctx context.Context, i dintegration.Integration, secrets map[string]string) (channel.Provider, error) {
		return provRegistry.Channel(pctx, i.Provider, secrets)
	})

	// --- Application services ----------------------------------------------
	// For events: prefer durable Kafka when available, but the in-proc bus
	// still feeds the WebSocket hub. We publish to both by composing them.
	pub := combinedPublisher{inproc: inproc, kafka: kProducer}

	// Enqueuer: only wire if Kafka is up. Otherwise the send / webhook
	// endpoints will return errors on the async paths — captured by /readyz.
	var enq queue.Enqueuer = disabledEnqueuer{}
	if kProducer != nil {
		enq = kProducer
	}

	inbound := appmsg.NewInboundService(appmsg.Deps{
		Integrations:       ingressSecretsAdapter{Integrations: integrations},
		WebhookEvents:      webhookEvents,
		BusinessEndpoints:  businessEndpoints,
		Contacts:           contacts,
		Identities:         identities,
		Sessions:           commSessions,
		Conversations:      conversations,
		Messages:           messages,
		MessageStatusByPMI: messages,
		Bus:                pub,
		LookupProvider:     providerLookup,
		NewID:              func() string { return ulid.Make().String() },
		Now:                func() time.Time { return time.Now().UTC() },
	})

	sendSvc := appmsg.NewSendService(appmsg.SendDeps{
		Messages:      messages,
		Conversations: conversations,
		Sessions:      commSessions,
		Endpoints:     businessEndpoints,
		Contacts:      contacts,
		Integrations:  integrations,
		Enqueuer:      enq,
		Publisher:     pub,
		Providers:     provRegistry,
		IDs:           idGenerator{},
		Clock:         systemClock{},
		Logger:        logger,
	})

	integSvc := appintegration.NewService(appintegration.Deps{
		Repo:      integrations,
		Secrets:   integrations,
		Endpoints: businessEndpoints,
		Resolver:  provResolver,
		IDs:       idGenerator{},
	})

	authSvc := appauth.NewService(users, sessionStore, perms.AsTyped(), cfg.Auth.SessionTTL)

	// --- Webhook ingress ----------------------------------------------------
	ingress := &webhook.Ingress{
		Integrations: integrations,
		Events:       webhookEvents,
		Enqueuer:     enq,
		Verifiers: webhook.StaticVerifierLookup(map[string]webhook.SignatureVerifier{
			"whatsapp": webhook.SignatureVerifierFunc(whatsapp.VerifySignature),
		}),
		Logger: logger,
		Now:    func() time.Time { return time.Now().UTC() },
	}

	// --- Cookie options -----------------------------------------------------
	secure := cfg.Env == "prod" || cfg.Env == "staging"
	cookieOpts := infauth.CookieOptions{Path: "/", MaxAge: cfg.Auth.SessionTTL, Secure: secure, SameSite: http.SameSiteLaxMode}
	csrfOpts := infauth.CookieOptions{Path: "/", MaxAge: cfg.Auth.SessionTTL, Secure: secure, SameSite: http.SameSiteLaxMode}

	// --- HTTP server + Mount -----------------------------------------------
	srv := fhttp.NewServer(cfg.HTTP, logger)

	v1.Mount(srv, v1.Deps{
		Auth: v1.AuthDeps{
			Service:       authSvc,
			Sessions:      sessionStore,
			CookieOpts:    cookieOpts,
			CSRFOpts:      csrfOpts,
			SessionCookie: cfg.Auth.SessionCookieName,
			CSRFCookie:    cfg.Auth.CSRFCookieName,
			Logger:        logger,
			Orgs:          orgs,
			Users:         users,
		},
		Webhook: v1.WebhookDeps{Ingress: ingress},
		Integrations: v1.IntegrationsDeps{
			Service:       integSvc,
			PublicBaseURL: publicBaseURL(cfg),
			Logger:        logger,
		},
		Messages: v1.MessagesDeps{
			Send:                      sendSvc,
			Messages:                  messages,
			Conversations:             conversationsLister{repo: conversations},
			IncludeConversationsIndex: true,
			Logger:                    logger,
		},
		PermissionResolver: perms,
		Logger:             logger,
		SlideEvery:         5 * time.Minute,
		Hub:                hub,
	})

	// --- Health probes ------------------------------------------------------
	srv.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	probes := []health.Probe{health.MySQLProbe(db), health.RedisProbe(rdb)}
	if len(cfg.Kafka.Brokers) > 0 {
		probes = append(probes, health.KafkaProbe(cfg.Kafka.Brokers))
	}
	srv.Handle("GET /readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, results := health.Ready(r.Context(), 2*time.Second, probes)
		status := http.StatusOK
		if !ok {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": statusStr(ok),
			"probes": results,
		})
	}))
	srv.Handle("GET /metrics", m.Handler())

	// --- Workers ------------------------------------------------------------
	pools := []*workers.Pool{}
	if kConsumer != nil {
		webhookWorker := workers.NewWebhookWorker(inbound, logger)
		sendWorker := workers.NewSendWorker(sendSvc, logger)

		webhookPool := &workers.Pool{
			Name:        "webhook.process",
			Concurrency: cfg.Workers.WebhookProcess.EffectiveConcurrency(),
			Runner:      workers.RunnerFunc(func(c context.Context) error { return webhookWorker.Run(c, kConsumer, cfg.Kafka.ClientID+"-webhook") }),
			Log:         logger,
		}
		sendPool := &workers.Pool{
			Name:        "message.send",
			Concurrency: cfg.Workers.MessageSend.EffectiveConcurrency(),
			Runner:      workers.RunnerFunc(func(c context.Context) error { return sendWorker.Run(c, kConsumer, cfg.Kafka.ClientID+"-send") }),
			Log:         logger,
		}
		pools = append(pools, webhookPool, sendPool)
		for _, p := range pools {
			go func(p *workers.Pool) {
				if err := p.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("worker pool exited", slog.String("pool", p.Name), slog.Any("err", err))
				}
			}(p)
		}
		logger.Info("worker pools started", slog.Int("count", len(pools)))
	}

	// --- Listen -------------------------------------------------------------
	logger.Info("http listen", slog.String("addr", cfg.HTTP.Addr))
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	}

	// --- Graceful shutdown (reverse of open order) --------------------------
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown", slog.Any("err", err))
	}
	if hub != nil {
		hub.Close(shutdownCtx)
	}
	if kClient != nil {
		fkafka.Close(kClient)
	}
	if err := rdb.Close(); err != nil {
		logger.Error("redis close", slog.Any("err", err))
	}
	if err := db.Close(); err != nil {
		logger.Error("mysql close", slog.Any("err", err))
	}
	logger.Info("bye")
	return nil
}

// --- Helpers --------------------------------------------------------------

// statusStr renders a boolean readiness as a stable string.
func statusStr(ok bool) string {
	if ok {
		return "ready"
	}
	return "not_ready"
}

func newLogger(l config.LogConfig) *slog.Logger {
	level := slog.LevelInfo
	switch l.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level, AddSource: l.AddSource}
	if l.Format == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// publicBaseURL returns the URL prefix operators use to build webhook URLs.
func publicBaseURL(cfg config.Config) string {
	if cfg.HTTP.Addr == "" {
		return "http://localhost:8080"
	}
	return "http://localhost" + cfg.HTTP.Addr
}

// --- Interface adapters -----------------------------------------------------

// ingressSecretsAdapter narrows the mysql Integrations repo down to the two
// methods the inbound message service consumes without pulling in the full
// mutation surface.
type ingressSecretsAdapter struct {
	*fmysql.Integrations
}

// GetWithSecrets satisfies appmsg.IntegrationSecretsRepo — a single-arg
// lookup that derives org from the row. The underlying MySQL impl requires
// (orgID, id); we pass empty orgID and the schema PK on (id) does the rest.
func (a ingressSecretsAdapter) GetWithSecrets(ctx context.Context, id dintegration.ID) (dintegration.Integration, map[string]string, error) {
	return a.Integrations.GetWithSecrets(ctx, "", id)
}

// providerRegistryFunc adapts a closure into appmsg.ProviderRegistry.
type providerRegistryFunc func(ctx context.Context, key string, secrets map[string]string) (channel.Provider, error)

// Channel implements ProviderRegistry.
func (f providerRegistryFunc) Channel(ctx context.Context, key string, secrets map[string]string) (channel.Provider, error) {
	return f(ctx, key, secrets)
}

// providerResolverFunc adapts a closure into integration.ProviderResolver.
type providerResolverFunc func(ctx context.Context, i dintegration.Integration, secrets map[string]string) (channel.Provider, error)

// Resolve implements ProviderResolver.
func (f providerResolverFunc) Resolve(ctx context.Context, i dintegration.Integration, secrets map[string]string) (channel.Provider, error) {
	return f(ctx, i, secrets)
}

// combinedPublisher fans events out to both the in-proc bus (fast subscribers
// like the WebSocket bridge) and the durable Kafka log (cross-node
// subscribers + workers). Nil kafka means "in-proc only".
type combinedPublisher struct {
	inproc *events.InProc
	kafka  *fkafka.Producer
}

// Publish forwards to both channels. Kafka errors log but don't abort the
// in-proc dispatch (real-time UI updates still fire).
func (c combinedPublisher) Publish(ctx context.Context, evt devents.Envelope) error {
	if c.kafka != nil {
		if err := c.kafka.Publish(ctx, evt); err != nil {
			slog.Default().Warn("kafka publish failed", slog.Any("err", err), slog.String("type", string(evt.Type)))
		}
	}
	return c.inproc.Publish(ctx, evt)
}

// idGenerator adapts ulid.Make into appmsg.IDGenerator + integration.IDGenerator.
type idGenerator struct{}

// NewMessageID mints a ULID for outbound messages.
func (idGenerator) NewMessageID() string { return ulid.Make().String() }

// NewID mints a ULID for integrations.
func (idGenerator) NewID() dintegration.ID { return dintegration.ID(ulid.Make().String()) }

// systemClock is time.Now().UTC() as a Clock port.
type systemClock struct{}

// Now returns the current wall-clock time in UTC.
func (systemClock) Now() time.Time { return time.Now().UTC() }

// disabledEnqueuer returns an error on every enqueue — used when Kafka is not
// wired locally. Endpoints depending on it surface the error to the caller.
type disabledEnqueuer struct{}

// Enqueue always fails; caller propagates as 5xx.
func (disabledEnqueuer) Enqueue(_ context.Context, _ queue.Job) (string, error) {
	return "", errors.New("queue disabled: kafka is not connected")
}

// conversationsLister adapts *mysql.Conversations to v1.ConversationsLister
// without pushing the infra type into the API package.
type conversationsLister struct{ repo *fmysql.Conversations }

// ListConversations implements v1.ConversationsLister.
func (c conversationsLister) ListConversations(ctx context.Context, orgID string) ([]v1.ConversationSummary, error) {
	rows, err := c.repo.ListForOrg(ctx, organization.ID(orgID))
	if err != nil {
		return nil, err
	}
	out := make([]v1.ConversationSummary, 0, len(rows))
	for _, r := range rows {
		var lastAt *string
		if r.LastMessageAt != nil {
			s := r.LastMessageAt.UTC().Format(time.RFC3339)
			lastAt = &s
		}
		out = append(out, v1.ConversationSummary{
			ID:                 string(r.ID),
			OrgID:              string(r.OrgID),
			ContactID:          r.ContactID,
			ContactName:        r.ContactDisplay,
			Status:             string(r.Status),
			Channel:            r.Channel,
			LastMessageAt:      lastAt,
			LastMessagePreview: r.LastMessagePreview,
			UnreadCount:        r.UnreadCount,
			CreatedAt:          r.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}
