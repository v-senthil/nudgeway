// Package call is the application-layer service for the canonical Call
// entity. It orchestrates the domain call.Call, the CallRepo persistence
// port, related contact/session/conversation lookups, and the calling
// Provider port. No provider SDK is imported here.
package call

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/call"
	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/events"
	"github.com/v-senthil/nudgeway/internal/domain/identity"
	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/message"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/attachments"
	"github.com/v-senthil/nudgeway/internal/ports/calling"
	"github.com/v-senthil/nudgeway/internal/ports/eventbus"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// AttachmentDownloader resolves a provider-native media handle to a byte
// stream + content-type. The call service uses it to pull recording +
// transcript bytes off provider CDNs (e.g. Meta) without importing any
// provider package directly — implementations live in cmd/server and
// close over the concrete adapter registry.
//
// Structurally identical to appmsg.AttachmentDownloader; kept as an
// independent port here so the two application services stay independently
// portable per the dependency rule.
type AttachmentDownloader interface {
	// Download returns a byte stream + content-type. Callers pass whichever
	// of (mediaID, mediaURL) they have; implementations prefer mediaURL
	// when non-empty and fall back to mediaID.
	Download(ctx context.Context, providerKey string, integrationID integration.ID, mediaID, mediaURL string) (io.ReadCloser, string, error)
}

// ErrCallNotFound is returned by Get / mutation methods when the (org, id)
// tuple does not exist.
var ErrCallNotFound = errors.New("call: not found")

// ErrCallIntegrationMissing is returned when the caller-selected
// integration cannot be resolved to a live provider adapter.
var ErrCallIntegrationMissing = errors.New("call: integration missing")

// ErrCallValidation is returned for shape/validation failures on inbound
// requests (missing to / integration_id, etc.).
var ErrCallValidation = errors.New("call: validation failed")

// ErrTranscriptNotAvailable is returned by GetTranscript when the call
// row has no transcription_ref yet. Callers should surface this as a
// 409 conflict — the call may complete a transcript later.
var ErrTranscriptNotAvailable = errors.New("call: transcript not available")

// ErrCallPermissionMissing is the sentinel returned by RequestCall when the
// preflight permission check reports the recipient has not granted
// call permission. Callers should use errors.As on *PermissionErr to
// recover the {Status, ExpiresAt} payload for the 428 response body.
var ErrCallPermissionMissing = errors.New("call: recipient permission missing")

// CallPermissionInfo carries the recipient's current permission state for
// UI rendering. Populated on the PermissionErr wrapper.
type CallPermissionInfo struct {
	// Status is the provider-reported permission enum
	// ("no_permission" | "temporary" | "permanent" | ...). Empty when the
	// provider returned no permission block.
	Status string
	// ExpiresAt is the unix seconds when a "temporary" permission lapses.
	// Zero otherwise.
	ExpiresAt int64
}

// PermissionErr is the typed error wrapping ErrCallPermissionMissing with
// the current permission info. errors.Is against ErrCallPermissionMissing
// still matches; errors.As against *PermissionErr recovers Info.
type PermissionErr struct {
	Info CallPermissionInfo
}

// Error implements error.
func (e *PermissionErr) Error() string {
	if e.Info.Status == "" {
		return "call: recipient permission missing"
	}
	return fmt.Sprintf("call: recipient permission is %q", e.Info.Status)
}

// Unwrap returns the sentinel so errors.Is(err, ErrCallPermissionMissing) works.
func (e *PermissionErr) Unwrap() error { return ErrCallPermissionMissing }

// IntegrationSecrets exposes decrypted integration secrets to the calling
// service. Mirrors the shape used by the message send pipeline so wire-up
// can reuse the same infra adapter.
type IntegrationSecrets interface {
	repository.IntegrationRepo

	// GetWithSecrets returns the Integration alongside a map of decrypted
	// secret material keyed by name.
	GetWithSecrets(
		ctx context.Context,
		orgID organization.ID,
		id integration.ID,
	) (integration.Integration, map[string]string, error)
}

// CallingProviderRegistry resolves a calling.Provider for a given
// (providerKey, secrets) tuple. Defined here so the application service
// never imports a concrete provider package — cmd/server injects a
// concrete implementation that closes over the calling adapters.
//
// Structurally identical to calling.Registry; the alias lives here so
// the application package has a stable local name for the dependency.
type CallingProviderRegistry interface {
	Calling(ctx context.Context, providerKey string, secrets map[string]string) (calling.Provider, error)
}

// IDGenerator mints Call IDs. Kept as a port so tests can inject a
// deterministic generator without importing ulid here.
type IDGenerator interface {
	// NewCallID returns a fresh globally-unique call id (ULID string).
	NewCallID() string
}

// MessageIDGenerator mints Message IDs for the inline "call" system
// messages injected on call webhook ingest. Kept separate from
// IDGenerator so tests can inject a deterministic generator without
// depending on the outbound message service.
type MessageIDGenerator interface {
	// NewMessageID returns a fresh globally-unique message id (ULID
	// string).
	NewMessageID() string
}

// Clock returns the current time. Kept as a port for deterministic tests.
type Clock interface {
	Now() time.Time
}

// Deps bundles the service's dependencies.
type Deps struct {
	// Repo persists call rows.
	Repo repository.CallRepo
	// Contacts resolves contact rows for linking.
	Contacts repository.ContactRepo
	// Sessions resolves / creates sessions for inbound linkage.
	Sessions repository.SessionRepo
	// Conversations resolves / creates conversations for stitching.
	Conversations repository.ConversationRepo
	// Endpoints resolves the BusinessEndpoint that owns the call.
	Endpoints repository.BusinessEndpointRepo
	// Integrations resolves integration config + decrypts secrets.
	Integrations IntegrationSecrets
	// CallingProviders resolves the calling adapter for a given
	// (providerKey, secrets) tuple.
	CallingProviders CallingProviderRegistry
	// Publisher publishes canonical Call* events.
	Publisher eventbus.Publisher
	// IDs mints new call ids.
	IDs IDGenerator
	// Clock returns the current time.
	Clock Clock
	// Logger receives structured records.
	Logger *slog.Logger
	// Attachments persists recording + transcript bytes captured from
	// webhooks. Optional — nil disables the durable-storage path and the
	// service falls back to per-request Meta proxying.
	Attachments attachments.Store
	// Downloader fetches provider media by URL (uses the integration's
	// Bearer token). Optional in the same way as Attachments.
	Downloader AttachmentDownloader
	// Messages is the message repository used to inject an inline "call"
	// system message into the conversation thread on call ingest.
	// Optional — nil disables the inline-message path (call rows still
	// persist, just without an in-thread marker for operators).
	Messages repository.MessageRepo
	// MessageIDs mints the ULID for the inline "call" message row.
	// Required when Messages is non-nil.
	MessageIDs MessageIDGenerator
	// Identities upserts phone + BSUID identities for the caller. Optional —
	// nil disables the linkage bootstrap and the call row lands without
	// contact/session/conversation attachment (so no inline info messages).
	Identities repository.IdentityRepo
	// ContactIDs mints Contact IDs for freshly-observed callers. Required
	// when Identities is non-nil.
	ContactIDs interface {
		// NewContactID mints a globally-unique contact id (ULID string).
		NewContactID() string
	}
}

