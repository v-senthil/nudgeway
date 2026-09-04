// Package group is the use-case layer for the Group entity — sync from the
// provider, list, get, roster + send-to-group. Provider-agnostic: nothing in
// this file imports internal/providers/*.
package group

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	dgroup "github.com/v-senthil/nudgeway/internal/domain/group"
	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/channel"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// SyncResult reports what a Sync call produced.
type SyncResult struct {
	// GroupsUpserted is the number of Group rows created or refreshed.
	GroupsUpserted int
	// MembersUpserted is the total number of Member rows created or
	// refreshed across all synced groups.
	MembersUpserted int
}

// IntegrationSecrets exposes decrypted integration secrets to the group
// service. Mirrors the shape used by message.SendService.IntegrationSecrets
// so the wire-up can share one concrete infra implementation.
type IntegrationSecrets interface {
	repository.IntegrationRepo

	// GetWithSecrets returns the Integration alongside a map of decrypted
	// secret material.
	GetWithSecrets(
		ctx context.Context,
		orgID organization.ID,
		id integration.ID,
	) (integration.Integration, map[string]string, error)
}

// ProviderRegistry looks up a channel.Provider adapter by provider key,
// binding it to a specific integration's decrypted secrets. Same interface
// shape as the send service so wire-up in cmd/server can share one
// implementation across use cases.
type ProviderRegistry interface {
	Channel(ctx context.Context, providerKey string, secrets map[string]string) (channel.Provider, error)
}

// ProviderGroupsClient is the narrow surface a channel adapter exposes to
// the group use case beyond the base channel.Provider port. It is declared
// here (rather than on channel.Provider) so a channel that has no notion of
// groups does not have to stub out all three methods; the group service
// type-asserts at call time and returns ErrUnsupported when the adapter
// does not implement this.
//
// The shapes are provider-agnostic — the WhatsApp adapter's GroupSummary /
// GroupDetail / GroupMember types satisfy the same field-set naturally, but
// the application layer only sees the anonymous struct fields declared
// here.
type ProviderGroupsClient interface {
	// ListGroups fetches the current set of groups the integration knows
	// about. Order is provider-defined.
	ListGroups(ctx context.Context) ([]ProviderGroupSummary, error)
	// GetGroup fetches full metadata + roster for one group.
	GetGroup(ctx context.Context, providerGroupID string) (ProviderGroupDetail, error)
	// ListGroupMembers is a members-only sub-fetch.
	ListGroupMembers(ctx context.Context, providerGroupID string) ([]ProviderGroupMember, error)
	// CreateGroup creates a new group on the provider. Returns the
	// provider-side id synchronously; the invite link (when applicable)
	// arrives asynchronously via a lifecycle webhook.
	CreateGroup(ctx context.Context, req ProviderCreateGroupRequest) (ProviderCreateGroupResult, error)
}

// ProviderCreateGroupRequest is the provider-agnostic input for CreateGroup.
type ProviderCreateGroupRequest struct {
	Subject          string
	Description      string
	JoinApprovalMode string
}

// ProviderCreateGroupResult is the provider-agnostic output for CreateGroup.
type ProviderCreateGroupResult struct {
	ProviderGroupID string
}

// ProviderGroupSummary is the compact provider-native summary.
type ProviderGroupSummary struct {
	ProviderGroupID string
	Subject         string
	CreatedAtUnix   int64
}

// ProviderGroupDetail is the extended provider-native shape.
type ProviderGroupDetail struct {
	ProviderGroupID       string
	Subject               string
	Description           string
	CreationTimestampUnix int64
	Suspended             bool
	JoinApprovalMode      string
	TotalParticipantCount int
	Participants          []ProviderGroupMember
}

// ProviderGroupMember is the provider-native participant row.
type ProviderGroupMember struct {
	WaID  string
	BSUID string
	Role  string
}

// SendService is the minimal outbound-send surface the group service needs.
// Wired to the existing application/message.SendService in cmd/server. The
// group service does not know about queues, conversations, or message rows;
// it just asks "please dispatch this to this group id".
type SendService interface {
	// SendToGroup asks the outbound pipeline to route a message body at a
	// group. Implementations construct the appropriate channel.SendRequest
	// (recipient_type=group, To=providerGroupID) and enqueue on the
	// existing send lane.
	SendToGroup(ctx context.Context, req SendToGroupRequest) (SendToGroupResponse, error)
}

