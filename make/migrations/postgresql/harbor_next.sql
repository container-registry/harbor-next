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

-- execution.revision is declared int64 in the Go model (src/pkg/task/dao/model.go)
-- while the column stayed integer. 0181 widened p2p_preheat_instance.setup_timestamp,
-- task.status_revision and schedule.revision to bigint and left this one behind, so
-- the model and the column disagree on the only revision column still 32-bit.
-- Unlike schedule.revision, which stores a job check-in unix timestamp and would
-- overflow in 2038, this one is an optimistic-locking counter (revision = revision+1
-- in pkg/task/dao/execution.go) and is widened for consistency with the model, not
-- because it is close to overflowing.
-- Guarded on the current type so repeat runs never rewrite the table.
DO $$
BEGIN
    -- Resolve the schema from the same relation the unqualified ALTER below
    -- resolves to. current_schema() is only the first entry in search_path, so
    -- it would miss an execution table living in a later one and skip the
    -- widening without a word. to_regclass returns NULL when there is no
    -- execution relation at all, which leaves the guard false, as it should.
    --
    -- relkind keeps the guard on tables: to_regclass resolves any relation, so
    -- an index named execution earlier in search_path would otherwise match a
    -- pg_attribute row here and send ALTER TABLE at something it cannot alter.
    --
    -- The type is compared after resolving a domain to its base type, so a
    -- column already typed as a domain over bigint keeps the domain and its
    -- constraints instead of having them stripped off by the ALTER.
    IF EXISTS (
        SELECT 1
        FROM pg_attribute a
        JOIN pg_class c ON c.oid = a.attrelid
        JOIN pg_type t ON t.oid = a.atttypid
        WHERE a.attrelid = to_regclass('execution')
          AND c.relkind IN ('r', 'p')
          AND a.attname = 'revision'
          AND a.attnum > 0
          AND NOT a.attisdropped
          AND CASE WHEN t.typtype = 'd' THEN t.typbasetype ELSE a.atttypid END
              <> 'bigint'::regtype
    ) THEN
        ALTER TABLE execution ALTER COLUMN revision TYPE bigint;
    END IF;
END
$$;