// Service implements the call use-cases. All methods enforce tenancy by
// requiring an OrgID and passing it into every downstream call.
type Service struct {
	deps Deps
}

// New constructs a Service. Panics on missing required dependencies.
func New(deps Deps) *Service {
	if deps.Repo == nil || deps.Integrations == nil ||
		deps.CallingProviders == nil || deps.Publisher == nil ||
		deps.IDs == nil || deps.Clock == nil {
		panic("call.New: missing required dependency")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Service{deps: deps}
}

// InitiateRequest is the shape RequestCall takes from the REST handler.
type InitiateRequest struct {
	// OrgID is the tenant boundary.
	OrgID string
	// IntegrationID is the ULID of the integration to place the call
	// through.
	IntegrationID string
	// To is the callee address (E.164 phone or BSUID).
	To string
	// ToUserID is the callee BSUID when known.
	ToUserID string
	// ContactID optionally links the call to an existing contact row.
	ContactID string
	// IdempotencyKey is the caller-supplied idempotency token.
	IdempotencyKey string
	// CorrelationID stitches the outbound call back to the originating
	// request or job.
	CorrelationID string
	// Recording opts the call into recording. Optional.
	Recording *calling.RecordingOptions
	// Transcription opts the call into transcription. Optional.
	Transcription *calling.TranscriptionOptions
}

// InitiateResponse is the 202 body of POST /api/v1/calls.
type InitiateResponse struct {
	CallID string
	Status string
}

// RequestCall is the REST entry point for placing an outbound call. Steps:
//  1. Validate request shape.
//  2. Resolve integration + secrets + provider adapter.
//  3. Persist a Call(queued) row.
//  4. Invoke Provider.InitiateCall.
//  5. Stamp provider_call_id + status=ringing on the row.
//  6. Publish CallInitiated / CallRinging.
//
// Failures after step 3 mark the row failed (permanent errors) or leave
// it queued (transient) for a sweeper to retry.
func (s *Service) RequestCall(ctx context.Context, req InitiateRequest) (InitiateResponse, error) {
	if req.OrgID == "" {
		return InitiateResponse{}, fmt.Errorf("%w: missing org", ErrCallValidation)
	}
	if req.IntegrationID == "" {
		return InitiateResponse{}, fmt.Errorf("%w: missing integration_id", ErrCallValidation)
	}
	if req.To == "" && req.ToUserID == "" {
		return InitiateResponse{}, fmt.Errorf("%w: to or to_user_id required", ErrCallValidation)
	}
	orgID := organization.ID(req.OrgID)

	// Resolve integration + adapter.
	integ, secrets, err := s.deps.Integrations.GetWithSecrets(ctx, orgID, integration.ID(req.IntegrationID))
	if err != nil {
		return InitiateResponse{}, fmt.Errorf("%w: %w", ErrCallIntegrationMissing, err)
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	if v, ok := integ.Config["phone_number_id"].(string); ok && v != "" {
		secrets["phone_number_id"] = v
	}
	if v, ok := integ.Config["waba_id"].(string); ok && v != "" {
		secrets["waba_id"] = v
	}
	secrets["_integration_id"] = string(integ.ID)
	secrets["_org_id"] = string(integ.OrgID)

	provider, err := s.deps.CallingProviders.Calling(ctx, integ.Provider, secrets)
	if err != nil {
		return InitiateResponse{}, fmt.Errorf("%w: resolve provider %q: %w", ErrCallIntegrationMissing, integ.Provider, err)
	}
	if !provider.Capabilities().InitiateOutbound {
		return InitiateResponse{}, fmt.Errorf("%w: %w", call.ErrProviderUnsupported, errors.New("outbound not supported"))
	}

	// Preflight: check the recipient's call permission BEFORE hitting Meta's
	// /calls endpoint. This avoids a 400 "user hasn't granted permission"
	// and lets the UI surface a green "Send permission request" affordance
	// instead of a generic provider_error. Permission failures are advisory
	// (network errors don't block the call attempt).
	perm, permErr := provider.GetPermission(ctx, req.To, req.ToUserID)
	if permErr == nil && (perm.Status == "" || perm.Status == "no_permission") {
		return InitiateResponse{}, &PermissionErr{Info: CallPermissionInfo{
			Status:    perm.Status,
			ExpiresAt: perm.ExpirationTime,
		}}
	}
	if permErr != nil {
		s.deps.Logger.Warn("call.permission preflight failed (advisory)",
			slog.String("org_id", req.OrgID),
			slog.Any("err", permErr),
		)
	}

	// Resolve the endpoint (phone_number_id) that will place the call.
	endpointExternalID, _ := secrets["phone_number_id"]

	// Persist the row in queued state. The provider call id is stamped
	// after InitiateCall returns.
	now := s.deps.Clock.Now().UTC()
	callID := call.ID(s.deps.IDs.NewCallID())
	row := call.Call{
		ID:             callID,
		OrgID:          orgID,
		IntegrationID:  string(integ.ID),
		Provider:       integ.Provider,
		ProviderCallID: "pending:" + string(callID),
		Direction:      call.DirectionOutbound,
		Status:         call.StatusQueued,
		From:           endpointExternalID,
		To:             req.To,
		ToUserID:       req.ToUserID,
		Extras: map[string]any{
			"idempotency_key": req.IdempotencyKey,
			"correlation_id":  req.CorrelationID,
		},
		CreatedAt: now,
	}
	if req.ContactID != "" {
		cid := contact.ID(req.ContactID)
		row.ContactID = &cid
	}
	if err := s.deps.Repo.Create(ctx, row); err != nil {
		return InitiateResponse{}, fmt.Errorf("persist call: %w", err)
	}

	// Fire the initiate. Transient failures bubble as 502; permanent
	// failures mark the call FAILED and still surface a 502.
	res, initErr := provider.InitiateCall(ctx, calling.CallRequest{
		OrganizationID:             string(orgID),
		IntegrationID:              string(integ.ID),
		BusinessEndpointExternalID: endpointExternalID,
		To:                         req.To,
		ToUserID:                   req.ToUserID,
		IdempotencyKey:             req.IdempotencyKey,
		CorrelationID:              req.CorrelationID,
		Recording:                  req.Recording,
		Transcription:              req.Transcription,
	})
	if initErr != nil {
		s.deps.Logger.Warn("call.initiate provider failed",
			slog.String("org_id", req.OrgID),
			slog.String("call_id", string(callID)),
			slog.Any("err", initErr),
		)
		_ = s.deps.Repo.UpdateStatus(ctx, orgID, callID, call.StatusFailed, s.deps.Clock.Now().UTC())
		return InitiateResponse{}, fmt.Errorf("provider initiate: %w", initErr)
	}

	// Stamp the provider call id + advance to ringing via UpsertByProviderID
	// (Create used a placeholder to satisfy the (org, provider, provider_call_id)
	// unique index). We overwrite the placeholder with the real id.
	now = s.deps.Clock.Now().UTC()
	row.ProviderCallID = res.ProviderCallID
	row.Status = call.StatusRinging
	row.StartedAt = &now
	row.UpdatedAt = &now
	if _, err := s.deps.Repo.UpsertByProviderID(ctx, row); err != nil {
		s.deps.Logger.Warn("call.initiate stamp provider id failed",
			slog.String("org_id", req.OrgID),
			slog.String("call_id", string(callID)),
			slog.String("provider_call_id", res.ProviderCallID),
			slog.Any("err", err),
		)
	}

	// Publish the canonical events.
	_ = s.deps.Publisher.Publish(ctx, events.Envelope{
		Type:           events.CallInitiated,
		OrganizationID: string(orgID),
		OccurredAt:     now,
		CorrelationID:  req.CorrelationID,
		Payload: events.CallEventPayload{
			CallID:                     string(callID),
			Provider:                   integ.Provider,
			ProviderCallID:             res.ProviderCallID,
			BusinessEndpointExternalID: endpointExternalID,
			Direction:                  string(call.DirectionOutbound),
			Status:                     string(call.StatusRinging),
			From:                       endpointExternalID,
			To:                         req.To,
			ToUserID:                   req.ToUserID,
			StartedAt:                  now,
			Timestamp:                  now,
		},
	})

	s.deps.Logger.Info("call.initiate accepted",
		slog.String("org_id", req.OrgID),
		slog.String("call_id", string(callID)),
		slog.String("provider_call_id", res.ProviderCallID),
	)

	return InitiateResponse{CallID: string(callID), Status: string(call.StatusRinging)}, nil
}

// ProcessInboundEvent handles a webhook-derived events.Envelope carrying a
// CallEventPayload. It upserts the corresponding Call row and publishes
// the mirrored canonical event.
//
// The envelope's Type selects the transition:
//   - CallInitiated / CallRinging → status=ringing (or queued for outbound)
//   - CallAnswered                → status=answered, stamps AnsweredAt
//   - CallEnded / CallEndedDetailed → status=completed, stamps EndedAt +
//     duration
//   - CallFailed                  → status=failed | declined | no_answer
//     depending on payload.HangupReason
//   - CallRecordingCreated        → attaches RecordingURL to the row
func (s *Service) ProcessInboundEvent(ctx context.Context, envelope events.Envelope) error {
	payload, ok := envelope.Payload.(events.CallEventPayload)
	if !ok {
		return fmt.Errorf("call.ProcessInboundEvent: payload is not CallEventPayload (%T)", envelope.Payload)
	}
	if payload.ProviderCallID == "" || payload.Provider == "" {
		return fmt.Errorf("%w: missing provider_call_id/provider on inbound event", ErrCallValidation)
	}
	orgID := organization.ID(envelope.OrganizationID)
	now := s.deps.Clock.Now().UTC()

	// Recording arrives after termination; attach + stash media id + short-
	// circuit. The parser stashes the media asset id under Raw
	// ["recording_media_id"] — persist it into Extras so GetRecording can
	// look it up (the URL Meta ships is short-lived and requires the
	// Bearer token; the media id is the durable handle).
	if envelope.Type == events.CallRecordingCreated && payload.RecordingURL != "" {
		existing, err := s.findByProviderID(ctx, orgID, payload.Provider, payload.ProviderCallID)
		// If the row doesn't exist yet (Meta webhook order isn't
		// guaranteed — recording can arrive before connect/terminate),
		// bootstrap it with a fresh ULID + completed status so recording
		// metadata has somewhere to land.
		if errors.Is(err, call.ErrNotFound) {
			bootstrap := call.Call{
				ID:             call.ID(s.deps.IDs.NewCallID()),
				OrgID:          orgID,
				Provider:       payload.Provider,
				ProviderCallID: payload.ProviderCallID,
				Direction:      call.DirectionInbound,
				Status:         call.StatusCompleted,
				From:           payload.From,
				To:             payload.To,
				FromUserID:     payload.FromUserID,
				ToUserID:       payload.ToUserID,
				CreatedAt:      now,
			}
			s.enrichIntegration(ctx, orgID, &bootstrap, payload.BusinessEndpointExternalID)
			s.enrichLinkage(ctx, orgID, &bootstrap, payload)
			if saved, uerr := s.deps.Repo.UpsertByProviderID(ctx, bootstrap); uerr == nil {
				existing = saved
				err = nil
			} else {
				s.deps.Logger.Warn("call: recording bootstrap upsert failed",
					slog.String("provider_call_id", payload.ProviderCallID),
					slog.Any("err", uerr),
				)
			}
		}
		if err == nil {
			s.injectCallInfoMessage(ctx, existing, envelope)
			_ = s.deps.Repo.AttachRecording(ctx, orgID, existing.ID, payload.RecordingURL, payload.DurationSeconds)
			var mediaID string
			if payload.Raw != nil {
				if v, ok := payload.Raw["recording_media_id"].(string); ok {
					mediaID = v
				}
			}

			extras := map[string]any{}
			for k, v := range existing.Extras {
				extras[k] = v
			}
			changed := false
			if mediaID != "" {
				extras["recording_media_id"] = mediaID
				changed = true
			}

			// Durable persist: download the recording bytes with the
			// integration's Bearer token and stash into the attachments
			// store keyed by SHA-256. Failures are logged and swallowed —
			// the Meta short-lived URL is still saved on the row so the
			// GetRecording proxy path still works until Meta expires it.
			if s.deps.Attachments != nil && s.deps.Downloader != nil && existing.IntegrationID != "" {
				// Force the mediaID two-hop resolve path: Meta's inline
				// RecordingURL is short-lived (~5 min) and expires before
				// most replay/retry paths. The media id is durable for
				// Meta's retention window (~30 days) — the downloader
				// resolves a fresh URL each time.
				body, ctype, derr := s.deps.Downloader.Download(ctx, payload.Provider, integration.ID(existing.IntegrationID), mediaID, "")
				if derr != nil {
					s.deps.Logger.Warn("call: recording download failed",
						slog.String("call_id", string(existing.ID)),
						slog.String("provider_call_id", payload.ProviderCallID),
						slog.Any("err", derr),
					)
				} else {
					key, _, _, perr := s.deps.Attachments.Put(ctx, ctype, body)
					_ = body.Close()
					if perr != nil {
						s.deps.Logger.Warn("call: recording store failed",
							slog.String("call_id", string(existing.ID)),
							slog.String("provider_call_id", payload.ProviderCallID),
							slog.Any("err", perr),
						)
					} else {
						extras["recording_key"] = key
						if ctype != "" {
							extras["recording_content_type"] = ctype
						}
						changed = true
					}
				}
			}

			if changed {
				merge := call.Call{
					OrgID:          orgID,
					Provider:       payload.Provider,
					ProviderCallID: payload.ProviderCallID,
					Extras:         extras,
				}
				if _, err := s.deps.Repo.UpsertByProviderID(ctx, merge); err != nil {
					s.deps.Logger.Warn("call: stash recording metadata failed",
						slog.String("call_id", string(existing.ID)),
						slog.Any("err", err),
					)
				}
			}
		}
		return s.deps.Publisher.Publish(ctx, envelope)
	}

	// Transcript arrives on its own webhook after the call ends. When it
	// lands as its own envelope (event=call_transcript_available), stamp
	// the ref onto the incumbent row via a merge upsert and short-
	// circuit — otherwise the merge below would wipe the already-known
	// status/timestamps because the transcript payload doesn't carry them.
	if payload.TranscriptionRef != "" {
		// UpsertByProviderID falls through to Create when the row is
		// missing; Create requires an ID + non-empty Direction/Status.
		// Mint them here so a transcript-first webhook (Meta ordering
		// isn't guaranteed) still bootstraps the row.
		merge := call.Call{
			ID:               call.ID(s.deps.IDs.NewCallID()),
			OrgID:            orgID,
			Provider:         payload.Provider,
			ProviderCallID:   payload.ProviderCallID,
			Direction:        call.DirectionInbound,
			Status:           call.StatusCompleted,
			From:             payload.From,
			To:               payload.To,
			FromUserID:       payload.FromUserID,
			ToUserID:         payload.ToUserID,
			TranscriptionRef: payload.TranscriptionRef,
			CreatedAt:        now,
		}
		s.enrichIntegration(ctx, orgID, &merge, payload.BusinessEndpointExternalID)
		s.enrichLinkage(ctx, orgID, &merge, payload)

		// Durable persist: download the transcript document and stash
		// into the attachments store keyed by SHA-256. Failures are
		// logged and swallowed — the ref is still stored on the row so
		// the GetTranscript proxy path still works.
		if s.deps.Attachments != nil && s.deps.Downloader != nil {
			existing, ferr := s.findByProviderID(ctx, orgID, payload.Provider, payload.ProviderCallID)
			// When the row is missing, use the bootstrap integration id
			// we just resolved from the endpoint lookup above.
			if ferr != nil {
				existing = merge
				ferr = nil
			}
			if ferr == nil && existing.IntegrationID != "" {
				// TranscriptionRef is the media id, NOT a URL. Pass empty
				// mediaURL so the downloader does the two-hop resolve
				// (GET /{mediaID} for a fresh URL, then download). Same
				// reasoning as the recording path above.
				body, _, derr := s.deps.Downloader.Download(ctx, payload.Provider, integration.ID(existing.IntegrationID), payload.TranscriptionRef, "")
				if derr != nil {
					s.deps.Logger.Warn("call: transcript download failed",
						slog.String("call_id", string(existing.ID)),
						slog.String("provider_call_id", payload.ProviderCallID),
						slog.Any("err", derr),
					)
				} else {
					key, _, _, perr := s.deps.Attachments.Put(ctx, "application/json", body)
					_ = body.Close()
					if perr != nil {
						s.deps.Logger.Warn("call: transcript store failed",
							slog.String("call_id", string(existing.ID)),
							slog.String("provider_call_id", payload.ProviderCallID),
							slog.Any("err", perr),
						)
					} else {
						extras := map[string]any{}
						for k, v := range existing.Extras {
							extras[k] = v
						}
						extras["transcript_key"] = key
						merge.Extras = extras
					}
				}
			}
		}

		if _, err := s.deps.Repo.UpsertByProviderID(ctx, merge); err != nil {
			s.deps.Logger.Warn("call: attach transcription_ref failed",
				slog.String("provider_call_id", payload.ProviderCallID),
				slog.Any("err", err),
			)
		}
		return s.deps.Publisher.Publish(ctx, envelope)
	}

	// Compute the next status from the event type + payload.
	nextStatus := call.Status(payload.Status)
	if nextStatus == "" {
		nextStatus = statusFromEventType(envelope.Type, payload.HangupReason)
	}
	direction := call.Direction(payload.Direction)
	if direction == "" {
		direction = call.DirectionInbound
	}

	// Upsert the row so a first-ever inbound webhook creates the row and
	// a subsequent status update advances it.
	row := call.Call{
		ID:              call.ID(s.deps.IDs.NewCallID()),
		OrgID:           orgID,
		Provider:        payload.Provider,
		ProviderCallID:  payload.ProviderCallID,
		Direction:       direction,
		Status:          nextStatus,
		From:            payload.From,
		To:              payload.To,
		FromUserID:      payload.FromUserID,
		ToUserID:        payload.ToUserID,
		DurationSeconds: payload.DurationSeconds,
		HangupReason:    payload.HangupReason,
		Extras: map[string]any{
			"causation_id":   envelope.CausationID,
			"correlation_id": envelope.CorrelationID,
		},
		CreatedAt: now,
	}
	// Preserve the SDP offer that Meta ships on the `connect` webhook so
	// the operator's browser can build a WebRTC answer against it. Stored
	// in the row's `metadata` JSON column (Extras) — no schema change.
	if payload.Raw != nil {
		if sdp, ok := payload.Raw["session_sdp"].(string); ok && sdp != "" {
			row.Extras["session_sdp"] = sdp
			if t, ok := payload.Raw["session_sdp_type"].(string); ok && t != "" {
				row.Extras["session_sdp_type"] = t
			} else {
				row.Extras["session_sdp_type"] = "offer"
			}
		}
	}
	if !payload.StartedAt.IsZero() {
		t := payload.StartedAt.UTC()
		row.StartedAt = &t
	}
	if !payload.AnsweredAt.IsZero() {
		t := payload.AnsweredAt.UTC()
		row.AnsweredAt = &t
	}
	if !payload.EndedAt.IsZero() {
		t := payload.EndedAt.UTC()
		row.EndedAt = &t
	}
	// Best-effort endpoint resolution: look up the phone_number_id to
	// backfill the linkage. Failures are non-fatal — the row still gets
	// persisted, just without a BusinessEndpointID.
	if s.deps.Endpoints != nil && payload.BusinessEndpointExternalID != "" {
		ep, err := s.deps.Endpoints.FindByExternalID(ctx, orgID, payload.Provider, payload.BusinessEndpointExternalID)
		if err == nil {
			bid := ep.ID
			row.BusinessEndpointID = &bid
			row.IntegrationID = ep.IntegrationID
		}
	}

	// Bootstrap Contact/Session/Conversation from the caller's phone + BSUID
	// so the call row stitches into the chat thread and the info messages
	// have somewhere to land.
	s.enrichLinkage(ctx, orgID, &row, payload)

	saved, err := s.deps.Repo.UpsertByProviderID(ctx, row)
	if err != nil {
		return fmt.Errorf("upsert call: %w", err)
	}

	// Inject an inline info message into the conversation thread so
	// operators see the call transition inline in the chat. Best-effort:
	// any failure is logged + swallowed so the call event pipeline never
	// blocks on it. One info row is emitted per (call, status).
	s.injectCallInfoMessage(ctx, saved, envelope)

	s.deps.Logger.Info("call.event ingested",
		slog.String("org_id", envelope.OrganizationID),
		slog.String("call_id", string(saved.ID)),
		slog.String("provider_call_id", payload.ProviderCallID),
		slog.String("event_type", string(envelope.Type)),
		slog.String("status", string(nextStatus)),
	)

	// Re-publish the envelope so downstream subscribers (WebSocket bridge,
	// audit) see the canonical Call event with the resolved CallID.
	if payload.CallID == "" {
		payload.CallID = string(saved.ID)
		envelope.Payload = payload
	}
	return s.deps.Publisher.Publish(ctx, envelope)
}

// enrichIntegration best-effort backfills IntegrationID + BusinessEndpointID
// on a call row using the endpoint lookup. Used by recording + transcript
// short-circuits so bootstrap rows land with the linkage the download +
// GetRecording paths need. Silent no-op when endpoints repo isn't wired.
func (s *Service) enrichIntegration(ctx context.Context, orgID organization.ID, row *call.Call, externalID string) {
	if s.deps.Endpoints == nil || externalID == "" {
		return
	}
	ep, err := s.deps.Endpoints.FindByExternalID(ctx, orgID, row.Provider, externalID)
	if err != nil {
		return
	}
	bid := ep.ID
	row.BusinessEndpointID = &bid
	row.IntegrationID = ep.IntegrationID
}

// Get returns a single Call.
func (s *Service) Get(ctx context.Context, orgID organization.ID, id call.ID) (call.Call, error) {
	c, err := s.deps.Repo.Get(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, call.ErrNotFound) {
			return call.Call{}, ErrCallNotFound
		}
		return call.Call{}, err
	}
	return c, nil
}

// List returns a page of Call rows for the org.
func (s *Service) List(ctx context.Context, orgID organization.ID, filter repository.CallListFilter) (repository.CallPage, error) {
	return s.deps.Repo.List(ctx, orgID, filter)
}

// ResolveContactNames batch-resolves display names for the given contact
// ids under the org. Missing rows are omitted. Phase 1 shortcut: does N
// single Gets since ContactRepo does not expose a bulk fetch yet — the
// call list is capped at 200 rows so the fan-out is bounded.
//
// Returns nil when Contacts is not wired (silent no-op — callers still
// render a DTO, just without contact_name populated).
func (s *Service) ResolveContactNames(ctx context.Context, orgID organization.ID, ids []contact.ID) map[contact.ID]string {
	if s.deps.Contacts == nil || len(ids) == 0 {
		return nil
	}
	// Dedup ids so we don't Get the same row twice per page.
	seen := make(map[contact.ID]struct{}, len(ids))
	out := make(map[contact.ID]string, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		c, err := s.deps.Contacts.Get(ctx, orgID, id)
		if err != nil {
			continue
		}
		if c.DisplayName != "" {
			out[id] = c.DisplayName
		}
	}
	return out
}

// Answer instructs the provider to accept an inbound call (bare accept,
// no browser-side WebRTC handshake). Retained for callers that don't ship
// SDP; new UI paths should call AnswerWithSession.
func (s *Service) Answer(ctx context.Context, orgID organization.ID, id call.ID) error {
	return s.dispatch(ctx, orgID, id, func(p calling.Provider, providerCallID string) error {
		if !p.Capabilities().AnswerInbound {
			return call.ErrProviderUnsupported
		}
		return p.AnswerCall(ctx, providerCallID, nil)
	}, call.StatusAnswered)
}

// AnswerWithSession accepts an inbound call and forwards the browser's
// WebRTC answer SDP + recording/transcription preferences to the
// provider. Recording / transcription may be nil to leave the feature off.
func (s *Service) AnswerWithSession(
	ctx context.Context,
	orgID organization.ID,
	id call.ID,
	answerSDP string,
	recording *calling.RecordingOptions,
	transcription *calling.TranscriptionOptions,
) error {
	opts := &calling.AnswerOptions{
		AnswerSDP:     answerSDP,
		Recording:     recording,
		Transcription: transcription,
	}
	return s.dispatch(ctx, orgID, id, func(p calling.Provider, providerCallID string) error {
		if !p.Capabilities().AnswerInbound {
			return call.ErrProviderUnsupported
		}
		return p.AnswerCall(ctx, providerCallID, opts)
	}, call.StatusAnswered)
}

// GetOfferSDP returns the SDP offer previously captured from the
// provider's `connect` webhook for id. Returns empty strings when the
// row has no offer stored (e.g. business-initiated outbound where the
// offer originates on our side, or a call whose webhook did not carry a
// session block). The caller renders 404 in that case.
func (s *Service) GetOfferSDP(ctx context.Context, orgID organization.ID, id call.ID) (sdp, sdpType string, err error) {
	c, err := s.deps.Repo.Get(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, call.ErrNotFound) {
			return "", "", ErrCallNotFound
		}
		return "", "", err
	}
	if c.Extras == nil {
		return "", "", nil
	}
	sdp, _ = c.Extras["session_sdp"].(string)
	sdpType, _ = c.Extras["session_sdp_type"].(string)
	if sdpType == "" && sdp != "" {
		sdpType = "offer"
	}
	return sdp, sdpType, nil
}

