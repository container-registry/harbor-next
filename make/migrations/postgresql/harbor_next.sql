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

-- Multi-format artifact repositories (npm, Maven): rebuildable Postgres
-- projection over the OCI `_index` control artifact. Authoritative mutable
-- state lives in OCI annotations; these tables are a derived view
-- (reconcilable from `_index`). Formerly numbered migration 0191.
CREATE TABLE IF NOT EXISTS multi_format_package (
  id BIGSERIAL PRIMARY KEY,
  project_id BIGINT NOT NULL,
  format VARCHAR(32) NOT NULL,
  native_name VARCHAR(512) NOT NULL,
  proj_version BIGINT NOT NULL DEFAULT 0,
  mutable_state JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_index_digest VARCHAR(255) NOT NULL DEFAULT '',
  creation_time TIMESTAMP DEFAULT now(),
  update_time TIMESTAMP DEFAULT now(),
  UNIQUE (project_id, format, native_name)
);

CREATE TABLE IF NOT EXISTS multi_format_version (
  id BIGSERIAL PRIMARY KEY,
  package_id BIGINT NOT NULL REFERENCES multi_format_package(id) ON DELETE CASCADE,
  version VARCHAR(255) NOT NULL,
  payload_digest VARCHAR(255) NOT NULL DEFAULT '',
  payload_size BIGINT NOT NULL DEFAULT 0,
  yanked BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMP NOT NULL DEFAULT now(),
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  UNIQUE (package_id, version)
);

CREATE INDEX IF NOT EXISTS idx_multi_format_package_proj_fmt ON multi_format_package(project_id, format);
CREATE INDEX IF NOT EXISTS idx_multi_format_version_pkg ON multi_format_version(package_id);

-- Multi-project (project of projects): ordered sub-project references of a
-- multi-project. A project is marked as a multi-project by the
-- project_metadata key "multi_project" = "true"; this table holds which
-- projects it aggregates and the ranked order used for pull resolution
-- (lower rank resolves first). Sub-projects must not themselves be
-- multi-projects (enforced at the API layer, not by schema).
CREATE TABLE IF NOT EXISTS multi_project_reference (
  id BIGSERIAL PRIMARY KEY,
  multi_project_id BIGINT NOT NULL REFERENCES project(project_id),
  sub_project_id BIGINT NOT NULL REFERENCES project(project_id),
  rank BIGINT NOT NULL DEFAULT 0,
  creation_time TIMESTAMP DEFAULT now(),
  update_time TIMESTAMP DEFAULT now(),
  UNIQUE (multi_project_id, sub_project_id),
  CHECK (multi_project_id <> sub_project_id)
);

CREATE INDEX IF NOT EXISTS idx_multi_project_reference_multi ON multi_project_reference(multi_project_id, rank);
CREATE INDEX IF NOT EXISTS idx_multi_project_reference_sub ON multi_project_reference(sub_project_id);

-- Ranks are dense positions (1..N per multi-project), maintained by the
-- application on every insert/move/delete. Renormalize here so rows written
-- before that invariant existed (or drifted) are compacted; idempotent.
WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY multi_project_id ORDER BY rank, id) AS rn
  FROM multi_project_reference
)
UPDATE multi_project_reference m
SET rank = ranked.rn
FROM ranked
WHERE m.id = ranked.id AND m.rank <> ranked.rn;