// SendToGroupRequest is the DTO consumed by SendService.SendToGroup.
type SendToGroupRequest struct {
	OrgID          string
	ActorUserID    string
	GroupID        string
	Type           string
	Payload        []byte
	IdempotencyKey string
	RequestID      string
	CorrelationID  string
}

// SendToGroupResponse is the DTO returned by SendService.SendToGroup.
type SendToGroupResponse struct {
	MessageID string
	Status    string
}

// ErrUnsupported is returned by Sync / SendMessage when the resolved
// provider adapter does not implement ProviderGroupsClient — i.e. the
// integration is on a channel that has no groups concept (SMS, email, ...).
var ErrUnsupported = errors.New("group: provider does not support groups")

// ErrNoIntegration is returned when Sync is asked to run against an
// integration id that does not resolve to an active row for the org.
var ErrNoIntegration = errors.New("group: integration missing")

// ErrGroupNotFound is returned by Get / Members / SendMessage when the
// group is not persisted for the caller's org.
var ErrGroupNotFound = errors.New("group: not found")

// Clock returns wall-clock. Injected for deterministic tests.
type Clock interface {
	Now() time.Time
}

// Deps bundles the constructor arguments of Service.
type Deps struct {
	// Repo persists Group + Member rows.
	Repo repository.GroupRepo
	// Integrations reads integration rows + their decrypted secrets so the
	// service can bind the provider adapter.
	Integrations IntegrationSecrets
	// Providers resolves the channel adapter for a given (provider,
	// secrets). Reuses the ProviderRegistry pattern from appmsg.SendService
	// so cmd/server can wire one concrete registry.
	Providers ProviderRegistry
	// Send dispatches through the existing outbound pipeline. Nil disables
	// SendMessage; List / Get / Sync remain functional.
	Send SendService
	// Conversations persists Conversation rows. When set, Create will
	// idempotently ensure a Type=group Conversation row exists for each
	// newly-created Group so the inbox list surfaces it. Nil is tolerated
	// for backwards-compatible tests but production wire-up sets it.
	Conversations repository.ConversationRepo
	// Clock provides deterministic time. Nil defaults to systemClock{}.
	Clock Clock
	// Logger receives structured records with org_id, group_id.
	Logger *slog.Logger
}

// Service implements the group use cases.
type Service struct {
	repo          repository.GroupRepo
	integrations  IntegrationSecrets
	providers     ProviderRegistry
	send          SendService
	conversations repository.ConversationRepo
	clock         Clock
	log           *slog.Logger
}

