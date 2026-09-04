package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	dcall "github.com/fullwa/fullwa/internal/domain/providercall"
)

// GroupSummary is the compact shape returned by ListGroups. Deliberately a
// leaf DTO — nothing in this file references domain/group so the adapter
// stays free of a domain-package back-reference the port would otherwise
// require.
//
// Reference: ~/Documents/whatsapp_doc_tracker/docs/groups/reference.md
// section "Get active groups".
type GroupSummary struct {
	// ProviderGroupID is Meta's opaque group id string.
	ProviderGroupID string
	// Subject is the human-visible group name.
	Subject string
	// CreatedAtUnix is the group creation timestamp Meta reports; 0 when
	// the field is absent from the response.
	CreatedAtUnix int64
}

// GroupDetail is the extended shape returned by GetGroup with fields=
// subject,description,participants,join_approval_mode,creation_timestamp,
// suspended,total_participant_count.
//
// Reference: groups/reference.md section "Get group info".
type GroupDetail struct {
	ProviderGroupID       string
	Subject               string
	Description           string
	CreationTimestampUnix int64
	Suspended             bool
	JoinApprovalMode      string
	TotalParticipantCount int
	Participants          []GroupMember
}

// GroupMember is a participant row surfaced by GetGroup / ListGroupMembers.
// Meta only ships wa_id today for the participants list; the caller decides
// whether to reconcile it into a full Contact.
type GroupMember struct {
	WaID string
	// BSUID is the participant's business-scoped user id when Meta ships
	// one. Empty for participants Meta identified only by wa_id.
	BSUID string
	// Role is one of "member" / "admin" / "superadmin". The reference
	// endpoint doesn't currently return per-participant role in the
	// participants array — the field is retained so the parser can fill it
	// once Meta exposes it (already surfaced on group_participants_update
	// webhooks).
	Role string
}

// ListGroups fetches the current tenant's active groups.
// Endpoint: GET /<phone_number_id>/groups?limit=<n>&after=<cursor>.
// Returns the summaries plus the after-cursor Meta returns for pagination;
// an empty afterCursor means the page is final.
//
// Reference: groups/reference.md section "Get active groups".
func (p *Provider) ListGroups(ctx context.Context) ([]GroupSummary, error) {
	url := fmt.Sprintf("%s/%s/%s/groups?limit=%d",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID, defaultGroupsPageSize)
	var resp metaGroupsListResponse
	if err := p.client.doJSON(ctx, string(dcall.OpListGroups), http.MethodGet, url, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]GroupSummary, 0, len(resp.Data.Groups))
	for _, g := range resp.Data.Groups {
		out = append(out, GroupSummary{
			ProviderGroupID: g.ID,
			Subject:         g.Subject,
			CreatedAtUnix:   parseUnixString(g.CreatedAt),
		})
	}
	return out, nil
}

// GetGroup fetches the full metadata for one group. `providerGroupID` is
// Meta's opaque group id string. The requested field list matches the
// full-detail set the operator UI renders; adapters may vary the fields at
// their discretion.
//
// Reference: groups/reference.md section "Get group info".
func (p *Provider) GetGroup(ctx context.Context, providerGroupID string) (GroupDetail, error) {
	if providerGroupID == "" {
		return GroupDetail{}, fmt.Errorf("whatsapp: GetGroup: providerGroupID required")
	}
	const fields = "subject,description,participants,join_approval_mode,creation_timestamp,suspended,total_participant_count"
	url := fmt.Sprintf("%s/%s/%s?fields=%s",
		p.cfg.baseURL(), p.cfg.version(), providerGroupID, fields)
	var resp metaGroupInfoResponse
	if err := p.client.doJSON(ctx, string(dcall.OpGetGroup), http.MethodGet, url, nil, &resp); err != nil {
		return GroupDetail{}, err
	}
	detail := GroupDetail{
		ProviderGroupID:       providerGroupID,
		Subject:               resp.Subject,
		Description:           resp.Description,
		CreationTimestampUnix: parseUnixString(resp.CreationTimestamp),
		Suspended:             resp.Suspended,
		JoinApprovalMode:      resp.JoinApprovalMode,
		TotalParticipantCount: resp.TotalParticipantCount,
	}
	detail.Participants = make([]GroupMember, 0, len(resp.Participants))
	for _, m := range resp.Participants {
		detail.Participants = append(detail.Participants, GroupMember{
			WaID: m.WaID,
			// Role is not populated by the current Meta reference response;
			// left blank so downstream defaults to RoleMember.
		})
	}
	return detail, nil
}

