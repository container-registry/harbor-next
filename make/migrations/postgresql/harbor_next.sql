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
-- branding and trusted_issuers/robot_trusted_issuers/claim_rules were
-- formerly release-2.15 migrations 0181/0182; both numbers were later reused
-- by real upstream migrations, so they moved here instead of being renumbered.

-- Branding customization
CREATE TABLE IF NOT EXISTS branding (
    id           INTEGER PRIMARY KEY NOT NULL,
    config       TEXT NOT NULL,
    update_time  TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Federated robot accounts: trusted issuers
--
-- These tables were first named identity_providers/robot_identity_providers
-- with an identity_provider_id column. Rename them in place before the CREATE
-- statements below so existing rows survive, then carry the renamed config key
-- and RBAC resource along. Every step is guarded, so a fresh database or a
-- repeat run skips it.
DO $$
DECLARE
    r record;
BEGIN
    IF to_regclass('identity_providers') IS NOT NULL AND to_regclass('trusted_issuers') IS NULL THEN
        ALTER TABLE identity_providers RENAME TO trusted_issuers;
    END IF;
    IF to_regclass('robot_identity_providers') IS NOT NULL AND to_regclass('robot_trusted_issuers') IS NULL THEN
        ALTER TABLE robot_identity_providers RENAME TO robot_trusted_issuers;
    END IF;

    FOR r IN
        SELECT a.attrelid::regclass AS tbl
        FROM pg_attribute a
        WHERE a.attrelid IN (to_regclass('robot_trusted_issuers'), to_regclass('claim_rules'))
          AND a.attname = 'identity_provider_id'
          AND a.attnum > 0
          AND NOT a.attisdropped
          AND NOT EXISTS (
              SELECT 1 FROM pg_attribute n
              WHERE n.attrelid = a.attrelid AND n.attname = 'trusted_issuer_id' AND NOT n.attisdropped
          )
    LOOP
        EXECUTE format('ALTER TABLE %s RENAME COLUMN identity_provider_id TO trusted_issuer_id', r.tbl);
    END LOOP;

    -- constraints, their indexes and the serial sequences keep the names
    -- they were created with, so follow them up by name
    FOR r IN
        SELECT con.conrelid::regclass AS tbl, con.conname
        FROM pg_constraint con
        WHERE con.conrelid IN (to_regclass('trusted_issuers'), to_regclass('robot_trusted_issuers'), to_regclass('claim_rules'))
          AND con.conname LIKE '%identity_provider%'
    LOOP
        EXECUTE format('ALTER TABLE %s RENAME CONSTRAINT %I TO %I',
            r.tbl, r.conname, replace(r.conname, 'identity_provider', 'trusted_issuer'));
    END LOOP;

    FOR r IN
        SELECT c.oid::regclass AS rel, c.relkind, c.relname
        FROM pg_class c
        WHERE c.relname LIKE '%identity_provider%'
          AND c.relkind IN ('i', 'S')
          AND c.relnamespace IN (
              SELECT relnamespace FROM pg_class
              WHERE oid IN (to_regclass('trusted_issuers'), to_regclass('robot_trusted_issuers'))
          )
          AND (
              EXISTS (
                  SELECT 1 FROM pg_index i
                  WHERE i.indexrelid = c.oid
                    AND i.indrelid IN (to_regclass('trusted_issuers'), to_regclass('robot_trusted_issuers'), to_regclass('claim_rules'))
              )
              OR EXISTS (
                  SELECT 1 FROM pg_depend d
                  WHERE d.classid = 'pg_class'::regclass
                    AND d.objid = c.oid
                    AND d.refobjid IN (to_regclass('trusted_issuers'), to_regclass('robot_trusted_issuers'))
              )
          )
    LOOP
        IF to_regclass(quote_ident(replace(r.relname, 'identity_provider', 'trusted_issuer'))) IS NULL THEN
            EXECUTE format('ALTER %s %s RENAME TO %I',
                CASE r.relkind WHEN 'S' THEN 'SEQUENCE' ELSE 'INDEX' END,
                r.rel, replace(r.relname, 'identity_provider', 'trusted_issuer'));
        END IF;
    END LOOP;

    IF to_regclass('properties') IS NOT NULL THEN
        UPDATE properties SET k = 'enable_project_federated_robot_accounts'
        WHERE k = 'enable_project_federated_idp'
          AND NOT EXISTS (SELECT 1 FROM properties WHERE k = 'enable_project_federated_robot_accounts');
    END IF;
    IF to_regclass('permission_policy') IS NOT NULL THEN
        UPDATE permission_policy p SET resource = 'trusted-issuer'
        WHERE p.resource = 'federated-idp'
          AND NOT EXISTS (
              SELECT 1 FROM permission_policy n
              WHERE n.scope = p.scope AND n.resource = 'trusted-issuer'
                AND n.action IS NOT DISTINCT FROM p.action AND n.effect IS NOT DISTINCT FROM p.effect
          );
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS trusted_issuers (
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

CREATE TABLE IF NOT EXISTS robot_trusted_issuers (
    id                SERIAL PRIMARY KEY,
    trusted_issuer_id INT NOT NULL REFERENCES trusted_issuers(id) ON DELETE CASCADE,
    robot_id          BIGINT NOT NULL REFERENCES robot(id) ON DELETE CASCADE,
    creation_time     TIMESTAMP DEFAULT NOW(),
    UNIQUE (trusted_issuer_id, robot_id)
);

CREATE TABLE IF NOT EXISTS claim_rules (
    id                SERIAL PRIMARY KEY,
    trusted_issuer_id INT NOT NULL REFERENCES trusted_issuers(id) ON DELETE CASCADE,
    robot_id          BIGINT NOT NULL DEFAULT 0,
    claim_path        TEXT NOT NULL,
    value             TEXT,
    creation_time     TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_claim_rules_lookup
    ON claim_rules (trusted_issuer_id, claim_path, value, robot_id);

CREATE INDEX IF NOT EXISTS idx_trusted_issuers_jwks_cache
    ON trusted_issuers (id, jwks_expires_at, jwks_last_fetch_attempt);

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
    -- execution table at all, which leaves the guard false, as it should.
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
