package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	tmpldom "github.com/v-senthil/nudgeway/internal/domain/template"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// Templates implements repository.TemplateRepo against the templates table
// declared in migrations/20260904000003_templates.
type Templates struct {
	db *sql.DB
}

// NewTemplates constructs a Templates repository.
func NewTemplates(db *sql.DB) *Templates { return &Templates{db: db} }

// templateCols is the canonical SELECT column list for templates.
const templateCols = `id, org_id, integration_id, provider_template_id, name, language, category, status, components, variables, last_synced_at, created_at, updated_at`

// Create inserts a new template row. Returns tmpldom.ErrInvalid wrapping
// the duplicate-key error when the (org, integration, name, language)
// unique index rejects the insert — callers translate that into a 409/422.
func (r *Templates) Create(ctx context.Context, t tmpldom.Template) error {
	idBytes, err := ulidToBytes(string(t.ID))
	if err != nil {
		return fmt.Errorf("templates id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(t.OrgID))
	if err != nil {
		return fmt.Errorf("templates org: %w", err)
	}
	intBytes, err := ulidToBytes(string(t.IntegrationID))
	if err != nil {
		return fmt.Errorf("templates integration: %w", err)
	}
	comps, err := json.Marshal(orEmptyComponents(t.Components))
	if err != nil {
		return fmt.Errorf("templates components: %w", err)
	}
	vars, err := json.Marshal(orEmptyStringMap(t.Variables))
	if err != nil {
		return fmt.Errorf("templates variables: %w", err)
	}
	var syncedAt any
	if t.LastSyncedAt != nil {
		syncedAt = t.LastSyncedAt.UTC()
	}
	const q = `INSERT INTO templates
	    (id, org_id, integration_id, provider_template_id, name, language, category, status,
	     components, variables, last_synced_at)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, q,
		idBytes, orgBytes, intBytes, t.ProviderTemplateID, t.Name, t.Language,
		string(t.Category), string(t.Status), comps, vars, syncedAt,
	); err != nil {
		if isDuplicateErr(err) {
			return fmt.Errorf("%w: template (name, language) already exists", tmpldom.ErrInvalid)
		}
		return fmt.Errorf("templates insert: %w", err)
	}
	return nil
}

// Get returns a single template row by (org, id). Returns tmpldom.ErrNotFound
// when the row does not exist.
func (r *Templates) Get(ctx context.Context, orgID organization.ID, id tmpldom.ID) (tmpldom.Template, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("templates org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("templates id: %w", err)
	}
	q := "SELECT " + templateCols + " FROM templates WHERE org_id = ? AND id = ? LIMIT 1"
	row := r.db.QueryRowContext(ctx, q, orgBytes, idBytes)
	t, err := scanTemplate(row.Scan)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return tmpldom.Template{}, tmpldom.ErrNotFound
		}
		return tmpldom.Template{}, err
	}
	return t, nil
}

// FindByNameLanguage returns a single template row by its natural key
// (org, integration, name, language). Returns tmpldom.ErrNotFound when
// no row matches. Callers on the send path use this to enrich outbound
// message metadata with the resolved template text.
func (r *Templates) FindByNameLanguage(
	ctx context.Context,
	orgID organization.ID,
	integrationID integration.ID,
	name string,
	language string,
) (tmpldom.Template, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("templates org: %w", err)
	}
	intBytes, err := ulidToBytes(string(integrationID))
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("templates integration: %w", err)
	}
	q := "SELECT " + templateCols + " FROM templates WHERE org_id = ? AND integration_id = ? AND name = ? AND language = ? LIMIT 1"
	row := r.db.QueryRowContext(ctx, q, orgBytes, intBytes, name, language)
	t, err := scanTemplate(row.Scan)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return tmpldom.Template{}, tmpldom.ErrNotFound
		}
		return tmpldom.Template{}, err
	}
	return t, nil
}

// List returns a page of templates for one org filtered by IntegrationID
// and Status. Ordered by updated_at DESC (id tiebreaker). Cursor is opaque
// base64 encoding (updated_at_unix_micro | id_ulid).
func (r *Templates) List(ctx context.Context, orgID organization.ID, filter repository.TemplateListFilter) (repository.TemplatePage, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return repository.TemplatePage{}, fmt.Errorf("templates org: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	conds := []string{"org_id = ?"}
	args := []any{orgBytes}
	if filter.IntegrationID != nil {
		intBytes, err := ulidToBytes(string(*filter.IntegrationID))
		if err != nil {
			return repository.TemplatePage{}, fmt.Errorf("templates integration filter: %w", err)
		}
		conds = append(conds, "integration_id = ?")
		args = append(args, intBytes)
	}
	if filter.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.Cursor != "" {
		at, idBytes, err := decodeTemplateCursor(filter.Cursor)
		if err != nil {
			return repository.TemplatePage{}, err
		}
		conds = append(conds, "(updated_at, id) < (?, ?)")
		args = append(args, at.UTC(), idBytes)
	}
	q := "SELECT " + templateCols + " FROM templates WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY updated_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return repository.TemplatePage{}, fmt.Errorf("templates list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := repository.TemplatePage{}
	for rows.Next() {
		t, err := scanTemplate(rows.Scan)
		if err != nil {
			return repository.TemplatePage{}, err
		}
		out.Templates = append(out.Templates, t)
	}
	if err := rows.Err(); err != nil {
		return repository.TemplatePage{}, fmt.Errorf("templates rows: %w", err)
	}
	if len(out.Templates) > limit {
		last := out.Templates[limit-1]
		out.Templates = out.Templates[:limit]
		out.NextCursor = encodeTemplateCursor(last.UpdatedAt, string(last.ID))
	}
	return out, nil
}

// Upsert inserts or updates by the (org, integration, name, language)
// unique key. The row's ID is preserved when it already exists — callers
// mint a fresh ID for the create case, but the ON DUPLICATE branch keeps
// the incumbent id so foreign references stay valid.
func (r *Templates) Upsert(ctx context.Context, t tmpldom.Template) error {
	idBytes, err := ulidToBytes(string(t.ID))
	if err != nil {
		return fmt.Errorf("templates id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(t.OrgID))
	if err != nil {
		return fmt.Errorf("templates org: %w", err)
	}
	intBytes, err := ulidToBytes(string(t.IntegrationID))
	if err != nil {
		return fmt.Errorf("templates integration: %w", err)
	}
	comps, err := json.Marshal(orEmptyComponents(t.Components))
	if err != nil {
		return fmt.Errorf("templates components: %w", err)
	}
	vars, err := json.Marshal(orEmptyStringMap(t.Variables))
	if err != nil {
		return fmt.Errorf("templates variables: %w", err)
	}
	var syncedAt any
	if t.LastSyncedAt != nil {
		syncedAt = t.LastSyncedAt.UTC()
	}
	// ON DUPLICATE KEY UPDATE the mutable fields but keep the incumbent id
	// (id is not in the SET list). MySQL's `VALUES()` on the unique key
	// tuple is a no-op — we key on (org_id, integration_id, name, language)
	// and the SET list refreshes the payload.
	const q = `INSERT INTO templates
	    (id, org_id, integration_id, provider_template_id, name, language, category, status,
	     components, variables, last_synced_at)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	  ON DUPLICATE KEY UPDATE
	    provider_template_id = VALUES(provider_template_id),
	    category             = VALUES(category),
	    status               = VALUES(status),
	    components           = VALUES(components),
	    variables            = VALUES(variables),
	    last_synced_at       = VALUES(last_synced_at)`
	if _, err := r.db.ExecContext(ctx, q,
		idBytes, orgBytes, intBytes, t.ProviderTemplateID, t.Name, t.Language,
		string(t.Category), string(t.Status), comps, vars, syncedAt,
	); err != nil {
		return fmt.Errorf("templates upsert: %w", err)
	}
	return nil
}

// UpdateStatus advances the row's Status + last_synced_at without touching
// the components / variables. Idempotent.
func (r *Templates) UpdateStatus(ctx context.Context, orgID organization.ID, id tmpldom.ID, status tmpldom.Status, syncedAt time.Time) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("templates org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("templates id: %w", err)
	}
	const q = `UPDATE templates SET status = ?, last_synced_at = ? WHERE org_id = ? AND id = ?`
	if _, err := r.db.ExecContext(ctx, q, string(status), syncedAt.UTC(), orgBytes, idBytes); err != nil {
		return fmt.Errorf("templates update status: %w", err)
	}
	return nil
}

// Delete removes a template row by (org, id). Missing rows are a no-op.
func (r *Templates) Delete(ctx context.Context, orgID organization.ID, id tmpldom.ID) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("templates org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("templates id: %w", err)
	}
	const q = `DELETE FROM templates WHERE org_id = ? AND id = ?`
	if _, err := r.db.ExecContext(ctx, q, orgBytes, idBytes); err != nil {
		return fmt.Errorf("templates delete: %w", err)
	}
	return nil
}

// scanTemplate decodes a single row.
func scanTemplate(scan func(dest ...any) error) (tmpldom.Template, error) {
	var (
		id, org, intg []byte
		provTmplID    string
		name, lang    string
		category      string
		status        string
		compsBytes    []byte
		varsBytes     []byte
		lastSynced    sql.NullTime
		created       time.Time
		updated       time.Time
	)
	if err := scan(
		&id, &org, &intg, &provTmplID, &name, &lang, &category, &status,
		&compsBytes, &varsBytes, &lastSynced, &created, &updated,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tmpldom.Template{}, ErrNotFound
		}
		return tmpldom.Template{}, fmt.Errorf("templates scan: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("templates bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("templates bad org: %w", err)
	}
	intStr, err := ulidFromBytes(intg)
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("templates bad integration: %w", err)
	}
	out := tmpldom.Template{
		ID:                 tmpldom.ID(idStr),
		OrgID:              organization.ID(orgStr),
		IntegrationID:      integration.ID(intStr),
		ProviderTemplateID: provTmplID,
		Name:               name,
		Language:           lang,
		Category:           tmpldom.Category(category),
		Status:             tmpldom.Status(status),
		CreatedAt:          created,
		UpdatedAt:          updated,
	}
	if lastSynced.Valid {
		t := lastSynced.Time
		out.LastSyncedAt = &t
	}
	if len(compsBytes) > 0 {
		var comps []tmpldom.Component
		if err := json.Unmarshal(compsBytes, &comps); err != nil {
			return tmpldom.Template{}, fmt.Errorf("templates components json: %w", err)
		}
		out.Components = comps
	}
	if len(varsBytes) > 0 {
		var vars map[string]string
		if err := json.Unmarshal(varsBytes, &vars); err != nil {
			return tmpldom.Template{}, fmt.Errorf("templates variables json: %w", err)
		}
		out.Variables = vars
	}
	return out, nil
}

// orEmptyComponents returns cs or an empty slice so JSON never becomes 'null'.
func orEmptyComponents(cs []tmpldom.Component) []tmpldom.Component {
	if cs == nil {
		return []tmpldom.Component{}
	}
	return cs
}

// orEmptyStringMap is the string-valued sibling of orEmptyMap.
func orEmptyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// encodeTemplateCursor packs (updated_at, id) into an opaque base64 token.
// Same shape as encodeAuditCursor but with a ULID id column instead of a
// numeric row id.
func encodeTemplateCursor(at time.Time, id string) string {
	plain := strconv.FormatInt(at.UTC().UnixMicro(), 10) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(plain))
}

// decodeTemplateCursor unpacks the opaque cursor to (updated_at, id_bytes).
// Returns tmpldom.ErrInvalid on any parse failure so the REST edge maps it
// to a 400.
func decodeTemplateCursor(cursor string) (time.Time, []byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("%w: invalid cursor", tmpldom.ErrInvalid)
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, nil, fmt.Errorf("%w: invalid cursor", tmpldom.ErrInvalid)
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("%w: invalid cursor", tmpldom.ErrInvalid)
	}
	idBytes, err := ulidToBytes(parts[1])
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("%w: invalid cursor", tmpldom.ErrInvalid)
	}
	return time.UnixMicro(micros).UTC(), idBytes, nil
}
