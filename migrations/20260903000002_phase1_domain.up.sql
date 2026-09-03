-- 20260903000002_phase1_domain.up.sql
-- Phase 1: contacts, identities, business endpoints, integrations, sessions,
-- conversations, message metadata, webhook events.
-- All tables InnoDB / utf8mb4. Every non-primary index leads with org_id.

-- ---------------------------------------------------------------------------
-- contacts (created before contact_identities; primary_identity_id FK added later)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS contacts (
  id                    VARBINARY(16) NOT NULL,
  org_id                VARBINARY(16) NOT NULL,
  display_name          VARCHAR(255)  NOT NULL DEFAULT '',
  avatar_url            VARCHAR(1024) NULL,
  primary_identity_id   VARBINARY(16) NULL,
  last_seen_at          DATETIME(3)   NULL,
  created_at            DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at            DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_contacts_org_primary_identity (org_id, primary_identity_id),
  KEY ix_contacts_org_updated (org_id, updated_at),
  KEY ix_contacts_org_last_seen (org_id, last_seen_at),
  CONSTRAINT fk_contacts_org FOREIGN KEY (org_id) REFERENCES organizations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ---------------------------------------------------------------------------
-- contact_identities
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS contact_identities (
  id                VARBINARY(16) NOT NULL,
  org_id            VARBINARY(16) NOT NULL,
  contact_id        VARBINARY(16) NOT NULL,
  identity_type     VARCHAR(32)   NOT NULL,
  provider          VARCHAR(64)   NOT NULL,
  identity_value    VARCHAR(320)  NOT NULL,
  normalized_value  VARCHAR(320)  NOT NULL,
  verified          TINYINT(1)    NOT NULL DEFAULT 0,
  metadata          JSON          NOT NULL,
  created_at        DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_identities_org_provider_norm (org_id, provider, normalized_value),
  KEY ix_identities_org_contact (org_id, contact_id),
  CONSTRAINT fk_identities_org     FOREIGN KEY (org_id)     REFERENCES organizations(id),
  CONSTRAINT fk_identities_contact FOREIGN KEY (contact_id) REFERENCES contacts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Now that both tables exist, wire contacts.primary_identity_id FK.
-- (Wrapped in a stored routine so re-running the migration is safe.)
ALTER TABLE contacts
  ADD CONSTRAINT fk_contacts_primary_identity
  FOREIGN KEY (primary_identity_id) REFERENCES contact_identities(id);

-- ---------------------------------------------------------------------------
-- integrations
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS integrations (
  id                VARBINARY(16) NOT NULL,
  org_id            VARBINARY(16) NOT NULL,
  type              VARCHAR(32)   NOT NULL,             -- 'channel' | 'ticketing' | 'bot' | 'ai' | 'calling'
  provider          VARCHAR(64)   NOT NULL,             -- 'whatsapp' | 'zoho_desk' | ...
  name              VARCHAR(255)  NOT NULL,
  status            ENUM('pending','active','error','disabled') NOT NULL DEFAULT 'pending',
  config            JSON          NOT NULL,
  credentials_ref   VARBINARY(64) NULL,
  capabilities      JSON          NOT NULL,
  health            JSON          NOT NULL,
  created_at        DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_integrations_org_provider_name (org_id, provider, name),
  KEY ix_integrations_org_type (org_id, type, status),
  CONSTRAINT fk_integrations_org FOREIGN KEY (org_id) REFERENCES organizations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ---------------------------------------------------------------------------
-- integration_credentials (envelope-encrypted; one per integration)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS integration_credentials (
  id             VARBINARY(16)  NOT NULL,
  org_id         VARBINARY(16)  NOT NULL,
  integration_id VARBINARY(16)  NOT NULL,
  ciphertext     MEDIUMBLOB     NOT NULL,
  kek_ref        VARCHAR(128)   NOT NULL,
  created_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_credentials_integration (integration_id),
  KEY ix_credentials_org (org_id),
  CONSTRAINT fk_credentials_org         FOREIGN KEY (org_id)         REFERENCES organizations(id),
  CONSTRAINT fk_credentials_integration FOREIGN KEY (integration_id) REFERENCES integrations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ---------------------------------------------------------------------------
-- business_endpoints
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS business_endpoints (
  id             VARBINARY(16) NOT NULL,
  org_id         VARBINARY(16) NOT NULL,
  channel        VARCHAR(32)   NOT NULL,             -- 'whatsapp' | 'sms' | 'email' | ...
  provider       VARCHAR(64)   NOT NULL,
  integration_id VARBINARY(16) NOT NULL,
  external_id    VARCHAR(255)  NOT NULL,             -- e.g. WhatsApp phone_number_id
  display        VARCHAR(255)  NOT NULL DEFAULT '',
  metadata       JSON          NOT NULL,
  created_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_endpoints_org_provider_external (org_id, provider, external_id),
  KEY ix_endpoints_org_integration (org_id, integration_id),
  CONSTRAINT fk_endpoints_org         FOREIGN KEY (org_id)         REFERENCES organizations(id),
  CONSTRAINT fk_endpoints_integration FOREIGN KEY (integration_id) REFERENCES integrations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ---------------------------------------------------------------------------
-- sessions_comm — communication sessions
--
-- Enforce "at most one ACTIVE session per (org, endpoint, contact)" using a
-- STORED GENERATED column that equals contact_id when status='active' and
-- NULL otherwise, then a UNIQUE index on (org_id, business_endpoint_id,
-- active_contact_id). MySQL 8 treats multiple NULLs as distinct, so closed
-- sessions do not collide.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sessions_comm (
  id                    VARBINARY(16) NOT NULL,
  org_id                VARBINARY(16) NOT NULL,
  contact_id            VARBINARY(16) NOT NULL,
  business_endpoint_id  VARBINARY(16) NOT NULL,
  status                ENUM('active','closed') NOT NULL DEFAULT 'active',
  opened_at             DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  closed_at             DATETIME(3)   NULL,
  metadata              JSON          NOT NULL,
  active_contact_id     VARBINARY(16) GENERATED ALWAYS AS
    (CASE WHEN status = 'active' THEN contact_id ELSE NULL END) STORED,
  PRIMARY KEY (id),
  UNIQUE KEY uk_sessions_active (org_id, business_endpoint_id, active_contact_id),
  KEY ix_sessions_org_contact (org_id, contact_id),
  KEY ix_sessions_org_endpoint_status (org_id, business_endpoint_id, status),
  CONSTRAINT fk_sessions_org      FOREIGN KEY (org_id)     REFERENCES organizations(id),
  CONSTRAINT fk_sessions_contact  FOREIGN KEY (contact_id) REFERENCES contacts(id),
  CONSTRAINT fk_sessions_endpoint FOREIGN KEY (business_endpoint_id) REFERENCES business_endpoints(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ---------------------------------------------------------------------------
-- conversations
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS conversations (
  id                 VARBINARY(16) NOT NULL,
  org_id             VARBINARY(16) NOT NULL,
  session_id         VARBINARY(16) NOT NULL,
  contact_id         VARBINARY(16) NOT NULL,
  status             ENUM('open','pending','resolved','reopened') NOT NULL DEFAULT 'open',
  assigned_user_id   VARBINARY(16) NULL,
  assigned_team_id   VARBINARY(16) NULL,
  priority           ENUM('low','normal','high','urgent') NOT NULL DEFAULT 'normal',
  unread_count       INT           NOT NULL DEFAULT 0,
  last_message_at    DATETIME(3)   NULL,
  sla_due_at         DATETIME(3)   NULL,
  ai_state           VARCHAR(64)   NOT NULL DEFAULT '',
  bot_state          VARCHAR(64)   NOT NULL DEFAULT '',
  tags               JSON          NOT NULL,
  created_at         DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  resolved_at        DATETIME(3)   NULL,
  PRIMARY KEY (id),
  KEY ix_conversations_org_status_last (org_id, status, last_message_at),
  KEY ix_conversations_org_contact (org_id, contact_id, last_message_at),
  KEY ix_conversations_org_session (org_id, session_id),
  KEY ix_conversations_org_assignee (org_id, assigned_user_id, status),
  CONSTRAINT fk_conversations_org     FOREIGN KEY (org_id)     REFERENCES organizations(id),
  CONSTRAINT fk_conversations_session FOREIGN KEY (session_id) REFERENCES sessions_comm(id),
  CONSTRAINT fk_conversations_contact FOREIGN KEY (contact_id) REFERENCES contacts(id),
  CONSTRAINT fk_conversations_user    FOREIGN KEY (assigned_user_id) REFERENCES users(id),
  CONSTRAINT fk_conversations_team    FOREIGN KEY (assigned_team_id) REFERENCES teams(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ---------------------------------------------------------------------------
-- messages (metadata only; payload lives in HBase)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS messages (
  id                   VARBINARY(16) NOT NULL,
  org_id               VARBINARY(16) NOT NULL,
  contact_id           VARBINARY(16) NOT NULL,
  session_id           VARBINARY(16) NOT NULL,
  conversation_id      VARBINARY(16) NOT NULL,
  channel              VARCHAR(32)   NOT NULL,
  provider             VARCHAR(64)   NOT NULL,
  direction            ENUM('inbound','outbound') NOT NULL,
  sender_identity      VARCHAR(320)  NOT NULL DEFAULT '',
  recipient_identity   VARCHAR(320)  NOT NULL DEFAULT '',
  message_type         VARCHAR(32)   NOT NULL,
  provider_message_id  VARCHAR(255)  NULL,
  status               ENUM('queued','sent','delivered','read','failed') NOT NULL DEFAULT 'queued',
  created_at           DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  sent_at              DATETIME(3)   NULL,
  delivered_at         DATETIME(3)   NULL,
  read_at              DATETIME(3)   NULL,
  payload_ref          VARCHAR(255)  NOT NULL DEFAULT '',
  metadata             JSON          NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_messages_org_provider_msg (org_id, provider, provider_message_id),
  KEY ix_messages_org_conv_created (org_id, conversation_id, created_at),
  KEY ix_messages_org_session_created (org_id, session_id, created_at),
  KEY ix_messages_org_contact_created (org_id, contact_id, created_at),
  KEY ix_messages_org_status (org_id, status),
  CONSTRAINT fk_messages_org         FOREIGN KEY (org_id)          REFERENCES organizations(id),
  CONSTRAINT fk_messages_contact     FOREIGN KEY (contact_id)      REFERENCES contacts(id),
  CONSTRAINT fk_messages_session     FOREIGN KEY (session_id)      REFERENCES sessions_comm(id),
  CONSTRAINT fk_messages_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ---------------------------------------------------------------------------
-- webhook_events (raw envelope reference; idempotency via unique index)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS webhook_events (
  id                 VARBINARY(16) NOT NULL,
  org_id             VARBINARY(16) NOT NULL,
  integration_id     VARBINARY(16) NOT NULL,
  provider           VARCHAR(64)   NOT NULL,
  external_event_id  VARCHAR(255)  NOT NULL,
  received_at        DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  processed_at       DATETIME(3)   NULL,
  status             ENUM('received','processing','processed','failed') NOT NULL DEFAULT 'received',
  raw_ref            VARCHAR(255)  NOT NULL DEFAULT '',
  error              TEXT          NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_webhook_events_integration_external (integration_id, external_event_id),
  KEY ix_webhook_events_org_received (org_id, received_at),
  KEY ix_webhook_events_org_status (org_id, status),
  CONSTRAINT fk_webhook_events_org         FOREIGN KEY (org_id)         REFERENCES organizations(id),
  CONSTRAINT fk_webhook_events_integration FOREIGN KEY (integration_id) REFERENCES integrations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
