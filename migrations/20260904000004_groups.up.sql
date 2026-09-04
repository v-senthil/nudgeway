-- 20260904000004_groups.up.sql
-- WhatsApp groups + participants persistence.
--
-- Design notes:
--   * `groups` — one row per provider-side group instance the tenant knows
--     about. `provider_group_id` is the opaque Meta CAPI group id
--     (e.g. Y2FwaV9ncm91cDoxNzA1... base64ish string).
--   * `group_members` — participant roster; either bound to a Contact (once
--     an inbound message from the participant resolves them) or left
--     un-linked with just the wa_id / bsuid we saw.
--   * All indexes lead with org_id per the Phase 1 tenancy rule.
--   * Idempotent create so re-running the migration is safe.

CREATE TABLE IF NOT EXISTS `groups` (
  id                   VARBINARY(16) NOT NULL,
  org_id               VARBINARY(16) NOT NULL,
  integration_id       VARBINARY(16) NOT NULL,
  provider_group_id    VARCHAR(64)   NOT NULL,
  subject              VARCHAR(256)  NOT NULL DEFAULT '',
  description          TEXT          NULL,
  size                 INT           NOT NULL DEFAULT 0,
  is_admin             BOOLEAN       NOT NULL DEFAULT FALSE,
  metadata             JSON          NOT NULL,
  created_at           DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at           DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_groups_org_integ_provider (org_id, integration_id, provider_group_id),
  KEY ix_groups_org_updated (org_id, updated_at),
  CONSTRAINT fk_groups_org         FOREIGN KEY (org_id)         REFERENCES organizations(id),
  CONSTRAINT fk_groups_integration FOREIGN KEY (integration_id) REFERENCES integrations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS group_members (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_id       VARBINARY(16)   NOT NULL,
  group_id     VARBINARY(16)   NOT NULL,
  contact_id   VARBINARY(16)   NULL,
  wa_id        VARCHAR(32)     NOT NULL DEFAULT '',
  bsuid        VARCHAR(160)    NOT NULL DEFAULT '',
  role         VARCHAR(16)     NOT NULL DEFAULT 'member',
  joined_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  left_at      DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_group_members_group_ids (group_id, wa_id, bsuid),
  KEY ix_group_members_org_contact (org_id, contact_id),
  KEY ix_group_members_org_group (org_id, group_id),
  CONSTRAINT fk_group_members_org     FOREIGN KEY (org_id)     REFERENCES organizations(id),
  CONSTRAINT fk_group_members_group   FOREIGN KEY (group_id)   REFERENCES `groups`(id),
  CONSTRAINT fk_group_members_contact FOREIGN KEY (contact_id) REFERENCES contacts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