// Reject instructs the provider to reject an inbound call.
func (s *Service) Reject(ctx context.Context, orgID organization.ID, id call.ID, reason string) error {
	return s.dispatch(ctx, orgID, id, func(p calling.Provider, providerCallID string) error {
		if !p.Capabilities().Reject {
			return call.ErrProviderUnsupported
		}
		return p.RejectCall(ctx, providerCallID, reason)
	}, call.StatusDeclined)
}

// End instructs the provider to terminate an in-progress call.
func (s *Service) End(ctx context.Context, orgID organization.ID, id call.ID) error {
	return s.dispatch(ctx, orgID, id, func(p calling.Provider, providerCallID string) error {
		if !p.Capabilities().Terminate {
			return call.ErrProviderUnsupported
		}
		return p.EndCall(ctx, providerCallID)
	}, call.StatusCompleted)
}

// GetRecording streams a call's recording bytes.
func (s *Service) GetRecording(ctx context.Context, orgID organization.ID, id call.ID) (io.ReadCloser, string, error) {
	c, err := s.deps.Repo.Get(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, call.ErrNotFound) {
			return nil, "", ErrCallNotFound
		}
		return nil, "", err
	}
	// Prefer the durable HBase copy when the webhook captured it. Older
	// rows (or rows where the store wasn't wired) fall through to the
	// Meta proxy path below.
	if s.deps.Attachments != nil {
		if k, ok := c.Extras["recording_key"].(string); ok && k != "" {
			body, gerr := s.deps.Attachments.Get(ctx, k)
			if gerr == nil {
				ctype := "audio/ogg"
				if v, ok := c.Extras["recording_content_type"].(string); ok && v != "" {
					ctype = v
				}
				return body, ctype, nil
			}
			// fall through on cache miss
		}
	}
	if c.IntegrationID == "" {
		return nil, "", ErrCallIntegrationMissing
	}
	integ, secrets, err := s.deps.Integrations.GetWithSecrets(ctx, orgID, integration.ID(c.IntegrationID))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrCallIntegrationMissing, err)
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	if v, ok := integ.Config["phone_number_id"].(string); ok && v != "" {
		secrets["phone_number_id"] = v
	}
	if v, ok := integ.Config["waba_id"].(string); ok && v != "" {
		secrets["waba_id"] = v
	}
	secrets["_integration_id"] = string(integ.ID)
	secrets["_org_id"] = string(integ.OrgID)
	provider, err := s.deps.CallingProviders.Calling(ctx, integ.Provider, secrets)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrCallIntegrationMissing, err)
	}
	// The provider adapter expects a media asset id, NOT the wacid. The
	// call_recording_available webhook stashed the media id in Extras
	// under `recording_media_id` — prefer that. Fall back to the wacid
	// only when no media id was captured (older rows) and let the
	// adapter's error clearly diagnose the miss.
	mediaID := c.ProviderCallID
	if c.Extras != nil {
		if v, ok := c.Extras["recording_media_id"].(string); ok && v != "" {
			mediaID = v
		}
	}
	return provider.GetRecording(ctx, mediaID)
}

