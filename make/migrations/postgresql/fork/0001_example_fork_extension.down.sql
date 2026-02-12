-- Fork Migration 0001 ROLLBACK: Remove fork-specific schema extension

DROP TRIGGER IF EXISTS fork_metadata_update_time_trigger ON fork_metadata;
DROP FUNCTION IF EXISTS fork_metadata_update_time();
DROP TABLE IF EXISTS fork_metadata;
