-- Table: identity_providers
-- Stores federated identity providers

CREATE TABLE IF NOT EXISTS identity_providers (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    issuer TEXT NOT NULL,
    openid_config_url TEXT,
    offline_validation BOOLEAN NOT NULL DEFAULT FALSE,
    supported_algorithms TEXT,
    claims_supported TEXT,
    jwks_uri TEXT,
    jwks_keys JSONB,
    jwks_cached_at TIMESTAMP,
    jwks_expires_at TIMESTAMP,
    jwks_last_fetch_attempt TIMESTAMP,
    project_id INT NOT NULL DEFAULT 0,
    creation_time TIMESTAMP DEFAULT NOW(),
    update_time TIMESTAMP DEFAULT NOW(),
    UNIQUE (issuer, project_id)
);

-- Table: robot_identity_providers
-- Join table linking robots and identity providers

CREATE TABLE IF NOT EXISTS robot_identity_providers (
    id SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id INT NOT NULL REFERENCES robot(id) ON DELETE CASCADE,
    creation_time TIMESTAMP DEFAULT NOW(),
    UNIQUE (identity_provider_id, robot_id)
);

-- Table: claim_rules
-- Stores JWT claim/value pairs for authentication
-- Scoped to identity provider or specific robot

CREATE TABLE IF NOT EXISTS claim_rules (
    id SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id INT NOT NULL DEFAULT 0,
    claim_path TEXT NOT NULL,
    value TEXT,
    creation_time TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_claim_rules_lookup
ON claim_rules (identity_provider_id, claim_path, value, robot_id);

-- Index for efficient cache lookup and rate limit checks
CREATE INDEX IF NOT EXISTS idx_identity_providers_jwks_cache
ON identity_providers (id, jwks_expires_at, jwks_last_fetch_attempt);