// GetTranscript resolves the call row, verifies a transcription_ref is
// present, and asks the provider for the raw transcript document. The
// bytes are returned as-is so the REST handler can serve them straight
// through — the shape mirrors Meta's transcript document (metadata +
// segments).
func (s *Service) GetTranscript(
	ctx context.Context,
	orgID organization.ID,
	id call.ID,
) (json.RawMessage, error) {
	c, err := s.deps.Repo.Get(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, call.ErrNotFound) {
			return nil, ErrCallNotFound
		}
		return nil, err
	}
	if c.TranscriptionRef == "" {
		return nil, ErrTranscriptNotAvailable
	}
	// Prefer the durable HBase copy when the webhook captured it. Older
	// rows (or rows where the store wasn't wired) fall through to the
	// Meta proxy path below.
	if s.deps.Attachments != nil {
		if k, ok := c.Extras["transcript_key"].(string); ok && k != "" {
			body, gerr := s.deps.Attachments.Get(ctx, k)
			if gerr == nil {
				defer func() { _ = body.Close() }()
				return io.ReadAll(body)
			}
			// fall through on cache miss
		}
	}
	if c.IntegrationID == "" {
		return nil, ErrCallIntegrationMissing
	}
	provider, err := s.resolveCallingProvider(ctx, orgID, integration.ID(c.IntegrationID))
	if err != nil {
		return nil, err
	}
	return provider.GetTranscript(ctx, c.TranscriptionRef)
}

