-- name: CountUsers :one
SELECT COUNT(*)::bigint AS user_count FROM users;

-- name: CountActiveAdmins :one
SELECT COUNT(*)::bigint AS admin_count
FROM users
WHERE role = 'admin' AND is_active;

-- name: GetUserByID :one
SELECT id, username, display_name, password_hash, role, is_active,
       must_change_password, failed_login_count, locked_until, last_login_at,
       created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByUsername :one
-- Login lookup is case-insensitive via the generated username_key column.
SELECT id, username, display_name, password_hash, role, is_active,
       must_change_password, failed_login_count, locked_until, last_login_at,
       created_at, updated_at
FROM users
WHERE username_key = lower(sqlc.arg('username')::text);

-- name: ListUsers :many
SELECT id, username, display_name, role, is_active, must_change_password,
       last_login_at, created_at, updated_at
FROM users
ORDER BY role, lower(username);

-- name: CreateUser :one
INSERT INTO users (username, display_name, password_hash, role, must_change_password)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: UpdateUserProfile :one
UPDATE users
SET display_name = $2, role = $3, is_active = $4
WHERE id = $1
RETURNING id;

-- name: SetUserPassword :one
-- Also clears the forced-change flag and any lockout: an admin reset is the
-- recovery path for a locked-out account.
UPDATE users
SET password_hash = $2, must_change_password = $3,
    failed_login_count = 0, locked_until = NULL
WHERE id = $1
RETURNING id;

-- name: DeleteUser :execrows
-- The caller must have already verified this is not the last active admin.
DELETE FROM users WHERE id = $1;

-- name: RegisterFailedLogin :one
-- Counts the failure and locks the account once the threshold is crossed. Done in
-- one statement so concurrent attempts cannot lose an increment.
UPDATE users
SET failed_login_count = failed_login_count + 1,
    locked_until = CASE
        WHEN failed_login_count + 1 >= sqlc.arg('threshold')::int
        THEN now() + sqlc.arg('lock_for')::interval
        ELSE locked_until
    END
WHERE id = $1
RETURNING failed_login_count, locked_until;

-- name: ClearFailedLogins :exec
UPDATE users
SET failed_login_count = 0, locked_until = NULL, last_login_at = now()
WHERE id = $1;

-- name: CreateAuthSession :exec
INSERT INTO auth_sessions (user_id, token_hash, csrf_token, expires_at, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetAuthSession :one
-- Joins the user so a disabled account is rejected on the very next request
-- rather than at token expiry.
SELECT
    s.id,
    s.user_id,
    s.csrf_token,
    s.created_at,
    s.last_seen_at,
    s.expires_at,
    s.revoked_at,
    u.username,
    u.display_name,
    u.role,
    u.is_active,
    u.must_change_password
FROM auth_sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: TouchAuthSession :exec
UPDATE auth_sessions SET last_seen_at = now() WHERE id = $1;

-- name: RevokeAuthSession :exec
UPDATE auth_sessions SET revoked_at = now() WHERE id = $1;

-- name: DeleteAuthSessionByHash :exec
DELETE FROM auth_sessions WHERE token_hash = $1;

-- name: DeleteSessionsForUser :exec
DELETE FROM auth_sessions WHERE user_id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM auth_sessions WHERE expires_at < now() OR revoked_at IS NOT NULL;

-- name: RotateSessionCSRF :exec
UPDATE auth_sessions SET csrf_token = $2 WHERE id = $1;

-- name: RecordLoginAttempt :exec
INSERT INTO login_attempts (attempt_key, succeeded) VALUES ($1, $2);

-- name: CountRecentLoginFailures :one
-- Sliding window used for the per-IP limit, so a spoofed or shared address can
-- still only make a bounded number of guesses.
SELECT COUNT(*)::bigint AS failure_count
FROM login_attempts
WHERE attempt_key = $1
  AND succeeded = false
  AND attempted_at > now() - sqlc.arg('window')::interval;

-- name: PruneLoginAttempts :execrows
DELETE FROM login_attempts WHERE attempted_at < now() - sqlc.arg('older_than')::interval;
