// Command nudgeway is the single-binary entrypoint for the Nudgeway platform.
//
// It boots the HTTP server, wires MySQL + Redis + Kafka, builds every
// application service, mounts the v1 REST API + webhook ingress + WebSocket
// hub, starts background worker pools, registers Prometheus metrics, and
// shuts everything down gracefully on SIGINT / SIGTERM.
//
// Configuration is loaded from config/local.yaml (or NUDGEWAY_CONFIG) with
// NUDGEWAY_* env vars overriding individual keys.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/tsuna/gohbase"

	v1 "github.com/v-senthil/nudgeway/internal/api/rest/v1"
	"github.com/v-senthil/nudgeway/internal/infrastructure/attachments"
	fhbase "github.com/v-senthil/nudgeway/internal/infrastructure/hbase"
	attachmentsPort "github.com/v-senthil/nudgeway/internal/ports/attachments"
	appanalytics "github.com/v-senthil/nudgeway/internal/application/analytics"
	appapitoken "github.com/v-senthil/nudgeway/internal/application/apitoken"
	appapitokenusage "github.com/v-senthil/nudgeway/internal/application/apitokenusage"
	appaudit "github.com/v-senthil/nudgeway/internal/application/audit"
	appauth "github.com/v-senthil/nudgeway/internal/application/auth"
	appcall "github.com/v-senthil/nudgeway/internal/application/call"
	appgrp "github.com/v-senthil/nudgeway/internal/application/group"
	appintegration "github.com/v-senthil/nudgeway/internal/application/integration"
	appsettings "github.com/v-senthil/nudgeway/internal/application/integrationsettings"
	appmetaanalytics "github.com/v-senthil/nudgeway/internal/application/metaanalytics"
	appmsg "github.com/v-senthil/nudgeway/internal/application/message"
	appproviderc "github.com/v-senthil/nudgeway/internal/application/providercall"
	apptmpl "github.com/v-senthil/nudgeway/internal/application/template"
	dapitoken "github.com/v-senthil/nudgeway/internal/domain/apitoken"
	dapitokenusage "github.com/v-senthil/nudgeway/internal/domain/apitokenusage"
	dpc "github.com/v-senthil/nudgeway/internal/domain/providercall"
	devents "github.com/v-senthil/nudgeway/internal/domain/events"
	dintegration "github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	tmpldom "github.com/v-senthil/nudgeway/internal/domain/template"
	"github.com/v-senthil/nudgeway/internal/events"
	infauth "github.com/v-senthil/nudgeway/internal/infrastructure/auth"
	"github.com/v-senthil/nudgeway/internal/infrastructure/config"
	"github.com/v-senthil/nudgeway/internal/infrastructure/crypto"
	"github.com/v-senthil/nudgeway/internal/infrastructure/health"
	fhttp "github.com/v-senthil/nudgeway/internal/infrastructure/http"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
	fkafka "github.com/v-senthil/nudgeway/internal/infrastructure/kafka"
	fmetrics "github.com/v-senthil/nudgeway/internal/infrastructure/metrics"
	fmysql "github.com/v-senthil/nudgeway/internal/infrastructure/mysql"
	fredis "github.com/v-senthil/nudgeway/internal/infrastructure/redis"
	fws "github.com/v-senthil/nudgeway/internal/infrastructure/websocket"
	"github.com/v-senthil/nudgeway/internal/ports/calling"
	"github.com/v-senthil/nudgeway/internal/ports/channel"
	"github.com/v-senthil/nudgeway/internal/ports/queue"
	"github.com/v-senthil/nudgeway/internal/providers/whatsapp"
	"github.com/v-senthil/nudgeway/internal/webhook"
	"github.com/v-senthil/nudgeway/internal/workers"
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
	cfgPath := flag.String("config", envOr("NUDGEWAY_CONFIG", "config/local.yaml"), "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Log)
	slog.SetDefault(logger)
	logger.Info("nudgeway starting",
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
	auditRepo := fmysql.NewAudit(db)
	providerCallRepo := fmysql.NewProviderCalls(db)
	templatesRepo := fmysql.NewTemplates(db)
	groupsRepo := fmysql.NewGroups(db)
	callsRepo := fmysql.NewCalls(db)
	analyticsRepo := fmysql.NewAnalytics(db)
	analyticsSourceRepo := fmysql.NewAnalyticsSource(db)
	apiTokensRepo := fmysql.NewAPITokens(db)
	apiTokenUsageRepo := fmysql.NewAPITokenUsage(db)

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

	// --- Attachment store: prefer HBase; fall back to local FS -------------
	var attachStore attachmentsPort.Store
	var hbClient gohbase.Client
	if len(cfg.HBase.ZookeeperQuorum) > 0 {
		zkNode := cfg.HBase.ZNodeParent
		if zkNode == "" {
			zkNode = "/hbase"
		}
		// gohbase's CreateTable RPC doesn't split "namespace:table" the way
		// HBase server expects, so use the default namespace for now with a
		// prefixed table name. Real namespaces need shell pre-creation +
		// gohbase upgrade or a raw admin call.
		ns := ""
		tbl := "nudgeway_attachments"
		client, admin, hErr := fhbase.NewClient(cfg.HBase.ZookeeperQuorum, zkNode)
		if hErr != nil {
			logger.Warn("hbase connect failed; falling back to local FS attachments", slog.Any("err", hErr))
		} else {
			schemaCtx, schemaCancel := context.WithTimeout(ctx, 15*time.Second)
			if err := fhbase.EnsureSchema(schemaCtx, admin, ns, tbl); err != nil {
				logger.Warn("hbase schema ensure failed; falling back to local FS attachments", slog.Any("err", err))
			} else {
				fq := tbl
				if ns != "" {
					fq = ns + ":" + tbl
				}
				attachStore = fhbase.NewAttachments(client, fq)
				hbClient = client
				logger.Info("hbase attachments ready", slog.String("table", fq))
			}
			schemaCancel()
		}
	}
	if attachStore == nil {
		attachRoot := cfg.Attachments.Root
		if attachRoot == "" {
			attachRoot = "./attachments"
		}
		lfs, err := attachments.New(attachments.Config{Root: attachRoot})
		if err != nil {
			return fmt.Errorf("attachments: %w", err)
		}
		attachStore = lfs
		logger.Info("attachment store (localfs) ready", slog.String("root", attachRoot))
	}

	// --- In-proc event bus + WebSocket hub ---------------------------------
	inproc := events.NewInProc()
	hub := fws.NewHub(logger)
	fws.RegisterEventBridge(inproc, hub, logger)

	// --- Audit + provider-call execution logs -------------------------------
	// Applications construct once and share; Record is fire-and-forget.
	auditSvc := appaudit.New(appaudit.Deps{Repo: auditRepo, Logger: logger})
	providerCallSvc := appproviderc.NewService(appproviderc.Deps{Repo: providerCallRepo, Logger: logger})

	// waTracer bridges whatsapp.TraceEvent → providercall.Entry so every
	// outbound Meta call becomes a persisted provider_calls row. The
	// bridge is provider-agnostic: it stamps Provider="whatsapp" on
	// entries and lets the application service handle body truncation.
	waTracer := whatsappTracer{svc: providerCallSvc}

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
	//
	// Reserved keys `_integration_id` and `_org_id` in the secrets bag
	// carry wire-up metadata that upstream call sites (send, read,
	// attachment download, integration test) inject so the tracer can
	// tag every emitted execution-log row without changing the
	// ProviderRegistry port signature.
	provRegistry := providerRegistryFunc(func(_ context.Context, key string, secrets map[string]string) (channel.Provider, error) {
		if key != "whatsapp" {
			return nil, fmt.Errorf("provider %q not supported yet", key)
		}
		integID := secrets["_integration_id"]
		orgID := secrets["_org_id"]
		waP := whatsapp.New(whatsapp.Config{
			PhoneNumberID: secrets["phone_number_id"],
			WABAID:        secrets["waba_id"],
			AccessToken:   secrets["access_token"],
			AppSecret:     secrets["app_secret"],
		}).WithTracer(waTracer, integID, orgID)
		// Wrap so type assertions like prov.(appgrp.ProviderGroupsClient)
		// succeed. The wrapper embeds *whatsapp.Provider for the
		// channel.Provider surface and overrides the group methods to
		// return the app-level DTO shapes.
		return &whatsappChannelProvider{Provider: waP}, nil
	})

	// providerResolver dispatches integration.Service.Test to the adapter
	// via HealthCheck.
	provResolver := providerResolverFunc(func(pctx context.Context, i dintegration.Integration, secrets map[string]string) (channel.Provider, error) {
		if secrets == nil {
			secrets = map[string]string{}
		}
		secrets["_integration_id"] = string(i.ID)
		secrets["_org_id"] = string(i.OrgID)
		return provRegistry.Channel(pctx, i.Provider, secrets)
	})

	// attachmentDownloader closes over the mysql Integrations repo + provider
	// registry so the InboundService can fetch inbound media without
	// importing any provider adapter directly.
	attachDownloader := attachmentDownloaderFunc(func(dctx context.Context, providerKey string, integID dintegration.ID, mediaID, mediaURL string) (io.ReadCloser, string, error) {
		row, secrets, err := integrations.GetWithSecrets(dctx, "", integID)
		if err != nil {
			return nil, "", fmt.Errorf("attachment download: load integration: %w", err)
		}
		if v, ok := row.Config["phone_number_id"].(string); ok {
			secrets["phone_number_id"] = v
		}
		if v, ok := row.Config["waba_id"].(string); ok {
			secrets["waba_id"] = v
		}
		secrets["_integration_id"] = string(row.ID)
		secrets["_org_id"] = string(row.OrgID)
		p, err := provRegistry.Channel(dctx, providerKey, secrets)
		if err != nil {
			return nil, "", fmt.Errorf("attachment download: resolve provider: %w", err)
		}
		waP, ok := p.(*whatsapp.Provider)
		if !ok {
			// provRegistry wraps the concrete Provider in
			// whatsappChannelProvider for the group DTO adapters. Unwrap
			// so the media-download path (recording + transcript + inbound
			// message attachments) still works.
			if w, wok := p.(*whatsappChannelProvider); wok {
				waP = w.Provider
				ok = true
			}
		}
		if !ok {
			return nil, "", fmt.Errorf("attachment download: provider %q does not support media", providerKey)
		}
		// Prefer the URL Meta gave us in the webhook envelope — one HTTPS
		// call instead of two. Fall back to the ID-based lookup path when
		// only the mediaID is known.
		if mediaURL != "" {
			return waP.DownloadMediaByURL(dctx, mediaURL)
		}
		return waP.DownloadMedia(dctx, mediaID)
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
		Attachments:        attachStore,
		Downloader:         attachDownloader,
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
		Identities:    identities,
		Integrations:  integrations,
		Templates:     templatesRepo,
		Enqueuer:      enq,
		Publisher:     pub,
		Providers:     provRegistry,
		IDs:           idGenerator{},
		Clock:         systemClock{},
		Logger:        logger,
	})

	readSvc := appmsg.NewReadService(appmsg.ReadDeps{
		Messages:      messages,
		Conversations: conversations,
		Sessions:      commSessions,
		Endpoints:     businessEndpoints,
		Integrations:  integrations,
		Providers:     provRegistry,
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

	// callingRegistry resolves a calling.Provider bound to an integration's
	// decrypted secrets. Parallels provRegistry above — same whatsapp.Provider
	// backing store, same tracer wiring for provider_calls telemetry.
	callingRegistry := callingRegistryFunc(func(_ context.Context, key string, secrets map[string]string) (calling.Provider, error) {
		if key != "whatsapp" {
			return nil, fmt.Errorf("calling provider %q not supported", key)
		}
		integID := secrets["_integration_id"]
		orgID := secrets["_org_id"]
		waP := whatsapp.New(whatsapp.Config{
			PhoneNumberID: secrets["phone_number_id"],
			WABAID:        secrets["waba_id"],
			AccessToken:   secrets["access_token"],
			AppSecret:     secrets["app_secret"],
		}).WithTracer(waTracer, integID, orgID)
		return waP.CallingProvider(), nil
	})

	// templateProviderRegistry adapts the whatsapp adapter to
	// apptmpl.ProviderRegistry — bridges the map[string]any component shape
	// on the wire with the domain []tmpldom.Component shape the application
	// service exchanges. JSON round-trip keeps unknown provider fields
	// preserved through the Extra map.
	templateProviderRegistry := templateRegistryFunc(func(_ context.Context, key string, _ dintegration.Integration, secrets map[string]string) (apptmpl.TemplateProvider, error) {
		if key != "whatsapp" {
			return nil, fmt.Errorf("template provider %q not supported", key)
		}
		integID := secrets["_integration_id"]
		orgID := secrets["_org_id"]
		waP := whatsapp.New(whatsapp.Config{
			PhoneNumberID: secrets["phone_number_id"],
			WABAID:        secrets["waba_id"],
			AccessToken:   secrets["access_token"],
			AppSecret:     secrets["app_secret"],
		}).WithTracer(waTracer, integID, orgID)
		return whatsappTemplateAdapter{p: waP}, nil
	})

	templateSvc := apptmpl.NewService(apptmpl.Deps{
		Repo:         templatesRepo,
		Integrations: integrations,
		Providers:    templateProviderRegistry,
		IDs:          idGenerator{},
		Clock:        systemClock{},
		Logger:       logger,
	})

	groupSvc := appgrp.NewService(appgrp.Deps{
		Repo:         groupsRepo,
		Integrations: integrations,
		Providers:    provRegistry,
		// Send is nil: SendToGroup is not implemented in the message
		// pipeline yet; List / Get / Sync remain functional. Attempts to
		// call SendMessage return an explicit "send service not wired".
		Send:          nil,
		Conversations: conversations,
		Clock:         systemClock{},
		Logger:        logger,
	})

	callSvc := appcall.New(appcall.Deps{
		Repo:             callsRepo,
		Contacts:         contacts,
		Sessions:         commSessions,
		Conversations:    conversations,
		Endpoints:        businessEndpoints,
		Integrations:     integrations,
		CallingProviders: callingRegistry,
		Publisher:        pub,
		IDs:              idGenerator{},
		Clock:            systemClock{},
		Logger:           logger,
		Attachments:      attachStore,
		Downloader:       callAttachmentDownloader{fn: attachDownloader},
		Messages:         messages,
		MessageIDs:       idGenerator{},
		Identities:       identities,
		ContactIDs:       idGenerator{},
	})

	// Stitch the calling application service into the inbound webhook
	// dispatch loop so Call* envelopes emitted by the whatsapp adapter's
	// ParseCallWebhook path reach the DB + get republished for the WS
	// bridge. The inbound service is nil-safe on this dep; wiring it here
	// after both services exist keeps the boot order simple.
	inbound.SetCallInbound(callSvc)

	analyticsSvc := appanalytics.New(appanalytics.Deps{
		Repo:   analyticsRepo,
		Raw:    analyticsSourceRepo,
		Logger: logger,
	})

	// integrationSettingsResolver builds a settings ProviderClient for the
	// integration. Mirrors provRegistry — same whatsapp.Provider backing
	// store, same tracer wiring so provider_calls captures every business
	// profile / call settings / OBA round-trip.
	settingsResolver := integrationSettingsResolverFunc(func(_ context.Context, key string, integ dintegration.Integration, secrets map[string]string) (appsettings.ProviderClient, error) {
		if key != "whatsapp" {
			return nil, fmt.Errorf("settings provider %q not supported", key)
		}
		integID := secrets["_integration_id"]
		orgID := secrets["_org_id"]
		waP := whatsapp.New(whatsapp.Config{
			PhoneNumberID: secrets["phone_number_id"],
			WABAID:        secrets["waba_id"],
			AccessToken:   secrets["access_token"],
			AppSecret:     secrets["app_secret"],
		}).WithTracer(waTracer, integID, orgID)
		return whatsappSettingsAdapter{p: waP}, nil
	})

	// callPermissionAdapter bridges the settings drawer's optional
	// permission-lookup port to the calling application service. Kept as an
	// inline closure so integrationsettings stays free of a direct import
	// on ports/calling.
	callPermissionAdapter := callPermissionLookupFunc(func(ctx context.Context, orgID organization.ID, id dintegration.ID, waID string) (appsettings.CallPermission, error) {
		pm, err := callSvc.GetPermission(ctx, orgID, id, waID)
		if err != nil {
			return appsettings.CallPermission{}, err
		}
		return appsettings.CallPermission{Status: pm.Status, ExpirationTime: pm.ExpirationTime}, nil
	})

	settingsSvc := appsettings.NewService(appsettings.Deps{
		Integrations: integrations,
		Providers:    settingsResolver,
		Permissions:  callPermissionAdapter,
		Logger:       logger,
	})

	// metaAnalyticsResolver builds a Meta-analytics ProviderClient for
	// the integration. Mirrors settingsResolver — same whatsapp.Provider
	// backing store, same tracer wiring so provider_calls captures every
	// Meta analytics round-trip.
	metaAnalyticsResolver := metaAnalyticsResolverFunc(func(_ context.Context, key string, integ dintegration.Integration, secrets map[string]string) (appmetaanalytics.MetaAnalyticsProvider, error) {
		if key != "whatsapp" {
			return nil, fmt.Errorf("meta analytics provider %q not supported", key)
		}
		integID := secrets["_integration_id"]
		orgID := secrets["_org_id"]
		waP := whatsapp.New(whatsapp.Config{
			PhoneNumberID: secrets["phone_number_id"],
			WABAID:        secrets["waba_id"],
			AccessToken:   secrets["access_token"],
			AppSecret:     secrets["app_secret"],
		}).WithTracer(waTracer, integID, orgID)
		return whatsappMetaAnalyticsAdapter{p: waP}, nil
	})

	metaAnalyticsSvc := appmetaanalytics.NewService(appmetaanalytics.Deps{
		Integrations: integrations,
		Providers:    metaAnalyticsResolver,
		Logger:       logger,
	})

	// --- API tokens ---------------------------------------------------------
	// Long-lived programmatic-access tokens (MCP server, CI, scripts).
	// The service wraps the mysql repo and the shared argon2id helper.
	apiTokenSvc := appapitoken.NewService(appapitoken.Deps{
		Repo:   apiTokensRepo,
		Hasher: argon2Hasher{p: infauth.DefaultArgon2Params()},
	})
	bearerVerifier := bearerVerifierAdapter{svc: apiTokenSvc}

	// Per-bearer-request execution log. The application service handles
	// body redaction + truncation; the middleware feeds it through a
	// tiny adapter (declared below) so infrastructure stays free of
	// application imports (dependency rule).
	apiTokenUsageSvc := appapitokenusage.NewService(appapitokenusage.Deps{
		Repo:   apiTokenUsageRepo,
		Logger: logger,
	})
	tokenUsageRecorder := tokenUsageRecorderAdapter{svc: apiTokenUsageSvc}

	// --- Webhook ingress ----------------------------------------------------
	// DEV MODE: signature verification is currently disabled. Meta's App
	// Secret is not reliably configurable in this dev flow. The ingress
	// instead matches the payload's phone_number_id + waba_id against the
	// integration.Config values (see webhook.ClaimsVerifier). Never ship
	// this to prod — set NUDGEWAY_REQUIRE_SIGNATURE=1 (or config webhook
	// section) once the Meta App Secret is trusted.
	requireSig := os.Getenv("NUDGEWAY_REQUIRE_SIGNATURE") == "1"
	if !requireSig {
		logger.Warn("webhook signature verification DISABLED — dev mode, do NOT ship to prod",
			slog.String("fallback", "phone_number_id + waba_id claims match"))
	}
	ingress := &webhook.Ingress{
		Integrations: integrations,
		Events:       webhookEvents,
		Enqueuer:     enq,
		Verifiers: webhook.StaticVerifierLookup(map[string]webhook.SignatureVerifier{
			"whatsapp": webhook.SignatureVerifierFunc(whatsapp.VerifySignature),
		}),
		ClaimsVerifiers: webhook.StaticClaimsVerifierLookup(map[string]webhook.ClaimsVerifier{
			"whatsapp": webhook.ClaimsVerifierFunc(whatsapp.VerifyClaims),
		}),
		RequireSignature: requireSig,
		Logger:           logger,
		Now:              func() time.Time { return time.Now().UTC() },
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
			Read:                      readSvc,
			Messages:                  messages,
			Conversations:             conversationsLister{repo: conversations},
			IncludeConversationsIndex: true,
			Logger:                    logger,
		},
		Attachments: v1.AttachmentsDeps{
			Store:  attachStore,
			Logger: logger,
		},
		AttachmentsUpload: v1.AttachmentsUploadDeps{
			Store:         attachStore,
			MediaUploader: metaMediaUploader{
				store:        attachStore,
				integrations: integrations,
				providers:    provRegistry,
				logger:       logger,
			},
			PublicBaseURL: publicBaseURL(cfg),
			Logger:        logger,
		},
		Audit: v1.AuditDeps{
			Service: auditSvc,
			Logger:  logger,
		},
		ProviderCalls: v1.ProviderCallsDeps{
			Service: providerCallSvc,
			Logger:  logger,
		},
		Templates: v1.TemplateDeps{
			Service: templateSvc,
			Logger:  logger,
		},
		Groups: v1.GroupsDeps{
			Service: groupSvc,
			Logger:  logger,
		},
		Calls: v1.CallsDeps{
			Service: callSvc,
			Audit:   auditSvc,
			Logger:  logger,
		},
		Analytics: v1.AnalyticsDeps{
			Service: analyticsSvc,
			Logger:  logger,
		},
		IntegrationSettings: v1.IntegrationSettingsDeps{
			Service: settingsSvc,
			Audit:   auditSvc,
			Logger:  logger,
		},
		MetaAnalytics: v1.MetaAnalyticsDeps{
			Service: metaAnalyticsSvc,
			Logger:  logger,
		},
		APITokens: v1.APITokensDeps{
			Service: apiTokenSvc,
			Logger:  logger,
		},
		APITokenUsage: v1.APITokenUsageDeps{
			Service: apiTokenUsageSvc,
			Logger:  logger,
		},
		BearerVerifier:     bearerVerifier,
		TokenUsageRecorder: tokenUsageRecorder,
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

	// --- Analytics rollup runner -------------------------------------------
	// The runner ticks every 15 minutes, re-rolling yesterday + today per
	// org so late-arriving webhook statuses eventually converge. Its loop
	// blocks on the returned ctx; shutdown cancels the parent ctx which
	// propagates and exits the goroutine cleanly.
	analyticsRunner := workers.NewAnalyticsRollupRunner(workers.AnalyticsRollupDeps{
		Service:  analyticsSvc,
		Orgs:     orgLister{db: db},
		Repo:     analyticsRepo,
		Logger:   logger,
		Interval: 15 * time.Minute,
	})
	go func() {
		if err := analyticsRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("analytics rollup exited", slog.Any("err", err))
		}
	}()

	// --- API-token usage rollup runner -------------------------------------
	// Same shape as the analytics rollup: tick every 15 minutes, re-roll
	// yesterday + today + tomorrow per org so late-arriving requests
	// converge into the correct api_token_usage_daily row.
	apiTokenUsageRunner := workers.NewAPITokenUsageRollupRunner(workers.APITokenUsageRollupDeps{
		Repo:     apiTokenUsageRepo,
		Orgs:     orgLister{db: db},
		Logger:   logger,
		Interval: 15 * time.Minute,
	})
	go func() {
		if err := apiTokenUsageRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("api-token usage rollup exited", slog.Any("err", err))
		}
	}()

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
	if hbClient != nil {
		fhbase.Close(hbClient)
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

// metaMediaUploader implements v1.MediaUploader by picking the org's first
// channel integration and handing the stored bytes to that provider via
// its adapter. The upload is best-effort: failures do NOT bounce the
// local Put — media_url still works as a fallback for send.
type metaMediaUploader struct {
	store        attachmentsPort.Store
	integrations *fmysql.Integrations
	providers    providerRegistryFunc
	logger       *slog.Logger
}

// Upload implements v1.MediaUploader. It resolves the first channel-kind
// integration for orgID, opens the stored blob, hands it to the provider,
// and stashes the returned mediaID via SetMediaID when the store supports
// it.
func (u metaMediaUploader) Upload(ctx context.Context, orgID, key, contentType, filename string) (string, string, string, error) {
	if u.integrations == nil {
		return "", "", "", fmt.Errorf("meta upload: integrations repo not wired")
	}
	list, err := u.integrations.List(ctx, organization.ID(orgID))
	if err != nil {
		return "", "", "", fmt.Errorf("meta upload: list integrations: %w", err)
	}
	var integ dintegration.Integration
	found := false
	for _, i := range list {
		if i.Type == dintegration.TypeChannel && i.Provider == "whatsapp" {
			integ = i
			found = true
			break
		}
	}
	if !found {
		return "", "", "", fmt.Errorf("meta upload: no whatsapp integration for org %s", orgID)
	}
	row, secrets, err := u.integrations.GetWithSecrets(ctx, organization.ID(orgID), integ.ID)
	if err != nil {
		return "", "", "", fmt.Errorf("meta upload: load secrets: %w", err)
	}
	if v, ok := row.Config["phone_number_id"].(string); ok {
		secrets["phone_number_id"] = v
	}
	if v, ok := row.Config["waba_id"].(string); ok {
		secrets["waba_id"] = v
	}
	secrets["_integration_id"] = string(row.ID)
	secrets["_org_id"] = string(row.OrgID)
	p, err := u.providers.Channel(ctx, row.Provider, secrets)
	if err != nil {
		return "", "", "", fmt.Errorf("meta upload: resolve provider: %w", err)
	}
	waP, ok := p.(*whatsapp.Provider)
	if !ok {
		if w, wok := p.(*whatsappChannelProvider); wok {
			waP = w.Provider
			ok = true
		}
	}
	if !ok {
		return "", "", "", fmt.Errorf("meta upload: provider %q does not support upload", row.Provider)
	}
	body, err := u.store.Get(ctx, key)
	if err != nil {
		return "", "", "", fmt.Errorf("meta upload: open blob: %w", err)
	}
	defer func() { _ = body.Close() }()
	mediaID, err := waP.UploadMedia(ctx, contentType, filename, body)
	if err != nil {
		return "", "", "", fmt.Errorf("meta upload: %w", err)
	}
	// Best-effort persist of the media_id alongside the blob so re-uploads
	// aren't needed on re-send.
	if setter, ok := u.store.(interface {
		SetMediaID(ctx context.Context, key, providerKey, integrationID, mediaID string) error
	}); ok {
		if err := setter.SetMediaID(ctx, key, row.Provider, string(integ.ID), mediaID); err != nil {
			u.logger.Warn("meta upload: SetMediaID failed",
				slog.Any("err", err),
				slog.String("key", key),
			)
		}
	}
	return row.Provider, string(integ.ID), mediaID, nil
}

// whatsappTracer bridges whatsapp.Tracer.OnCall into the provider-agnostic
// providercall.Service.Record. Every outbound Meta HTTP call the adapter
// makes yields one persisted execution-log row. The bridge is thin on
// purpose — body truncation, direction defaulting, and error handling live
// in the application service.
type whatsappTracer struct{ svc *appproviderc.Service }

// OnCall implements whatsapp.Tracer.
func (t whatsappTracer) OnCall(ctx context.Context, e whatsapp.TraceEvent) {
	if t.svc == nil {
		return
	}
	t.svc.Record(ctx, dpc.Entry{
		OrgID:         e.OrgID,
		IntegrationID: e.IntegrationID,
		Provider:      "whatsapp",
		Operation:     e.Operation,
		Direction:     dpc.DirectionOutbound,
		Method:        e.Method,
		URL:           e.URL,
		StatusCode:    e.StatusCode,
		LatencyMs:     e.LatencyMs,
		RequestBody:   e.RequestBody,
		ResponseBody:  e.ResponseBody,
		ErrorClass:    e.ErrClass,
		ErrorMessage:  e.ErrMessage,
		TraceID:       e.TraceID,
		OccurredAt:    time.Now().UTC(),
	})
}

// attachmentDownloaderFunc adapts a closure into appmsg.AttachmentDownloader.
type attachmentDownloaderFunc func(ctx context.Context, providerKey string, integrationID dintegration.ID, mediaID, mediaURL string) (io.ReadCloser, string, error)

// Download implements appmsg.AttachmentDownloader.
func (f attachmentDownloaderFunc) Download(ctx context.Context, providerKey string, integrationID dintegration.ID, mediaID, mediaURL string) (io.ReadCloser, string, error) {
	return f(ctx, providerKey, integrationID, mediaID, mediaURL)
}

// callAttachmentDownloader adapts the shared attachmentDownloaderFunc onto
// the appcall.AttachmentDownloader port. The two application services keep
// independent port types per the dependency rule; the underlying closure
// is reused so recording + transcript downloads share the same integration
// lookup and provider registry as inbound message media.
type callAttachmentDownloader struct {
	fn attachmentDownloaderFunc
}

// Download implements appcall.AttachmentDownloader.
func (c callAttachmentDownloader) Download(ctx context.Context, providerKey string, integrationID dintegration.ID, mediaID, mediaURL string) (io.ReadCloser, string, error) {
	return c.fn(ctx, providerKey, integrationID, mediaID, mediaURL)
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

// Publish forwards synchronously to the in-proc bus (fast; the WebSocket
// bridge needs it) and asynchronously to Kafka. Kafka is the durable
// cross-node log — publishing must never block the caller's request path,
// because a missing topic causes franz-go to retry for 15+ seconds and
// pin REST handlers open. Errors are logged; callers never see them.
func (c combinedPublisher) Publish(ctx context.Context, evt devents.Envelope) error {
	if c.kafka != nil {
		go func(e devents.Envelope) {
			// Detach from the request ctx (short-lived); cap our own
			// timeout so a wedged broker never leaks goroutines.
			pctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := c.kafka.Publish(pctx, e); err != nil {
				slog.Default().Warn("kafka publish failed",
					slog.Any("err", err),
					slog.String("type", string(e.Type)),
				)
			}
		}(evt)
	}
	return c.inproc.Publish(ctx, evt)
}

// idGenerator adapts ulid.Make into appmsg.IDGenerator + integration.IDGenerator.
type idGenerator struct{}

// NewMessageID mints a ULID for outbound messages.
func (idGenerator) NewMessageID() string { return ulid.Make().String() }

// NewID mints a ULID for integrations.
func (idGenerator) NewID() dintegration.ID { return dintegration.ID(ulid.Make().String()) }

// NewCallID mints a ULID for calls.
func (idGenerator) NewCallID() string { return ulid.Make().String() }

// NewTemplateID mints a ULID for templates.
func (idGenerator) NewTemplateID() string { return ulid.Make().String() }

// NewContactID mints a ULID for freshly-observed callers when the call
// service bootstraps a contact off an inbound call webhook.
func (idGenerator) NewContactID() string { return ulid.Make().String() }

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
			Type:               string(r.Type),
			GroupID:            r.GroupID,
			Subject:            r.GroupSubject,
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

// callingRegistryFunc adapts a closure into appcall.CallingProviderRegistry.
type callingRegistryFunc func(ctx context.Context, key string, secrets map[string]string) (calling.Provider, error)

// Calling implements appcall.CallingProviderRegistry.
func (f callingRegistryFunc) Calling(ctx context.Context, key string, secrets map[string]string) (calling.Provider, error) {
	return f(ctx, key, secrets)
}

// templateRegistryFunc adapts a closure into apptmpl.ProviderRegistry.
type templateRegistryFunc func(ctx context.Context, key string, integ dintegration.Integration, secrets map[string]string) (apptmpl.TemplateProvider, error)

// Template implements apptmpl.ProviderRegistry.
func (f templateRegistryFunc) Template(ctx context.Context, key string, integ dintegration.Integration, secrets map[string]string) (apptmpl.TemplateProvider, error) {
	return f(ctx, key, integ, secrets)
}

// metaAnalyticsResolverFunc adapts a closure into
// appmetaanalytics.Resolver.
type metaAnalyticsResolverFunc func(ctx context.Context, key string, integ dintegration.Integration, secrets map[string]string) (appmetaanalytics.MetaAnalyticsProvider, error)

// MetaAnalytics implements appmetaanalytics.Resolver.
func (f metaAnalyticsResolverFunc) MetaAnalytics(ctx context.Context, key string, integ dintegration.Integration, secrets map[string]string) (appmetaanalytics.MetaAnalyticsProvider, error) {
	return f(ctx, key, integ, secrets)
}

// whatsappMetaAnalyticsAdapter bridges the whatsapp adapter's
// provider-native Meta analytics methods to the provider-neutral
// appmetaanalytics.MetaAnalyticsProvider port. Every method is a
// one-shot shape translation; the whatsapp package remains the sole
// owner of the Meta wire format.
type whatsappMetaAnalyticsAdapter struct {
	p *whatsapp.Provider
}

// MessagingAnalytics implements appmetaanalytics.MetaAnalyticsProvider.
func (a whatsappMetaAnalyticsAdapter) MessagingAnalytics(ctx context.Context, wabaID string, req appmetaanalytics.MessagingAnalyticsRequest) (appmetaanalytics.MessagingAnalyticsResponse, error) {
	out, err := a.p.MessagingAnalytics(ctx, wabaID, whatsapp.MessagingAnalyticsRequest{
		Start:        req.Start,
		End:          req.End,
		Granularity:  req.Granularity,
		PhoneNumbers: req.PhoneNumbers,
		ProductTypes: req.ProductTypes,
		CountryCodes: req.CountryCodes,
	})
	if err != nil {
		return appmetaanalytics.MessagingAnalyticsResponse{}, err
	}
	pts := make([]appmetaanalytics.MessagingAnalyticsDataPoint, 0, len(out.Analytics.DataPoints))
	for _, dp := range out.Analytics.DataPoints {
		pts = append(pts, appmetaanalytics.MessagingAnalyticsDataPoint{
			Start: dp.Start, End: dp.End, Sent: dp.Sent, Delivered: dp.Delivered,
		})
	}
	return appmetaanalytics.MessagingAnalyticsResponse{
		Analytics: appmetaanalytics.MessagingAnalyticsPayload{
			PhoneNumbers: out.Analytics.PhoneNumbers,
			CountryCodes: out.Analytics.CountryCodes,
			Granularity:  out.Analytics.Granularity,
			DataPoints:   pts,
		},
		ID: out.ID,
	}, nil
}

// ConversationAnalytics implements appmetaanalytics.MetaAnalyticsProvider.
func (a whatsappMetaAnalyticsAdapter) ConversationAnalytics(ctx context.Context, wabaID string, req appmetaanalytics.ConversationAnalyticsRequest) (appmetaanalytics.ConversationAnalyticsResponse, error) {
	out, err := a.p.ConversationAnalytics(ctx, wabaID, whatsapp.ConversationAnalyticsRequest{
		Start:                  req.Start,
		End:                    req.End,
		Granularity:            req.Granularity,
		PhoneNumbers:           req.PhoneNumbers,
		MetricTypes:            req.MetricTypes,
		ConversationCategories: req.ConversationCategories,
		ConversationTypes:      req.ConversationTypes,
		ConversationDirections: req.ConversationDirections,
		Dimensions:             req.Dimensions,
		CountryCodes:           req.CountryCodes,
	})
	if err != nil {
		return appmetaanalytics.ConversationAnalyticsResponse{}, err
	}
	data := make([]appmetaanalytics.ConversationAnalyticsData, 0, len(out.ConversationAnalytics.Data))
	for _, d := range out.ConversationAnalytics.Data {
		pts := make([]appmetaanalytics.ConversationAnalyticsDataPoint, 0, len(d.DataPoints))
		for _, dp := range d.DataPoints {
			pts = append(pts, appmetaanalytics.ConversationAnalyticsDataPoint{
				Start:                 dp.Start,
				End:                   dp.End,
				Conversation:          dp.Conversation,
				PhoneNumber:           dp.PhoneNumber,
				Country:               dp.Country,
				ConversationType:      dp.ConversationType,
				ConversationDirection: dp.ConversationDirection,
				ConversationCategory:  dp.ConversationCategory,
				Cost:                  dp.Cost,
			})
		}
		data = append(data, appmetaanalytics.ConversationAnalyticsData{DataPoints: pts})
	}
	return appmetaanalytics.ConversationAnalyticsResponse{
		ConversationAnalytics: appmetaanalytics.ConversationAnalyticsPayload{Data: data},
		ID:                    out.ID,
	}, nil
}

// PricingAnalytics implements appmetaanalytics.MetaAnalyticsProvider.
func (a whatsappMetaAnalyticsAdapter) PricingAnalytics(ctx context.Context, wabaID string, req appmetaanalytics.PricingAnalyticsRequest) (appmetaanalytics.PricingAnalyticsResponse, error) {
	out, err := a.p.PricingAnalytics(ctx, wabaID, whatsapp.PricingAnalyticsRequest{
		Start:             req.Start,
		End:               req.End,
		Granularity:       req.Granularity,
		PhoneNumbers:      req.PhoneNumbers,
		CountryCodes:      req.CountryCodes,
		MetricTypes:       req.MetricTypes,
		PricingTypes:      req.PricingTypes,
		PricingCategories: req.PricingCategories,
		Dimensions:        req.Dimensions,
	})
	if err != nil {
		return appmetaanalytics.PricingAnalyticsResponse{}, err
	}
	data := make([]appmetaanalytics.PricingAnalyticsData, 0, len(out.PricingAnalytics.Data))
	for _, d := range out.PricingAnalytics.Data {
		pts := make([]appmetaanalytics.PricingAnalyticsDataPoint, 0, len(d.DataPoints))
		for _, dp := range d.DataPoints {
			pts = append(pts, appmetaanalytics.PricingAnalyticsDataPoint{
				Start:           dp.Start,
				End:             dp.End,
				Country:         dp.Country,
				PhoneNumber:     dp.PhoneNumber,
				Tier:            dp.Tier,
				PricingType:     dp.PricingType,
				PricingCategory: dp.PricingCategory,
				Volume:          dp.Volume,
				Cost:            dp.Cost,
			})
		}
		data = append(data, appmetaanalytics.PricingAnalyticsData{DataPoints: pts})
	}
	return appmetaanalytics.PricingAnalyticsResponse{
		PricingAnalytics: appmetaanalytics.PricingAnalyticsPayload{Data: data},
		ID:               out.ID,
	}, nil
}

// CallAnalytics implements appmetaanalytics.MetaAnalyticsProvider.
func (a whatsappMetaAnalyticsAdapter) CallAnalytics(ctx context.Context, wabaID string, req appmetaanalytics.CallAnalyticsRequest) (appmetaanalytics.CallAnalyticsResponse, error) {
	out, err := a.p.CallAnalyticsMeta(ctx, wabaID, whatsapp.CallAnalyticsMetaRequest{
		Start:        req.Start,
		End:          req.End,
		Granularity:  req.Granularity,
		PhoneNumbers: req.PhoneNumbers,
		CountryCodes: req.CountryCodes,
		Directions:   req.Directions,
		Dimensions:   req.Dimensions,
		MetricTypes:  req.MetricTypes,
	})
	if err != nil {
		return appmetaanalytics.CallAnalyticsResponse{}, err
	}
	pts := make([]appmetaanalytics.CallAnalyticsDataPoint, 0, len(out.CallAnalytics.DataPoints))
	for _, dp := range out.CallAnalytics.DataPoints {
		pts = append(pts, appmetaanalytics.CallAnalyticsDataPoint{
			Start:           dp.Start,
			End:             dp.End,
			Count:           dp.Count,
			Cost:            dp.Cost,
			AverageDuration: dp.AverageDuration,
			PhoneNumber:     dp.PhoneNumber,
			Country:         dp.Country,
			Direction:       dp.Direction,
		})
	}
	return appmetaanalytics.CallAnalyticsResponse{
		CallAnalytics: appmetaanalytics.CallAnalyticsPayload{
			Granularity: out.CallAnalytics.Granularity,
			DataPoints:  pts,
		},
		ID: out.ID,
	}, nil
}

// TemplateAnalytics implements appmetaanalytics.MetaAnalyticsProvider.
func (a whatsappMetaAnalyticsAdapter) TemplateAnalytics(ctx context.Context, wabaID string, req appmetaanalytics.TemplateAnalyticsRequest) (appmetaanalytics.TemplateAnalyticsResponse, error) {
	out, err := a.p.TemplateAnalytics(ctx, wabaID, whatsapp.TemplateAnalyticsRequest{
		Start:           req.Start,
		End:             req.End,
		Granularity:     req.Granularity,
		TemplateIDs:     req.TemplateIDs,
		MetricTypes:     req.MetricTypes,
		ProductType:     req.ProductType,
		UseWABATimezone: req.UseWABATimezone,
	})
	if err != nil {
		return appmetaanalytics.TemplateAnalyticsResponse{}, err
	}
	buckets := make([]appmetaanalytics.TemplateAnalyticsBucket, 0, len(out.Data))
	for _, b := range out.Data {
		pts := make([]appmetaanalytics.TemplateAnalyticsDataPoint, 0, len(b.DataPoints))
		for _, dp := range b.DataPoints {
			clicks := make([]appmetaanalytics.TemplateAnalyticsClick, 0, len(dp.Clicked))
			for _, c := range dp.Clicked {
				clicks = append(clicks, appmetaanalytics.TemplateAnalyticsClick{
					Type: c.Type, ButtonContent: c.ButtonContent, Count: c.Count,
				})
			}
			costs := make([]appmetaanalytics.TemplateAnalyticsCost, 0, len(dp.Cost))
			for _, c := range dp.Cost {
				costs = append(costs, appmetaanalytics.TemplateAnalyticsCost{Type: c.Type, Value: c.Value})
			}
			pts = append(pts, appmetaanalytics.TemplateAnalyticsDataPoint{
				TemplateID: dp.TemplateID,
				Start:      dp.Start,
				End:        dp.End,
				Sent:       dp.Sent,
				Delivered:  dp.Delivered,
				Read:       dp.Read,
				Clicked:    clicks,
				Cost:       costs,
			})
		}
		buckets = append(buckets, appmetaanalytics.TemplateAnalyticsBucket{
			WABATimezone: b.WABATimezone,
			Granularity:  b.Granularity,
			ProductType:  b.ProductType,
			DataPoints:   pts,
		})
	}
	resp := appmetaanalytics.TemplateAnalyticsResponse{Data: buckets}
	if out.Paging != nil {
		p := &appmetaanalytics.TemplateAnalyticsPaging{}
		p.Cursors.Before = out.Paging.Cursors.Before
		p.Cursors.After = out.Paging.Cursors.After
		resp.Paging = p
	}
	return resp, nil
}

// integrationSettingsResolverFunc adapts a closure into
// appsettings.Resolver.
type integrationSettingsResolverFunc func(ctx context.Context, key string, integ dintegration.Integration, secrets map[string]string) (appsettings.ProviderClient, error)

// Settings implements appsettings.Resolver.
func (f integrationSettingsResolverFunc) Settings(ctx context.Context, key string, integ dintegration.Integration, secrets map[string]string) (appsettings.ProviderClient, error) {
	return f(ctx, key, integ, secrets)
}

// callPermissionLookupFunc adapts a closure into
// appsettings.CallPermissionLookup so the settings drawer can query the
// calling application service without the settings package importing
// ports/calling.
type callPermissionLookupFunc func(ctx context.Context, orgID organization.ID, id dintegration.ID, waID string) (appsettings.CallPermission, error)

// LookupCallPermission implements appsettings.CallPermissionLookup.
func (f callPermissionLookupFunc) LookupCallPermission(ctx context.Context, orgID organization.ID, id dintegration.ID, waID string) (appsettings.CallPermission, error) {
	return f(ctx, orgID, id, waID)
}

// whatsappSettingsAdapter bridges the WhatsApp adapter's provider-native
// business-profile / call-settings / OBA methods with the
// provider-neutral appsettings.ProviderClient port. Every method is a
// one-shot translation between the adapter's shape and the DTO shape;
// the whatsapp package remains the sole owner of the Meta wire format.
type whatsappSettingsAdapter struct {
	p *whatsapp.Provider
}

// GetBusinessProfile implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) GetBusinessProfile(ctx context.Context) (appsettings.BusinessProfileDTO, error) {
	bp, err := a.p.GetBusinessProfile(ctx)
	if err != nil {
		return appsettings.BusinessProfileDTO{}, err
	}
	return appsettings.BusinessProfileDTO{
		About:             bp.About,
		Address:           bp.Address,
		Description:       bp.Description,
		Email:             bp.Email,
		ProfilePictureURL: bp.ProfilePictureURL,
		Vertical:          bp.Vertical,
		Websites:          bp.Websites,
	}, nil
}

// UpdateBusinessProfile implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) UpdateBusinessProfile(ctx context.Context, bp appsettings.BusinessProfileDTO) error {
	return a.p.UpdateBusinessProfile(ctx, whatsapp.BusinessProfile{
		About:             bp.About,
		Address:           bp.Address,
		Description:       bp.Description,
		Email:             bp.Email,
		ProfilePictureURL: bp.ProfilePictureURL,
		Vertical:          bp.Vertical,
		Websites:          bp.Websites,
	})
}

// GetCallSettings implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) GetCallSettings(ctx context.Context) (appsettings.CallSettingsDTO, error) {
	cs, err := a.p.GetCallSettings(ctx)
	if err != nil {
		return appsettings.CallSettingsDTO{}, err
	}
	return toCallSettingsDTO(cs), nil
}

// UpdateCallSettings implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) UpdateCallSettings(ctx context.Context, cs appsettings.CallSettingsDTO) error {
	return a.p.UpdateCallSettings(ctx, fromCallSettingsDTO(cs))
}