// NewService constructs a Service. Panics on missing required deps.
func NewService(d Deps) *Service {
	if d.Repo == nil {
		panic("group.NewService: Repo required")
	}
	if d.Integrations == nil {
		panic("group.NewService: Integrations required")
	}
	if d.Providers == nil {
		panic("group.NewService: Providers required")
	}
	if d.Clock == nil {
		d.Clock = systemClock{}
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{
		repo:          d.Repo,
		integrations:  d.Integrations,
		providers:     d.Providers,
		send:          d.Send,
		conversations: d.Conversations,
		clock:         d.Clock,
		log:           d.Logger,
	}
}

// Sync fetches the current set of groups from the provider and upserts them
// into the repository. Also refreshes each group's member roster.
//
// The call fans out one GetGroup per listed summary so metadata + roster
// stay together. Errors on individual groups are logged and skipped so a
// single stale group id in the middle of the page cannot fail the whole
// sync.
func (s *Service) Sync(ctx context.Context, orgID organization.ID, integrationID integration.ID) (SyncResult, error) {
	integ, secrets, err := s.integrations.GetWithSecrets(ctx, orgID, integrationID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %w", ErrNoIntegration, err)
	}
	// Match the wire-up convention used by SendService: seed the secrets
	// map with the non-secret config fields the provider factory needs so
	// downstream construction does not have to re-query the integration.
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

	prov, err := s.providers.Channel(ctx, integ.Provider, secrets)
	if err != nil {
		return SyncResult{}, fmt.Errorf("resolve provider %q: %w", integ.Provider, err)
	}
	gc, ok := prov.(ProviderGroupsClient)
	if !ok {
		return SyncResult{}, ErrUnsupported
	}

	summaries, err := gc.ListGroups(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list groups: %w", err)
	}

	res := SyncResult{}
	now := s.clock.Now().UTC()
	for _, sum := range summaries {
		detail, err := gc.GetGroup(ctx, sum.ProviderGroupID)
		if err != nil {
			s.log.Warn("group.sync: get_group failed; skipping",
				slog.String("org_id", string(orgID)),
				slog.String("provider_group_id", sum.ProviderGroupID),
				slog.Any("err", err),
			)
			continue
		}
		g := dgroup.Group{
			OrgID:           orgID,
			IntegrationID:   integ.ID,
			ProviderGroupID: sum.ProviderGroupID,
			Subject:         firstNonEmpty(detail.Subject, sum.Subject),
			Description:     detail.Description,
			Size:            detail.TotalParticipantCount,
			IsAdmin:         true, // groups the business phone number owns are always its own admin
			Metadata: map[string]any{
				"join_approval_mode": detail.JoinApprovalMode,
				"suspended":          detail.Suspended,
				"creation_timestamp": detail.CreationTimestampUnix,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		saved, err := s.repo.Upsert(ctx, g)
		if err != nil {
			s.log.Warn("group.sync: upsert group failed; skipping",
				slog.String("org_id", string(orgID)),
				slog.String("provider_group_id", sum.ProviderGroupID),
				slog.Any("err", err),
			)
			continue
		}
		res.GroupsUpserted++
		for _, p := range detail.Participants {
			role := dgroup.Role(p.Role)
			if !dgroup.ValidRole(role) {
				role = dgroup.RoleMember
			}
			m := dgroup.Member{
				OrgID:    orgID,
				GroupID:  saved.ID,
				WaID:     p.WaID,
				BSUID:    p.BSUID,
				Role:     role,
				JoinedAt: now,
			}
			if err := s.repo.AddMember(ctx, m); err != nil {
				s.log.Warn("group.sync: add_member failed",
					slog.String("org_id", string(orgID)),
					slog.String("group_id", string(saved.ID)),
					slog.String("wa_id", p.WaID),
					slog.Any("err", err),
				)
				continue
			}
			res.MembersUpserted++
		}
	}
	s.log.Info("group.sync complete",
		slog.String("org_id", string(orgID)),
		slog.String("integration_id", string(integ.ID)),
		slog.Int("groups_upserted", res.GroupsUpserted),
		slog.Int("members_upserted", res.MembersUpserted),
	)
	return res, nil
}

// CreateInput bundles the fields needed to create a new group on the
// provider and persist it locally.
type CreateInput struct {
	IntegrationID    integration.ID
	Subject          string
	Description      string
	JoinApprovalMode string
}

// Create asks the provider to create a new group, then persists the
// returned provider group id under a fresh domain ULID. The invite link
// (when applicable) arrives asynchronously via a lifecycle webhook and is
// merged onto the row by the webhook consumer.
func (s *Service) Create(ctx context.Context, orgID organization.ID, in CreateInput) (dgroup.Group, error) {
	if in.Subject == "" {
		return dgroup.Group{}, fmt.Errorf("group.Create: subject required")
	}
	if in.IntegrationID == "" {
		return dgroup.Group{}, fmt.Errorf("group.Create: integration_id required")
	}
	integ, secrets, err := s.integrations.GetWithSecrets(ctx, orgID, in.IntegrationID)
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("%w: %w", ErrNoIntegration, err)
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

	prov, err := s.providers.Channel(ctx, integ.Provider, secrets)
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("resolve provider %q: %w", integ.Provider, err)
	}
	gc, ok := prov.(ProviderGroupsClient)
	if !ok {
		return dgroup.Group{}, ErrUnsupported
	}
	res, err := gc.CreateGroup(ctx, ProviderCreateGroupRequest{
		Subject:          in.Subject,
		Description:      in.Description,
		JoinApprovalMode: in.JoinApprovalMode,
	})
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("create group: %w", err)
	}
	now := s.clock.Now().UTC()
	g := dgroup.Group{
		OrgID:           orgID,
		IntegrationID:   integ.ID,
		ProviderGroupID: res.ProviderGroupID,
		Subject:         in.Subject,
		Description:     in.Description,
		Size:            0,
		IsAdmin:         true,
		Metadata: map[string]any{
			"join_approval_mode": in.JoinApprovalMode,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	saved, err := s.repo.Upsert(ctx, g)
	if err != nil {
		return dgroup.Group{}, fmt.Errorf("persist group: %w", err)
	}
	// Best-effort: mirror the group into the inbox by ensuring a
	// Type=group Conversation row exists. Idempotent on repeat creates via
	// the (org_id, group_id) unique key. Failure here is logged but does
	// NOT unwind the group create — the row can be materialised later by
	// a background sweeper or a manual sync.
	if s.conversations != nil {
		if _, cerr := s.conversations.EnsureGroupConversation(ctx, orgID, saved.ID); cerr != nil {
			s.log.Warn("group.create: conversation mirror failed",
				slog.String("org_id", string(orgID)),
				slog.String("group_id", string(saved.ID)),
				slog.Any("err", cerr),
			)
		}
	}
	s.log.Info("group.create complete",
		slog.String("org_id", string(orgID)),
		slog.String("integration_id", string(integ.ID)),
		slog.String("provider_group_id", res.ProviderGroupID),
		slog.String("group_id", string(saved.ID)),
	)
	return saved, nil
}

// List returns the groups for the org filtered by filter.
func (s *Service) List(ctx context.Context, orgID organization.ID, filter repository.GroupListFilter) ([]dgroup.Group, string, error) {
	rows, next, err := s.repo.List(ctx, orgID, filter)
	if err != nil {
		return nil, "", fmt.Errorf("list groups: %w", err)
	}
	return rows, next, nil
}

// Get returns one group by id.
func (s *Service) Get(ctx context.Context, orgID organization.ID, id dgroup.ID) (dgroup.Group, error) {
	g, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, dgroup.ErrNotFound) {
			return dgroup.Group{}, ErrGroupNotFound
		}
		return dgroup.Group{}, fmt.Errorf("get group: %w", err)
	}
	return g, nil
}

// Members returns the roster for a group.
func (s *Service) Members(ctx context.Context, orgID organization.ID, groupID dgroup.ID) ([]dgroup.Member, error) {
	// Validate the group belongs to the org first so a missing row returns
	// the sentinel rather than an empty roster (which would be
	// indistinguishable from an empty group).
	if _, err := s.Get(ctx, orgID, groupID); err != nil {
		return nil, err
	}
	members, err := s.repo.ListMembers(ctx, orgID, groupID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

// SendMessage dispatches a message payload to a group. Delegates to the
// underlying SendService — this method is a thin adapter that resolves the
// group row (for tenancy checks) and forwards.
func (s *Service) SendMessage(ctx context.Context, orgID organization.ID, actorUserID string, groupID dgroup.ID, msgType string, payload []byte, idempotencyKey, requestID string) (SendToGroupResponse, error) {
	if s.send == nil {
		return SendToGroupResponse{}, errors.New("group: send service not wired")
	}
	if _, err := s.Get(ctx, orgID, groupID); err != nil {
		return SendToGroupResponse{}, err
	}
	res, err := s.send.SendToGroup(ctx, SendToGroupRequest{
		OrgID:          string(orgID),
		ActorUserID:    actorUserID,
		GroupID:        string(groupID),
		Type:           msgType,
		Payload:        payload,
		IdempotencyKey: idempotencyKey,
		RequestID:      requestID,
		CorrelationID:  requestID,
	})
	if err != nil {
		return SendToGroupResponse{}, fmt.Errorf("send to group: %w", err)
	}
	return res, nil
}

// firstNonEmpty returns a if non-empty else b.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// systemClock returns wall-clock time.
type systemClock struct{}

// Now returns time.Now().UTC().
func (systemClock) Now() time.Time { return time.Now().UTC() }

// MarshalMetadata serialises a group's metadata bag into a stable JSON blob
// with sorted keys. Exposed so callers embedding a Group into a REST DTO
// can share the same serialisation as the persistence layer.
func MarshalMetadata(md map[string]any) ([]byte, error) {
	if md == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(md)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return b, nil
}