// ListGroupMembers is a convenience wrapper around GetGroup restricted to
// the participants field. Emits its own operation tag so operators can
// distinguish a members-only fetch from a full group fetch in the tracer.
//
// Reference: groups/reference.md section "Get group info".
func (p *Provider) ListGroupMembers(ctx context.Context, providerGroupID string) ([]GroupMember, error) {
	if providerGroupID == "" {
		return nil, fmt.Errorf("whatsapp: ListGroupMembers: providerGroupID required")
	}
	url := fmt.Sprintf("%s/%s/%s?fields=participants",
		p.cfg.baseURL(), p.cfg.version(), providerGroupID)
	var resp metaGroupInfoResponse
	if err := p.client.doJSON(ctx, string(dcall.OpListGroupMembers), http.MethodGet, url, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]GroupMember, 0, len(resp.Participants))
	for _, m := range resp.Participants {
		out = append(out, GroupMember{WaID: m.WaID})
	}
	return out, nil
}

// CreateGroupRequest is the input DTO for CreateGroup.
type CreateGroupRequest struct {
	// Subject is the human-visible group name (required, ≤128 chars).
	Subject string
	// Description is the optional group description (≤2048 chars).
	Description string
	// JoinApprovalMode is one of "" (server default) | "auto_approve" |
	// "approval_required".
	JoinApprovalMode string
}

// CreateGroupResult is the output DTO for CreateGroup.
type CreateGroupResult struct {
	// ProviderGroupID is Meta's opaque group id string returned
	// synchronously. The invite_link arrives asynchronously via the
	// group_lifecycle_update webhook.
	ProviderGroupID string
}

// CreateGroup creates a new WhatsApp group under the current phone number.
// Endpoint: POST /<phone_number_id>/groups.
//
// Reference: groups/reference.md section "Create group".
func (p *Provider) CreateGroup(ctx context.Context, req CreateGroupRequest) (CreateGroupResult, error) {
	if req.Subject == "" {
		return CreateGroupResult{}, fmt.Errorf("whatsapp: CreateGroup: subject required")
	}
	body := map[string]any{
		"messaging_product": "whatsapp",
		"subject":           req.Subject,
	}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if req.JoinApprovalMode != "" {
		body["join_approval_mode"] = req.JoinApprovalMode
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return CreateGroupResult{}, fmt.Errorf("whatsapp: CreateGroup marshal: %w", err)
	}
	url := fmt.Sprintf("%s/%s/%s/groups",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID)
	var resp metaCreateGroupResponse
	if err := p.client.doJSON(ctx, string(dcall.OpCreateGroup), http.MethodPost, url, raw, &resp); err != nil {
		return CreateGroupResult{}, err
	}
	id := resp.ID
	if id == "" && len(resp.Groups) > 0 {
		id = resp.Groups[0].ID
	}
	if id == "" {
		return CreateGroupResult{}, fmt.Errorf("whatsapp: CreateGroup: missing id in response")
	}
	return CreateGroupResult{ProviderGroupID: id}, nil
}

// metaCreateGroupResponse is the wire shape Meta returns from POST /groups.
// Two variants observed in the wild: top-level {id} or {groups:[{id}]}.
type metaCreateGroupResponse struct {
	ID     string `json:"id"`
	Groups []struct {
		ID string `json:"id"`
	} `json:"groups"`
}

// ---------- Meta wire types (adapter-local; NEVER leak out of this package) ----------

const defaultGroupsPageSize = 25

type metaGroupsListResponse struct {
	Data struct {
		Groups []struct {
			ID        string          `json:"id"`
			Subject   string          `json:"subject"`
			CreatedAt json.RawMessage `json:"created_at"`
		} `json:"groups"`
	} `json:"data"`
	Paging struct {
		Cursors struct {
			Before string `json:"before"`
			After  string `json:"after"`
		} `json:"cursors"`
	} `json:"paging"`
}

type metaGroupInfoResponse struct {
	ID                    string          `json:"id"`
	Subject               string          `json:"subject"`
	Description           string          `json:"description"`
	CreationTimestamp     json.RawMessage `json:"creation_timestamp"`
	Suspended             bool            `json:"suspended"`
	JoinApprovalMode      string          `json:"join_approval_mode"`
	TotalParticipantCount int             `json:"total_participant_count"`
	Participants          []struct {
		WaID string `json:"wa_id"`
	} `json:"participants"`
}

// parseUnixString accepts either a JSON number or a JSON string carrying an
// integer Unix timestamp. Meta's Graph API sometimes ships the field as one
// or the other depending on which sub-field of the same response you asked
// for; a small helper keeps the call sites tidy.
func parseUnixString(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	// JSON string?
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0
	}
	return n
}