// dispatch resolves the provider for the target call and invokes op. On
// success, updates the call status to next.
func (s *Service) dispatch(
	ctx context.Context,
	orgID organization.ID,
	id call.ID,
	op func(p calling.Provider, providerCallID string) error,
	next call.Status,
) error {
	c, err := s.deps.Repo.Get(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, call.ErrNotFound) {
			return ErrCallNotFound
		}
		return err
	}
	if c.IntegrationID == "" {
		return ErrCallIntegrationMissing
	}
	integ, secrets, err := s.deps.Integrations.GetWithSecrets(ctx, orgID, integration.ID(c.IntegrationID))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCallIntegrationMissing, err)
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	if v, ok := integ.Config["phone_number_id"].(string); ok && v != "" {
		secrets["phone_number_id"] = v
	}
	if v, ok := integ.Config["waba_id"].(string); ok && v != "" {
		secrets["waba_id"] = v
	}
	secrets["_integration_id"] = string(integ.ID)
	secrets["_org_id"] = string(integ.OrgID)
	provider, err := s.deps.CallingProviders.Calling(ctx, integ.Provider, secrets)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCallIntegrationMissing, err)
	}
	if err := op(provider, c.ProviderCallID); err != nil {
		return err
	}
	now := s.deps.Clock.Now().UTC()
	if err := s.deps.Repo.UpdateStatus(ctx, orgID, id, next, now); err != nil {
		s.deps.Logger.Warn("call.dispatch status write failed",
			slog.String("org_id", string(orgID)),
			slog.String("call_id", string(id)),
			slog.Any("err", err),
		)
	}
	return nil
}