// GetOBAStatus implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) GetOBAStatus(ctx context.Context) (appsettings.OBAStatusDTO, error) {
	s, err := a.p.GetOBAStatus(ctx)
	if err != nil {
		return appsettings.OBAStatusDTO{}, err
	}
	return appsettings.OBAStatusDTO{OBAStatus: s.OBAStatus, StatusMessage: s.StatusMessage}, nil
}

// ApplyOBA implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) ApplyOBA(ctx context.Context) (appsettings.OBAStatusDTO, error) {
	s, err := a.p.ApplyOBA(ctx)
	if err != nil {
		return appsettings.OBAStatusDTO{}, err
	}
	return appsettings.OBAStatusDTO{OBAStatus: s.OBAStatus, StatusMessage: s.StatusMessage}, nil
}

// WithdrawOBA implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) WithdrawOBA(ctx context.Context) (appsettings.OBAStatusDTO, error) {
	s, err := a.p.WithdrawOBA(ctx)
	if err != nil {
		return appsettings.OBAStatusDTO{}, err
	}
	return appsettings.OBAStatusDTO{OBAStatus: s.OBAStatus, StatusMessage: s.StatusMessage}, nil
}

// GetUsername implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) GetUsername(ctx context.Context) (appsettings.UsernameDTO, error) {
	u, err := a.p.GetUsername(ctx)
	if err != nil {
		return appsettings.UsernameDTO{}, err
	}
	return appsettings.UsernameDTO{Username: u.Username, Status: u.Status}, nil
}

