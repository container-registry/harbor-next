-- Harbor Next authoritative schema
--
-- This public, unversioned schema is reconciled after Harbor's numbered
-- migrations every time database migration is enabled. Keep every statement
-- idempotent: this file is intentionally executed more than once and is not
-- tracked in schema_migrations.
--
-- Only additive, data-preserving changes belong here. Destructive changes and
-- large data backfills require a separately reviewed operational procedure.
--
-- branding and identity_providers/robot_identity_providers/claim_rules were
-- formerly release-2.15 migrations 0181/0182; both numbers were later reused
-- by real upstream migrations, so they moved here instead of being renumbered.

-- Branding customization
CREATE TABLE IF NOT EXISTS branding (
    id           INTEGER PRIMARY KEY NOT NULL,
    config       TEXT NOT NULL,
    update_time  TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Workload identity federation
CREATE TABLE IF NOT EXISTS identity_providers (
    id                      SERIAL PRIMARY KEY,
    name                    TEXT NOT NULL,
    description             TEXT,
    issuer                  TEXT NOT NULL,
    openid_config_url       TEXT,
    offline_validation      BOOLEAN NOT NULL DEFAULT FALSE,
    supported_algorithms    TEXT,
    claims_supported        TEXT,
    jwks_uri                TEXT,
    jwks_keys               JSONB,
    jwks_cached_at          TIMESTAMP,
    jwks_expires_at         TIMESTAMP,
    jwks_last_fetch_attempt TIMESTAMP,
    project_id              INT NOT NULL DEFAULT 0,
    creation_time           TIMESTAMP DEFAULT NOW(),
    update_time             TIMESTAMP DEFAULT NOW(),
    UNIQUE (issuer, project_id)
);

CREATE TABLE IF NOT EXISTS robot_identity_providers (
    id                   SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id             BIGINT NOT NULL REFERENCES robot(id) ON DELETE CASCADE,
    creation_time        TIMESTAMP DEFAULT NOW(),
    UNIQUE (identity_provider_id, robot_id)
);

CREATE TABLE IF NOT EXISTS claim_rules (
    id                   SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id             BIGINT NOT NULL DEFAULT 0,
    claim_path           TEXT NOT NULL,
    value                TEXT,
    creation_time        TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_claim_rules_lookup
    ON claim_rules (identity_provider_id, claim_path, value, robot_id);

CREATE INDEX IF NOT EXISTS idx_identity_providers_jwks_cache
    ON identity_providers (id, jwks_expires_at, jwks_last_fetch_attempt);

-- 0190_2.16.0 was amended in place (robot bigint IDs, audit client columns)
-- after deployed databases had already recorded schema_migrations >= 190, so
-- golang-migrate never replays it for them. Reconcile those amendments here;
-- fresh installs run the same statements in 0190 and these no-op. The
-- existence guards keep the file applicable to a bare schema (see
-- authoritative_db_test.go).

-- Robot account ID columns to bigint (upstream goharbor/harbor#23633)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'robot' AND column_name = 'creator_ref'
          AND data_type <> 'bigint'
    ) THEN
        ALTER TABLE robot ALTER COLUMN id TYPE bigint;
        ALTER TABLE robot ALTER COLUMN creator_ref TYPE bigint;
        ALTER SEQUENCE robot_id_seq AS bigint MAXVALUE 9007199254740991;
    END IF;
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'role_permission' AND column_name = 'role_id'
          AND data_type <> 'bigint'
    ) THEN
        ALTER TABLE role_permission ALTER COLUMN role_id TYPE bigint;
    END IF;
END
$$;

-- Audit log client IP address / User-Agent columns
DO $$
BEGIN
    IF to_regclass('audit_log_ext') IS NOT NULL THEN
        ALTER TABLE audit_log_ext ADD COLUMN IF NOT EXISTS client_address varchar(255) DEFAULT '';
        ALTER TABLE audit_log_ext ADD COLUMN IF NOT EXISTS user_agent varchar(1024) DEFAULT '';
    END IF;
END
$$;
