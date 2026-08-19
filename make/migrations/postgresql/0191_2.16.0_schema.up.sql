/*
multi_format_package / multi_format_version: the rebuildable Postgres projection over the OCI
`_index` control artifact for native package formats (npm, Maven) served on top
of Harbor's OCI backend. Authoritative mutable state lives in OCI annotations;
these tables are a derived view (reconcilable from `_index`).
*/
CREATE TABLE multi_format_package (
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

CREATE TABLE multi_format_version (
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

CREATE INDEX idx_multi_format_package_proj_fmt ON multi_format_package(project_id, format);
CREATE INDEX idx_multi_format_version_pkg ON multi_format_version(package_id);
