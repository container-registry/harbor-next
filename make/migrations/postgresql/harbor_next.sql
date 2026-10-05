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
-- Renames the identity_providers schema to trusted_issuers. Each step runs only
-- while the old name exists and the new one does not, so a repeat run and a
-- fresh database are both no-ops here.
DO $$
DECLARE
    renames CONSTANT text[][] := ARRAY[
        ['TABLE', 'identity_providers', 'trusted_issuers'],
        ['TABLE', 'robot_identity_providers', 'robot_trusted_issuers'],
        ['SEQUENCE', 'identity_providers_id_seq', 'trusted_issuers_id_seq'],
        ['SEQUENCE', 'robot_identity_providers_id_seq', 'robot_trusted_issuers_id_seq'],
        ['INDEX', 'idx_identity_providers_jwks_cache', 'idx_trusted_issuers_jwks_cache']
    ];
    r text[];
BEGIN
    FOREACH r SLICE 1 IN ARRAY renames LOOP
        IF to_regclass(r[2]) IS NOT NULL AND to_regclass(r[3]) IS NULL THEN
            EXECUTE format('ALTER %s %I RENAME TO %I', r[1], r[2], r[3]);
        END IF;
    END LOOP;
END
$$;

DO $$
DECLARE
    c record;
BEGIN
    FOR c IN
        SELECT a.attrelid::regclass AS tbl
        FROM pg_attribute a
        WHERE a.attrelid IN (to_regclass('robot_trusted_issuers'), to_regclass('claim_rules'))
          AND a.attname = 'identity_provider_id'
          AND NOT a.attisdropped
          AND NOT EXISTS (
              SELECT 1 FROM pg_attribute n
              WHERE n.attrelid = a.attrelid
                AND n.attname = 'trusted_issuer_id'
                AND NOT n.attisdropped
          )
    LOOP
        EXECUTE format('ALTER TABLE %s RENAME COLUMN identity_provider_id TO trusted_issuer_id', c.tbl);
    END LOOP;

    -- constraint names follow what CREATE TABLE below would generate
    FOR c IN
        SELECT con.conrelid::regclass AS tbl, con.conname AS old_name,
               replace(replace(con.conname, 'identity_providers', 'trusted_issuers'),
                       'identity_provider_id', 'trusted_issuer_id') AS new_name
        FROM pg_constraint con
        WHERE con.conrelid IN (to_regclass('trusted_issuers'), to_regclass('robot_trusted_issuers'),
                               to_regclass('claim_rules'))
          AND (con.conname LIKE '%identity_providers%' OR con.conname LIKE '%identity_provider_id%')
    LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = c.tbl AND conname = c.new_name) THEN
            EXECUTE format('ALTER TABLE %s RENAME CONSTRAINT %I TO %I', c.tbl, c.old_name, c.new_name);
        END IF;
    END LOOP;
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

-- Stored settings and robot permissions written under the old names. Config
-- keys without metadata are skipped on load, so they would silently reset.
DO $$
DECLARE
    keys CONSTANT text[][] := ARRAY[
        ['enable_commercial_identity_providers', 'enable_commercial_federated_robot_accounts'],
        ['enable_project_federated_idp', 'enable_project_federated_robot_accounts']
    ];
    kv text[];
BEGIN
    IF to_regclass('properties') IS NOT NULL THEN
        FOREACH kv SLICE 1 IN ARRAY keys LOOP
            IF EXISTS (SELECT 1 FROM properties WHERE properties.k = kv[2]) THEN
                DELETE FROM properties WHERE properties.k = kv[1];
            ELSE
                UPDATE properties SET k = kv[2] WHERE properties.k = kv[1];
            END IF;
        END LOOP;
    END IF;

    IF to_regclass('permission_policy') IS NOT NULL AND to_regclass('role_permission') IS NOT NULL THEN
        -- a role already holding the new policy drops its old duplicate
        DELETE FROM role_permission rp
        USING permission_policy o, permission_policy n, role_permission rn
        WHERE rp.permission_policy_id = o.id
          AND o.resource = 'federated-idp'
          AND n.resource = 'trusted-issuer'
          AND n.scope = o.scope AND n.action IS NOT DISTINCT FROM o.action
          AND n.effect IS NOT DISTINCT FROM o.effect
          AND rn.permission_policy_id = n.id
          AND rn.role_type = rp.role_type AND rn.role_id = rp.role_id;

        UPDATE role_permission rp
        SET permission_policy_id = n.id
        FROM permission_policy o, permission_policy n
        WHERE rp.permission_policy_id = o.id
          AND o.resource = 'federated-idp'
          AND n.resource = 'trusted-issuer'
          AND n.scope = o.scope AND n.action IS NOT DISTINCT FROM o.action
          AND n.effect IS NOT DISTINCT FROM o.effect;

        DELETE FROM permission_policy o
        USING permission_policy n
        WHERE o.resource = 'federated-idp'
          AND n.resource = 'trusted-issuer'
          AND n.scope = o.scope AND n.action IS NOT DISTINCT FROM o.action
          AND n.effect IS NOT DISTINCT FROM o.effect;

        UPDATE permission_policy SET resource = 'trusted-issuer' WHERE resource = 'federated-idp';
    END IF;
END
$$;

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
