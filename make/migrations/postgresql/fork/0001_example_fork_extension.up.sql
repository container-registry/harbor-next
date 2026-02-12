-- Fork Migration 0001: Example fork-specific schema extension
-- This demonstrates the pattern for fork-specific migrations.
-- Each migration must have a paired .up.sql and .down.sql file.
--
-- IMPORTANT: Fork migrations MUST be idempotent where possible.
-- Use "IF NOT EXISTS" / "IF EXISTS" guards so re-applying after
-- an upstream merge does not fail on already-existing objects.

-- Example: add a fork-specific metadata table
CREATE TABLE IF NOT EXISTS fork_metadata (
    id SERIAL PRIMARY KEY,
    resource_type VARCHAR(255) NOT NULL,
    resource_id BIGINT NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT,
    creation_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (resource_type, resource_id, key)
);

-- Example: add a trigger for auto-updating update_time
CREATE OR REPLACE FUNCTION fork_metadata_update_time()
RETURNS TRIGGER AS $$
BEGIN
    NEW.update_time = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS fork_metadata_update_time_trigger ON fork_metadata;
CREATE TRIGGER fork_metadata_update_time_trigger
    BEFORE UPDATE ON fork_metadata
    FOR EACH ROW
    EXECUTE FUNCTION fork_metadata_update_time();
