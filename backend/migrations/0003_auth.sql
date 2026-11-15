-- Multi-user support: accounts, sessions, and ownership of training data.
--
-- Order matters. Users are created first, then every table that belongs to a
-- person gains a NOT NULL owner column backfilled to the bootstrap admin. Doing
-- the backfill in the same migration means an existing single-user install keeps
-- all of its history instead of orphaning it.

CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      TEXT        NOT NULL,
    -- Case-insensitive uniqueness: "Admin" and "admin" are the same account.
    username_key  TEXT        GENERATED ALWAYS AS (lower(username)) STORED,
    display_name  TEXT        NOT NULL DEFAULT '',
    -- Argon2id encoded hash ("$argon2id$v=19$m=65536,t=3,p=2$..."). Never a
    -- plaintext password, never reversible.
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'user',
    is_active     BOOLEAN     NOT NULL DEFAULT true,
    -- Forces a password change on next login (admin-created accounts).
    must_change_password BOOLEAN NOT NULL DEFAULT false,
    failed_login_count   INTEGER NOT NULL DEFAULT 0,
    -- Set after too many failures; login is refused until this instant.
    locked_until  TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_role_check CHECK (role IN ('admin', 'user')),
    CONSTRAINT users_username_not_blank CHECK (btrim(username) <> ''),
    CONSTRAINT users_username_format_check CHECK (username ~ '^[A-Za-z0-9._-]{3,32}$'),
    CONSTRAINT users_failed_login_check CHECK (failed_login_count >= 0)
);

CREATE UNIQUE INDEX users_username_key_idx ON users (username_key);

CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- At least one admin must exist at all times, otherwise nobody can manage the
-- catalogue. The bootstrap path is the only way the first one appears; this
-- partial index makes "is there an admin" a cheap query used before any
-- role-changing operation.
CREATE INDEX users_admins_idx ON users (id) WHERE role = 'admin' AND is_active;

CREATE TABLE auth_sessions (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- SHA-256 of the bearer token. The token itself is never stored: a database
    -- leak must not hand over live sessions.
    token_hash    BYTEA       NOT NULL,
    -- CSRF token bound to this session, compared in constant time on writes.
    csrf_token    TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    -- Set when the user signs out or the account is disabled; kept for a short
    -- while so a replayed token is rejected with a clear reason.
    revoked_at    TIMESTAMPTZ,
    ip_address    TEXT,
    user_agent    TEXT,
    CONSTRAINT auth_sessions_token_key UNIQUE (token_hash)
);

CREATE INDEX auth_sessions_user_idx ON auth_sessions (user_id, expires_at DESC);
CREATE INDEX auth_sessions_expiry_idx ON auth_sessions (expires_at);

-- Rate limiting for login attempts. Kept in the database so a restart cannot be
-- used to reset the counter; keyed by a hash of the IP + login name so the table
-- does not become a log of every username tried.
CREATE TABLE login_attempts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    attempt_key TEXT        NOT NULL,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    succeeded   BOOLEAN     NOT NULL DEFAULT false
);

CREATE INDEX login_attempts_key_time_idx ON login_attempts (attempt_key, attempted_at DESC);
-- Housekeeping deletes old rows; this index makes that cheap.
CREATE INDEX login_attempts_time_idx ON login_attempts (attempted_at);