// findByProviderID scans the list for a matching provider_call_id. A
// dedicated repo method would be cleaner; kept as a scan here to avoid
// widening the port surface for a single call site.
func (s *Service) findByProviderID(
	ctx context.Context,
	orgID organization.ID,
	provider, providerCallID string,
) (call.Call, error) {
	// Bounded page scan — we look at the newest 200 calls, which covers
	// the recording-arrives-after-terminate window comfortably.
	page, err := s.deps.Repo.List(ctx, orgID, repository.CallListFilter{Limit: 200})
	if err != nil {
		return call.Call{}, err
	}
	for _, c := range page.Items {
		if c.Provider == provider && c.ProviderCallID == providerCallID {
			return c, nil
		}
	}
	return call.Call{}, call.ErrNotFound
}

// GetPermission resolves the provider for integrationID and asks it for
// the current call-permission state of the recipient. Returns the
// port-neutral shape so REST handlers can render it directly.
func (s *Service) GetPermission(
	ctx context.Context,
	orgID organization.ID,
	integrationID integration.ID,
	waID string,
) (calling.Permission, error) {
	provider, err := s.resolveCallingProvider(ctx, orgID, integrationID)
	if err != nil {
		return calling.Permission{}, err
	}
	return provider.GetPermission(ctx, waID, "")
}

