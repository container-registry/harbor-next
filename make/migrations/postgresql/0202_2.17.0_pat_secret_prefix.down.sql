-- Remove secret_prefix column
DROP INDEX IF EXISTS idx_pat_secret_prefix;
ALTER TABLE personal_access_token DROP COLUMN IF EXISTS secret_prefix;