// SetUsername implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) SetUsername(ctx context.Context, username, transferAction string) (appsettings.UsernameDTO, error) {
	u, err := a.p.SetUsername(ctx, username, transferAction)
	if err != nil {
		return appsettings.UsernameDTO{}, err
	}
	return appsettings.UsernameDTO{Username: u.Username, Status: u.Status}, nil
}

// DeleteUsername implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) DeleteUsername(ctx context.Context) error {
	return a.p.DeleteUsername(ctx)
}

// GetUsernameSuggestions implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) GetUsernameSuggestions(ctx context.Context) ([]string, error) {
	return a.p.GetUsernameSuggestions(ctx)
}

// SetWebhookOverride implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) SetWebhookOverride(ctx context.Context, callbackURL, verifyToken string) error {
	return a.p.SetWebhookOverride(ctx, callbackURL, verifyToken)
}

// GetPhoneNumber implements appsettings.ProviderClient.
func (a whatsappSettingsAdapter) GetPhoneNumber(ctx context.Context) (appsettings.PhoneNumberDTO, error) {
	pn, err := a.p.GetPhoneNumber(ctx)
	if err != nil {
		return appsettings.PhoneNumberDTO{}, err
	}
	return appsettings.PhoneNumberDTO{
		ID:                        pn.ID,
		DisplayPhoneNumber:        pn.DisplayPhoneNumber,
		VerifiedName:              pn.VerifiedName,
		Status:                    pn.Status,
		QualityRating:             pn.QualityRating,
		CountryCode:               pn.CountryCode,
		CountryDialCode:           pn.CountryDialCode,
		CodeVerificationStatus:    pn.CodeVerificationStatus,
		AccountMode:               pn.AccountMode,
		HostPlatform:              pn.HostPlatform,
		MessagingLimitTier:        pn.MessagingLimitTier,
		IsOfficialBusinessAccount: pn.IsOfficialBusinessAccount,
	}, nil
}