// SendPermissionRequest resolves the provider for integrationID and sends
// an interactive call_permission_request to waID. Returns the wamid.
// The prompt is optional; adapters supply a sensible default when empty.
func (s *Service) SendPermissionRequest(
	ctx context.Context,
	orgID organization.ID,
	integrationID integration.ID,
	waID, prompt string,
) (string, error) {
	if waID == "" {
		return "", fmt.Errorf("%w: waID required", ErrCallValidation)
	}
	provider, err := s.resolveCallingProvider(ctx, orgID, integrationID)
	if err != nil {
		return "", err
	}
	return provider.SendPermissionRequest(ctx, waID, prompt)
}

// resolveCallingProvider is the shared load-integration-and-adapter path
// used by GetPermission / SendPermissionRequest. Mirrors the enrichment
// done in RequestCall / dispatch so credential wire-up stays uniform.
func (s *Service) resolveCallingProvider(
	ctx context.Context,
	orgID organization.ID,
	id integration.ID,
) (calling.Provider, error) {
	integ, secrets, err := s.deps.Integrations.GetWithSecrets(ctx, orgID, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCallIntegrationMissing, err)
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	if v, ok := integ.Config["phone_number_id"].(string); ok && v != "" {
		secrets["phone_number_id"] = v
	}
	if v, ok := integ.Config["waba_id"].(string); ok && v != "" {
		secrets["waba_id"] = v
	}
	secrets["_integration_id"] = string(integ.ID)
	secrets["_org_id"] = string(integ.OrgID)
	provider, err := s.deps.CallingProviders.Calling(ctx, integ.Provider, secrets)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve provider %q: %w", ErrCallIntegrationMissing, integ.Provider, err)
	}
	return provider, nil
}

// injectCallMessage inserts a synthetic "call" system message into the
// conversation thread so operators see the call inline in the chat. The
// message carries call_id in its metadata so the frontend can navigate
// from the inline row to the full call detail.
//
// Dedup: FindByCallID(orgID, callID) checks whether a call message for
// this call id already exists — a repeated webhook delivery for the same
// call skips insertion. The check is keyed on metadata.call_id, which is
// stable per canonical call id — outbound retries + webhook replays that
// re-land the same call all resolve to the same call row (via the
// (org, provider, provider_call_id) unique index on `calls`) and therefore
// the same dedup key.
//
// Best-effort: any failure is logged + swallowed so the call event
// pipeline is never blocked on this.
// enrichLinkage resolves Contact + Session + Conversation for an inbound
// call using the caller's phone / BSUID and the row's already-resolved
// BusinessEndpointID. Mirrors the identity linkage flow in
// internal/application/message/inbound.handleInbound. Silent no-op when
// any prerequisite is missing — the row still persists, just without
// thread linkage (so no inline info messages).
func (s *Service) enrichLinkage(ctx context.Context, orgID organization.ID, row *call.Call, payload events.CallEventPayload) {
	if row.Direction != call.DirectionInbound {
		return
	}
	if s.deps.Identities == nil || s.deps.ContactIDs == nil {
		return
	}
	if s.deps.Contacts == nil || s.deps.Sessions == nil || s.deps.Conversations == nil {
		return
	}
	if row.BusinessEndpointID == nil {
		return
	}
	if payload.From == "" && payload.FromUserID == "" {
		return
	}

	// 1. Upsert Contact by phone identity (preferred) or BSUID identity.
	//    The Contact carries a display name from Meta's contacts block
	//    when available. Skip name update if the row already exists.
	now := s.deps.Clock.Now().UTC()
	contactID := contact.ID(s.deps.ContactIDs.NewContactID())
	// CallEventPayload doesn't carry a display name today; use the phone
	// number (or BSUID) as the placeholder name — a later inbound
	// message webhook will overwrite it via the message-side flow.
	displayName := payload.From
	if displayName == "" {
		displayName = payload.FromUserID
	}
	c := contact.Contact{
		ID:          contactID,
		OrgID:       orgID,
		DisplayName: displayName,
		CreatedAt:   now,
		LastSeenAt:  &now,
	}
	if err := s.deps.Contacts.Upsert(ctx, c); err != nil {
		s.deps.Logger.Warn("call: contact upsert failed",
			slog.String("org_id", string(orgID)),
			slog.Any("err", err),
		)
		return
	}

	// 2. Attach phone identity (find-or-create). This resolves to an
	//    existing contact when the phone was seen before, or binds our
	//    newly-minted contact.
	phone := payload.From
	if phone != "" {
		normalized := normalizePhone(phone)
		ident, _, err := s.deps.Identities.FindOrCreate(
			ctx, orgID, contactID, identity.TypeWhatsApp, "whatsapp", phone, normalized,
		)
		if err == nil {
			contactID = ident.ContactID
		}
	}

	// 3. Also attach BSUID identity when present.
	if payload.FromUserID != "" {
		bsuidIdent, _, err := s.deps.Identities.FindOrCreate(
			ctx, orgID, contactID, identity.TypeBSUID, "whatsapp", payload.FromUserID, payload.FromUserID,
		)
		if err == nil {
			contactID = bsuidIdent.ContactID
		}
	}

	// 4. Open or reuse Session for (org, endpoint, contact).
	sess, err := s.deps.Sessions.FindOrCreateActive(ctx, orgID, *row.BusinessEndpointID, contactID)
	if err != nil {
		s.deps.Logger.Warn("call: session open failed",
			slog.String("org_id", string(orgID)),
			slog.Any("err", err),
		)
		return
	}

	// 5. Open or reuse Conversation for the session.
	conv, err := s.deps.Conversations.FindOrCreateOpen(ctx, orgID, sess.ID, contactID)
	if err != nil {
		s.deps.Logger.Warn("call: conversation open failed",
			slog.String("org_id", string(orgID)),
			slog.Any("err", err),
		)
		return
	}

	// Stamp linkage on the row.
	row.ContactID = &contactID
	row.SessionID = &sess.ID
	row.ConversationID = &conv.ID
}

