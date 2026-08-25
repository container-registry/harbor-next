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
    robot_id             INT NOT NULL REFERENCES robot(id) ON DELETE CASCADE,
    creation_time        TIMESTAMP DEFAULT NOW(),
    UNIQUE (identity_provider_id, robot_id)
);

CREATE TABLE IF NOT EXISTS claim_rules (
    id                   SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id             INT NOT NULL DEFAULT 0,
    claim_path           TEXT NOT NULL,
    value                TEXT,
    creation_time        TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_claim_rules_lookup
    ON claim_rules (identity_provider_id, claim_path, value, robot_id);

CREATE INDEX IF NOT EXISTS idx_identity_providers_jwks_cache
    ON identity_providers (id, jwks_expires_at, jwks_last_fetch_attempt);