// toCallSettingsDTO copies whatsapp.CallSettings into the neutral DTO.
func toCallSettingsDTO(cs whatsapp.CallSettings) appsettings.CallSettingsDTO {
	dto := appsettings.CallSettingsDTO{
		Status:                   cs.Status,
		CallIconVisibility:       cs.CallIconVisibility,
		CallbackPermissionStatus: cs.CallbackPermissionStatus,
	}
	if cs.CallHours != nil {
		hours := make([]appsettings.WeeklyHoursDTO, 0, len(cs.CallHours.WeeklyOperatingHours))
		for _, w := range cs.CallHours.WeeklyOperatingHours {
			hours = append(hours, appsettings.WeeklyHoursDTO{
				DayOfWeek: w.DayOfWeek,
				OpenTime:  w.OpenTime,
				CloseTime: w.CloseTime,
			})
		}
		dto.CallHours = &appsettings.CallHoursDTO{
			Status:               cs.CallHours.Status,
			TimezoneID:           cs.CallHours.TimezoneID,
			WeeklyOperatingHours: hours,
		}
	}
	return dto
}

// fromCallSettingsDTO is the inverse of toCallSettingsDTO.
func fromCallSettingsDTO(dto appsettings.CallSettingsDTO) whatsapp.CallSettings {
	cs := whatsapp.CallSettings{
		Status:                   dto.Status,
		CallIconVisibility:       dto.CallIconVisibility,
		CallbackPermissionStatus: dto.CallbackPermissionStatus,
	}
	if dto.CallHours != nil {
		hours := make([]whatsapp.WeeklyHours, 0, len(dto.CallHours.WeeklyOperatingHours))
		for _, w := range dto.CallHours.WeeklyOperatingHours {
			hours = append(hours, whatsapp.WeeklyHours{
				DayOfWeek: w.DayOfWeek,
				OpenTime:  w.OpenTime,
				CloseTime: w.CloseTime,
			})
		}
		cs.CallHours = &whatsapp.CallHours{
			Status:               dto.CallHours.Status,
			TimezoneID:           dto.CallHours.TimezoneID,
			WeeklyOperatingHours: hours,
		}
	}
	return cs
}

