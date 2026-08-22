-- Create user_group_membership table to persist external auth group memberships
CREATE TABLE IF NOT EXISTS user_group_membership (
    user_id INTEGER NOT NULL,
    group_id INTEGER NOT NULL,
    creation_time TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);
-- No separate index on user_id: the (user_id, group_id) primary key
-- already supports lookups filtered by user_id alone.