// normalizePhone strips non-digits and returns E.164 without the '+'
// prefix — matching what the message inbound service does.
func normalizePhone(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			out = append(out, c)
		}
	}
	return string(out)
}

// callInfoLabel returns the human-readable label for a call-status info
// message and whether the status is terminal (clickable in the UI).
func callInfoLabel(direction call.Direction, status call.Status, durationSeconds int) (string, bool) {
	outbound := direction == call.DirectionOutbound
	switch status {
	case call.StatusRinging:
		if outbound {
			return "You called · ringing", false
		}
		return "Incoming call · ringing", false
	case call.StatusAnswered:
		return "Call answered", false
	case call.StatusInProgress:
		return "Call in progress", false
	case call.StatusCompleted:
		mm := durationSeconds / 60
		ss := durationSeconds % 60
		if outbound {
			return fmt.Sprintf("You called · %d:%02d", mm, ss), true
		}
		return fmt.Sprintf("Incoming call · %d:%02d", mm, ss), true
	case call.StatusFailed:
		return "Call failed", true
	case call.StatusMissed:
		return "Missed call", true
	case call.StatusDeclined:
		if outbound {
			return "Call declined", true
		}
		return "Rejected call", true
	case call.StatusNoAnswer:
		if outbound {
			return "You called · no answer", true
		}
		return "Missed call · no answer", true
	default:
		return string(status), false
	}
}

// injectCallInfoMessage inserts a thin centered "info" message into the
// conversation thread on every call status transition. One info message
// per (call_id, status) tuple — dedup via FindByCallIDAndStatus. Terminal
// statuses (completed / failed / missed / etc) get clickable=true in
// metadata so the frontend can wrap the row in a Link to /calls?id=...
// The old TypeCall path is deprecated but existing rows keep rendering.
func (s *Service) injectCallInfoMessage(ctx context.Context, saved call.Call, envelope events.Envelope) {
	if s.deps.Messages == nil || s.deps.MessageIDs == nil {
		return
	}
	// Need a conversation to attach to.
	if saved.ContactID == nil || saved.SessionID == nil || saved.ConversationID == nil {
		return
	}
	// Special case: recording_available doesn't advance status but still
	// emits a distinct info row so operators see "recording available".
	statusKey := string(saved.Status)
	label := ""
	terminal := false
	if envelope.Type == events.CallRecordingCreated {
		statusKey = "recording_available"
		label = "Recording available"
		terminal = true
	} else {
		label, terminal = callInfoLabel(saved.Direction, saved.Status, saved.DurationSeconds)
	}

	// Dedup: one info message per (call_id, status).
	if existing, ferr := s.deps.Messages.FindByCallIDAndStatus(ctx, saved.OrgID, string(saved.ID), statusKey); ferr == nil && existing.ID != "" {
		return
	}

	payload, _ := envelope.Payload.(events.CallEventPayload)
	direction := message.DirectionInbound
	if saved.Direction == call.DirectionOutbound {
		direction = message.DirectionOutbound
	}
	meta := map[string]any{
		"kind":           "call_status",
		"call_id":        string(saved.ID),
		"call_status":    statusKey,
		"call_direction": string(saved.Direction),
		"call_duration":  saved.DurationSeconds,
		"terminal":       terminal,
		"label":          label,
	}
	now := s.deps.Clock.Now().UTC()
	msg := message.Message{
		ID:                message.ID(s.deps.MessageIDs.NewMessageID()),
		OrgID:             saved.OrgID,
		ContactID:         *saved.ContactID,
		SessionID:         *saved.SessionID,
		ConversationID:    *saved.ConversationID,
		Channel:           "whatsapp",
		Provider:          saved.Provider,
		Direction:         direction,
		SenderIdentity:    saved.From,
		RecipientIdentity: saved.To,
		MessageType:       message.TypeInfo,
		Status:            message.StatusDelivered,
		CreatedAt:         now,
		Metadata:          meta,
	}
	if err := s.deps.Messages.Create(ctx, msg); err != nil {
		s.deps.Logger.Warn("call: inject info message failed",
			slog.String("org_id", string(saved.OrgID)),
			slog.String("call_id", string(saved.ID)),
			slog.String("status", statusKey),
			slog.Any("err", err),
		)
		return
	}

	_ = s.deps.Publisher.Publish(ctx, events.Envelope{
		Type:           events.MessageReceived,
		OrganizationID: string(saved.OrgID),
		OccurredAt:     now,
		CorrelationID:  envelope.CorrelationID,
		CausationID:    envelope.CausationID,
		Payload: events.MessageReceivedPayload{
			Provider:          saved.Provider,
			Channel:           "whatsapp",
			ProviderMessageID: "",
			From:              payload.From,
			FromUserID:        payload.FromUserID,
			To:                payload.To,
			MessageType:       string(message.TypeInfo),
			Timestamp:         now,
			ConversationID:    string(*saved.ConversationID),
			Raw:               meta,
		},
	})
}

// statusFromEventType maps an event Type + hangup reason onto a Call.Status.
func statusFromEventType(t events.Type, hangupReason string) call.Status {
	switch t {
	case events.CallInitiated, events.CallRinging:
		return call.StatusRinging
	case events.CallAnswered:
		return call.StatusAnswered
	case events.CallEnded, events.CallEndedDetailed:
		return call.StatusCompleted
	case events.CallFailed:
		switch hangupReason {
		case "declined", "rejected":
			return call.StatusDeclined
		case "no_answer", "missed":
			return call.StatusNoAnswer
		default:
			return call.StatusFailed
		}
	case events.CallRecordingCreated:
		return call.StatusCompleted
	}
	return call.StatusQueued
}