// whatsappChannelProvider embeds *whatsapp.Provider so it satisfies the
// channel.Provider port via promotion, then overrides the group-family
// methods to return app-level DTO shapes. That lets the group service's
// prov.(appgrp.ProviderGroupsClient) type assertion succeed without
// pushing whatsapp package types into application/group.
type whatsappChannelProvider struct {
	*whatsapp.Provider
}

// ListGroups implements appgrp.ProviderGroupsClient.
func (w *whatsappChannelProvider) ListGroups(ctx context.Context) ([]appgrp.ProviderGroupSummary, error) {
	items, err := w.Provider.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]appgrp.ProviderGroupSummary, 0, len(items))
	for _, g := range items {
		out = append(out, appgrp.ProviderGroupSummary{
			ProviderGroupID: g.ProviderGroupID,
			Subject:         g.Subject,
			CreatedAtUnix:   g.CreatedAtUnix,
		})
	}
	return out, nil
}

// GetGroup implements appgrp.ProviderGroupsClient.
func (w *whatsappChannelProvider) GetGroup(ctx context.Context, providerGroupID string) (appgrp.ProviderGroupDetail, error) {
	d, err := w.Provider.GetGroup(ctx, providerGroupID)
	if err != nil {
		return appgrp.ProviderGroupDetail{}, err
	}
	participants := make([]appgrp.ProviderGroupMember, 0, len(d.Participants))
	for _, m := range d.Participants {
		participants = append(participants, appgrp.ProviderGroupMember{
			WaID:  m.WaID,
			BSUID: m.BSUID,
			Role:  m.Role,
		})
	}
	return appgrp.ProviderGroupDetail{
		ProviderGroupID:       d.ProviderGroupID,
		Subject:               d.Subject,
		Description:           d.Description,
		CreationTimestampUnix: d.CreationTimestampUnix,
		Suspended:             d.Suspended,
		JoinApprovalMode:      d.JoinApprovalMode,
		TotalParticipantCount: d.TotalParticipantCount,
		Participants:          participants,
	}, nil
}

