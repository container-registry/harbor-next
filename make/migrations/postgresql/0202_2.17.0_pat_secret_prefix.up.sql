-- Add secret_prefix column for efficient PAT lookup
ALTER TABLE personal_access_token ADD COLUMN IF NOT EXISTS secret_prefix VARCHAR(8);
CREATE INDEX IF NOT EXISTS idx_pat_secret_prefix ON personal_access_token(secret_prefix) WHERE secret_prefix IS NOT NULL;