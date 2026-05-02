/* Track when an artifact row last changed (e.g. tag attached/detached). See goharbor/harbor#23149. */
ALTER TABLE artifact ADD COLUMN IF NOT EXISTS update_time timestamp default CURRENT_TIMESTAMP;