// ListGroupMembers implements appgrp.ProviderGroupsClient.
func (w *whatsappChannelProvider) ListGroupMembers(ctx context.Context, providerGroupID string) ([]appgrp.ProviderGroupMember, error) {
	items, err := w.Provider.ListGroupMembers(ctx, providerGroupID)
	if err != nil {
		return nil, err
	}
	out := make([]appgrp.ProviderGroupMember, 0, len(items))
	for _, m := range items {
		out = append(out, appgrp.ProviderGroupMember{
			WaID:  m.WaID,
			BSUID: m.BSUID,
			Role:  m.Role,
		})
	}
	return out, nil
}

// CreateGroup implements appgrp.ProviderGroupsClient.
func (w *whatsappChannelProvider) CreateGroup(ctx context.Context, req appgrp.ProviderCreateGroupRequest) (appgrp.ProviderCreateGroupResult, error) {
	res, err := w.Provider.CreateGroup(ctx, whatsapp.CreateGroupRequest{
		Subject:          req.Subject,
		Description:      req.Description,
		JoinApprovalMode: req.JoinApprovalMode,
	})
	if err != nil {
		return appgrp.ProviderCreateGroupResult{}, err
	}
	return appgrp.ProviderCreateGroupResult{ProviderGroupID: res.ProviderGroupID}, nil
}

