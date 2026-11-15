-- Ownership of training data.
--
-- This runs after 0003 and after the bootstrap admin has been provisioned by the
-- migration runner (see internal/database/migrate hooks). Backfilling to that
-- account keeps an existing single-user installation's entire history attached to
-- a real person instead of orphaning it.

ALTER TABLE workout_sessions  ADD COLUMN user_id BIGINT;
ALTER TABLE workout_templates ADD COLUMN user_id BIGINT;

UPDATE workout_sessions
   SET user_id = (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1);

UPDATE workout_templates
   SET user_id = (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1);

-- The backfill above can only be complete if an admin exists. If it is not,
-- these constraints fail and the migration is rolled back: better to stop than
-- to leave a user's history unowned.
ALTER TABLE workout_sessions
    ALTER COLUMN user_id SET NOT NULL,
    ADD CONSTRAINT workout_sessions_user_fk
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;

ALTER TABLE workout_templates
    ALTER COLUMN user_id SET NOT NULL,
    ADD CONSTRAINT workout_templates_user_fk
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;

-- "One workout in flight" becomes a per-user rule.
DROP INDEX IF EXISTS workout_sessions_single_active_key;
CREATE UNIQUE INDEX workout_sessions_single_active_per_user_key
    ON workout_sessions (user_id) WHERE status = 'in_progress';

-- Every history and statistics query now starts from the owner.
DROP INDEX IF EXISTS workout_sessions_started_idx;
CREATE INDEX workout_sessions_user_started_idx
    ON workout_sessions (user_id, started_at DESC);
CREATE INDEX workout_sessions_user_status_idx
    ON workout_sessions (user_id, status, started_at DESC);
CREATE INDEX workout_templates_user_idx ON workout_templates (user_id, name);
