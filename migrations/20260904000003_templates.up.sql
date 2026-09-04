-- 20260904000003_templates.up.sql
-- templates: provider-neutral message templates. WhatsApp Business Cloud API
-- calls them "message_templates" and stores them by WABA; other channels
-- surface the same shape. The canonical row is the tenant-scoped record;
-- provider_template_id keeps the pointer back to the provider after a
-- successful Create call so Sync can reconcile status callbacks.
--
-- Design notes:
--   * VARBINARY(16) ULID PK to match every other Phase 1/2 table.
--   * org_id is the first column of every non-primary index.
--   * (org_id, integration_id, name, language) is the unique idempotency
--     key — Meta requires globally-unique (name, language) per WABA and we
--     mirror that here.
--   * status column is a plain VARCHAR — the domain vocabulary is wider
--     than Meta's ENUM (DRAFT + PENDING + ...), so keeping the storage
--     narrow would leak provider semantics into the schema.
--   * components + variables live as JSON so the schema stays stable while
--     Meta continues to add button/header/media variants.
--   * last_synced_at NULL means the row has never been reconciled with the
--     provider (fresh DRAFT); a non-null value is the last provider fetch.

CREATE TABLE IF NOT EXISTS templates (
  id                    VARBINARY(16) NOT NULL,
  org_id                VARBINARY(16) NOT NULL,
  integration_id        VARBINARY(16) NOT NULL,
  provider_template_id  VARCHAR(64)   NOT NULL DEFAULT '',
  name                  VARCHAR(128)  NOT NULL,
  language              VARCHAR(16)   NOT NULL,
  category              VARCHAR(32)   NOT NULL DEFAULT 'MARKETING',
  status                VARCHAR(32)   NOT NULL DEFAULT 'DRAFT',
  components            JSON          NOT NULL,
  variables             JSON          NOT NULL,
  last_synced_at        DATETIME(3)   NULL,
  created_at            DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at            DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                      ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY ux_templates_org_integration_name_lang (org_id, integration_id, name, language),
  KEY ix_templates_org_status  (org_id, status),
  KEY ix_templates_org_updated (org_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