// whatsappTemplateAdapter bridges the WhatsApp adapter's provider-native
// template shapes with the provider-neutral apptmpl.TemplateProvider port.
// The Component vocabulary is folded through JSON so provider-specific
// fields (buttons, cards, headers) round-trip via tmpldom.Component's
// Extra map without a per-field bridge.
type whatsappTemplateAdapter struct {
	p *whatsapp.Provider
}

// ListTemplates implements apptmpl.TemplateProvider.
func (a whatsappTemplateAdapter) ListTemplates(ctx context.Context) ([]apptmpl.ProviderTemplateSummary, error) {
	items, err := a.p.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]apptmpl.ProviderTemplateSummary, 0, len(items))
	for _, it := range items {
		out = append(out, apptmpl.ProviderTemplateSummary{
			ID:         it.ID,
			Name:       it.Name,
			Language:   it.Language,
			Status:     it.Status,
			Category:   it.Category,
			Components: componentsFromMaps(it.Components),
		})
	}
	return out, nil
}

// CreateTemplate implements apptmpl.TemplateProvider.
func (a whatsappTemplateAdapter) CreateTemplate(ctx context.Context, req apptmpl.ProviderCreateRequest) (apptmpl.ProviderCreateResult, error) {
	res, err := a.p.CreateTemplate(ctx, whatsapp.TemplateCreateRequest{
		Name:                req.Name,
		Language:            req.Language,
		Category:            req.Category,
		Components:          componentsToMaps(req.Components),
		AllowCategoryChange: req.AllowCategoryChange,
	})
	if err != nil {
		return apptmpl.ProviderCreateResult{}, err
	}
	return apptmpl.ProviderCreateResult{
		ProviderTemplateID: res.ID,
		Status:             res.Status,
		Category:           res.Category,
	}, nil
}

