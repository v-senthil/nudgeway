-- 20260903000001_organizations_users_roles.up.sql
-- Phase 0 baseline: organizations, users, roles, permissions, and web sessions.
-- All tables are utf8mb4 / InnoDB. Every non-primary index leads with org_id
-- where applicable to enforce tenant-scoped access.

CREATE TABLE IF NOT EXISTS organizations (
  id            VARBINARY(16) NOT NULL,             -- ULID/UUIDv7 as 16 bytes
  slug          VARCHAR(64)   NOT NULL,
  name          VARCHAR(255)  NOT NULL,
  status        ENUM('active','suspended') NOT NULL DEFAULT 'active',
  settings      JSON          NOT NULL,
  created_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_organizations_slug (slug)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS users (
  id             VARBINARY(16) NOT NULL,
  org_id         VARBINARY(16) NOT NULL,
  email          VARCHAR(320)  NOT NULL,
  password_hash  VARBINARY(255) NOT NULL,             -- argon2id encoded string
  display_name   VARCHAR(255)  NOT NULL DEFAULT '',
  status         ENUM('active','disabled','invited') NOT NULL DEFAULT 'invited',
  last_login_at  DATETIME(3)   NULL,
  created_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_users_org_email (org_id, email),
  KEY ix_users_org (org_id, status),
  CONSTRAINT fk_users_org FOREIGN KEY (org_id) REFERENCES organizations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS teams (
  id         VARBINARY(16) NOT NULL,
  org_id     VARBINARY(16) NOT NULL,
  name       VARCHAR(255)  NOT NULL,
  created_at DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_teams_org_name (org_id, name),
  CONSTRAINT fk_teams_org FOREIGN KEY (org_id) REFERENCES organizations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS team_members (
  team_id  VARBINARY(16) NOT NULL,
  user_id  VARBINARY(16) NOT NULL,
  PRIMARY KEY (team_id, user_id),
  CONSTRAINT fk_team_members_team FOREIGN KEY (team_id) REFERENCES teams(id),
  CONSTRAINT fk_team_members_user FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS roles (
  id         VARBINARY(16) NOT NULL,
  org_id     VARBINARY(16) NOT NULL,
  name       VARCHAR(128)  NOT NULL,
  is_system  TINYINT(1)    NOT NULL DEFAULT 0,
  created_at DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_roles_org_name (org_id, name),
  CONSTRAINT fk_roles_org FOREIGN KEY (org_id) REFERENCES organizations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS role_permissions (
  role_id        VARBINARY(16) NOT NULL,
  permission_key VARCHAR(128)  NOT NULL,             -- e.g. "contacts.read"
  PRIMARY KEY (role_id, permission_key),
  CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id) REFERENCES roles(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS user_roles (
  user_id VARBINARY(16) NOT NULL,
  role_id VARBINARY(16) NOT NULL,
  PRIMARY KEY (user_id, role_id),
  CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES users(id),
  CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id) REFERENCES roles(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS web_sessions (
  id             VARBINARY(32) NOT NULL,            -- opaque session id, HMAC
  user_id        VARBINARY(16) NOT NULL,
  org_id         VARBINARY(16) NOT NULL,
  issued_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_seen_at   DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  expires_at     DATETIME(3)   NOT NULL,
  ip             VARBINARY(16) NULL,
  user_agent     VARCHAR(512)  NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  KEY ix_web_sessions_user (user_id),
  KEY ix_web_sessions_expiry (expires_at),
  CONSTRAINT fk_web_sessions_user FOREIGN KEY (user_id) REFERENCES users(id),
  CONSTRAINT fk_web_sessions_org  FOREIGN KEY (org_id)  REFERENCES organizations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS audit_logs (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_id         VARBINARY(16) NOT NULL,
  actor_user_id  VARBINARY(16) NULL,
  action         VARCHAR(128)  NOT NULL,
  resource_type  VARCHAR(64)   NOT NULL,
  resource_id    VARCHAR(64)   NOT NULL DEFAULT '',
  ip             VARBINARY(16) NULL,
  metadata       JSON          NOT NULL,
  occurred_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY ix_audit_org_time (org_id, occurred_at),
  KEY ix_audit_org_resource (org_id, resource_type, resource_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
