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
    audience                TEXT NOT NULL DEFAULT '',
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
    update_time             TIMESTAMP DEFAULT NOW()
);

-- A trusted issuer is identified by issuer plus audience; the application keeps
-- the pair unique, like the name.
ALTER TABLE identity_providers ADD COLUMN IF NOT EXISTS audience TEXT NOT NULL DEFAULT '';

-- Drops the legacy uniqueness on (issuer, project_id); fresh installs never get it.
ALTER TABLE identity_providers DROP CONSTRAINT IF EXISTS identity_providers_issuer_project_id_key;

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

-- Trusted issuers created before the audience column kept it as an issuer-level
-- aud claim rule. Copy it into the column so they keep matching tokens. Only an
-- empty audience with a single aud value is filled, so a later run changes nothing.
-- The aud claim stays, so a core rolled back to before the column still checks it.
UPDATE identity_providers ip
SET audience = cr.value
FROM claim_rules cr
WHERE ip.audience = ''
  AND cr.identity_provider_id = ip.id
  AND cr.robot_id = 0
  AND cr.claim_path = 'aud'
  AND cr.value <> ''
  AND NOT EXISTS (
      SELECT 1
      FROM claim_rules other
      WHERE other.identity_provider_id = ip.id
        AND other.robot_id = 0
        AND other.claim_path = 'aud'
        AND other.value <> cr.value
  )
  -- issuer plus audience must stay unique, so skip a value another trusted
  -- issuer of the same URL already has or would also get
  AND NOT EXISTS (
      SELECT 1
      FROM identity_providers same
      WHERE same.id <> ip.id
        AND same.issuer = ip.issuer
        AND (same.audience = cr.value
             OR (same.audience = ''
                 AND EXISTS (
                     SELECT 1
                     FROM claim_rules sc
                     WHERE sc.identity_provider_id = same.id
                       AND sc.robot_id = 0
                       AND sc.claim_path = 'aud'
                       AND sc.value = cr.value)))
  );

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

-- Custom project roles (upstream goharbor/harbor#23804). Every project role,
-- the five built-ins included, keeps its grants in
-- permission_policy/role_permission (goharbor/harbor#24029); core seeds the
-- built-in grants at startup.
ALTER TABLE role ADD COLUMN IF NOT EXISTS is_builtin   BOOLEAN      NOT NULL DEFAULT FALSE;
ALTER TABLE role ADD COLUMN IF NOT EXISTS description  TEXT;
ALTER TABLE role ADD COLUMN IF NOT EXISTS modified     BOOLEAN      NOT NULL DEFAULT FALSE;
ALTER TABLE role ADD COLUMN IF NOT EXISTS created_by   VARCHAR(255);
ALTER TABLE role ADD COLUMN IF NOT EXISTS created_at   TIMESTAMP WITH TIME ZONE;
ALTER TABLE role ADD COLUMN IF NOT EXISTS modified_by  VARCHAR(255);
ALTER TABLE role ADD COLUMN IF NOT EXISTS modified_at  TIMESTAMP WITH TIME ZONE;

-- Role names match the API/UI contract (was varchar(20)) and are unique
-- case-insensitively, so "Maintainer" and "maintainer" cannot coexist.
ALTER TABLE role ALTER COLUMN name TYPE varchar(255);
CREATE UNIQUE INDEX IF NOT EXISTS uq_role_name ON role (lower(name));

-- Roles seeded by the numbered migrations are built-in and immutable.
UPDATE role SET is_builtin = TRUE
WHERE name IN ('projectAdmin', 'developer', 'guest', 'maintainer', 'limitedGuest')
  AND is_builtin = FALSE;

-- A role still assigned to a member cannot be deleted, which closes the
-- count-then-delete race in the role Delete controller.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_project_member_role'
    ) THEN
        ALTER TABLE project_member
            ADD CONSTRAINT fk_project_member_role
            FOREIGN KEY (role) REFERENCES role (role_id) ON DELETE RESTRICT;
    END IF;
END
$$;

-- Policy change notification for the in-memory role policy store
-- (goharbor/harbor#24029). Every core replica reloads its policy when this
-- fires, and compares policy_version on a timer in case a NOTIFY is missed.
-- The trigger sends NOTIFY inside the writing transaction, so a rollback sends
-- nothing; the payload is only the new version and the writing replica.
CREATE TABLE IF NOT EXISTS policy_version (
  only_row    boolean   PRIMARY KEY DEFAULT TRUE,
  version     bigint    NOT NULL DEFAULT 1,
  update_time timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT policy_version_single_row CHECK (only_row)
);
INSERT INTO policy_version (only_row) VALUES (TRUE) ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION harbor_policy_bump() RETURNS void AS $$
DECLARE
  v bigint;
BEGIN
  UPDATE policy_version
     SET version = version + 1, update_time = CURRENT_TIMESTAMP
   WHERE only_row
  RETURNING version INTO v;

  PERFORM pg_notify(
    'harbor_policy',
    v::text || ':' || coalesce(current_setting('harbor.origin', TRUE), '?'));
END;
$$ LANGUAGE plpgsql;

-- Robots write to the same tables, so only rows linked to a project role
-- bump the version; otherwise creating a robot would reload every replica.
-- An UPDATE also checks the old rows: moving a grant away from a project role
-- removes it from the policy.
CREATE OR REPLACE FUNCTION harbor_role_permission_notify() RETURNS trigger AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM changed WHERE role_type = 'project-role') THEN
    PERFORM harbor_policy_bump();
  ELSIF TG_OP = 'UPDATE' THEN
    -- a separate statement: "previous" exists only in the UPDATE trigger
    IF EXISTS (SELECT 1 FROM previous WHERE role_type = 'project-role') THEN
      PERFORM harbor_policy_bump();
    END IF;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION harbor_permission_policy_notify() RETURNS trigger AS $$
BEGIN
  IF EXISTS (
    SELECT 1
      FROM role_permission rp
      JOIN changed c ON c.id = rp.permission_policy_id
     WHERE rp.role_type = 'project-role'
  ) THEN
    PERFORM harbor_policy_bump();
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Statement-level triggers with transition tables, one per event. They are
-- created only when missing: this file runs on every core start, and
-- DROP/CREATE TRIGGER would take an exclusive lock on both tables each time.
DO $$
DECLARE
  t record;
BEGIN
  FOR t IN SELECT * FROM (VALUES
      ('role_permission',   'role_permission_notify_ins',   'INSERT', 'NEW', 'harbor_role_permission_notify'),
      ('role_permission',   'role_permission_notify_upd',   'UPDATE', 'OLD TABLE AS previous NEW', 'harbor_role_permission_notify'),
      ('role_permission',   'role_permission_notify_del',   'DELETE', 'OLD', 'harbor_role_permission_notify'),
      ('permission_policy', 'permission_policy_notify_ins', 'INSERT', 'NEW', 'harbor_permission_policy_notify'),
      ('permission_policy', 'permission_policy_notify_upd', 'UPDATE', 'NEW', 'harbor_permission_policy_notify'),
      ('permission_policy', 'permission_policy_notify_del', 'DELETE', 'OLD', 'harbor_permission_policy_notify')
    ) AS v(tbl, name, event, transition, fn)
  LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_trigger
       WHERE tgrelid = to_regclass(t.tbl) AND tgname = t.name
    ) THEN
      EXECUTE format(
        'CREATE TRIGGER %I AFTER %s ON %I REFERENCING %s TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION %I()',
        t.name, t.event, t.tbl, t.transition, t.fn);
    END IF;
  END LOOP;
END
$$;