// GetTemplateStatus implements apptmpl.TemplateProvider.
func (a whatsappTemplateAdapter) GetTemplateStatus(ctx context.Context, providerTemplateID string) (apptmpl.ProviderTemplateSummary, error) {
	st, err := a.p.GetTemplateStatus(ctx, providerTemplateID)
	if err != nil {
		return apptmpl.ProviderTemplateSummary{}, err
	}
	return apptmpl.ProviderTemplateSummary{
		ID:       st.ID,
		Name:     st.Name,
		Language: st.Language,
		Status:   st.Status,
		Category: st.Category,
	}, nil
}

// componentsToMaps folds domain Components into the []map[string]any shape
// the WhatsApp create endpoint expects. JSON round-trip preserves every
// named field plus the Extra bag.
func componentsToMaps(cs []tmpldom.Component) []map[string]any {
	out := make([]map[string]any, 0, len(cs))
	for _, c := range cs {
		b, err := json.Marshal(c)
		if err != nil {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

// componentsFromMaps folds a provider list-response's Components (raw
// map[string]any) into the domain Component shape. Unknown keys are
// preserved in Extra via the JSON round-trip.
func componentsFromMaps(ms []map[string]any) []tmpldom.Component {
	out := make([]tmpldom.Component, 0, len(ms))
	for _, m := range ms {
		b, err := json.Marshal(m)
		if err != nil {
			continue
		}
		var c tmpldom.Component
		if err := json.Unmarshal(b, &c); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// argon2Hasher adapts the infrastructure/auth argon2id helpers to the
// appapitoken.PasswordHasher port. Kept in cmd/server so the application
// layer never imports the infrastructure package directly.
type argon2Hasher struct{ p infauth.Argon2Params }

// Hash returns an argon2id-encoded hash of pw using the configured params.
func (h argon2Hasher) Hash(pw string) (string, error) {
	return infauth.HashPassword(pw, h.p)
}

// Verify reports whether pw matches the encoded argon2id hash.
func (h argon2Hasher) Verify(pw, encoded string) (bool, error) {
	return infauth.VerifyPassword(pw, encoded)
}

// bearerVerifierAdapter adapts *appapitoken.Service to
// middleware.BearerVerifier, translating the application-layer principal
// into the infra-layer shape so the middleware package can stay free of
// application imports (dependency rule).
type bearerVerifierAdapter struct{ svc *appapitoken.Service }

// VerifyBearer implements middleware.BearerVerifier.
func (a bearerVerifierAdapter) VerifyBearer(ctx context.Context, plaintext string) (middleware.BearerPrincipal, error) {
	p, err := a.svc.Verify(ctx, plaintext)
	if err != nil {
		if errors.Is(err, appapitoken.ErrInvalidToken) {
			return middleware.BearerPrincipal{}, middleware.ErrInvalidBearer
		}
		return middleware.BearerPrincipal{}, err
	}
	return middleware.BearerPrincipal{
		OrgID:   string(p.OrgID),
		UserID:  string(p.UserID),
		TokenID: string(p.TokenID),
	}, nil
}

// tokenUsageRecorderAdapter adapts *appapitokenusage.Service to
// middleware.TokenUsageRecorder, translating the infrastructure-layer
// TokenUsageEvent into the domain-layer Entry shape so the middleware
// package can stay free of application + domain imports (dependency
// rule).
type tokenUsageRecorderAdapter struct{ svc *appapitokenusage.Service }

// RecordUsage implements middleware.TokenUsageRecorder.
func (a tokenUsageRecorderAdapter) RecordUsage(ctx context.Context, ev middleware.TokenUsageEvent) {
	if a.svc == nil {
		return
	}
	a.svc.Record(ctx, dapitokenusage.Entry{
		OrgID:         organization.ID(ev.OrgID),
		TokenID:       dapitoken.ID(ev.TokenID),
		OccurredAt:    ev.OccurredAt,
		RequestID:     ev.RequestID,
		Method:        ev.Method,
		Path:          ev.Path,
		StatusCode:    ev.StatusCode,
		LatencyMs:     ev.LatencyMs,
		RemoteIP:      ev.RemoteIP,
		UserAgent:     ev.UserAgent,
		RequestBody:   ev.RequestBody,
		ResponseBody:  ev.ResponseBody,
		RequestBytes:  ev.RequestBytes,
		ResponseBytes: ev.ResponseBytes,
		ErrorMessage:  ev.ErrorMessage,
	})
}

// orgLister satisfies workers.OrgLister by scanning the organizations
// table. Kept in cmd/server so the application layer never learns about
// the shape of the table.
type orgLister struct{ db *sql.DB }

// ListOrgIDs implements workers.OrgLister. The organizations.id column is
// VARBINARY(16) — raw ULID bytes — so we decode to the canonical string
// form before handing it back; downstream repos parse ULID text, not
// bytes.
func (o orgLister) ListOrgIDs(ctx context.Context) ([]organization.ID, error) {
	rows, err := o.db.QueryContext(ctx, "SELECT id FROM organizations")
	if err != nil {
		return nil, fmt.Errorf("list org ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []organization.ID
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan org id: %w", err)
		}
		if len(raw) != 16 {
			return nil, fmt.Errorf("scan org id: expected 16 bytes, got %d", len(raw))
		}
		var id ulid.ULID
		copy(id[:], raw)
		out = append(out, organization.ID(id.String()))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate org ids: %w", err)
	}
	return out, nil
}
