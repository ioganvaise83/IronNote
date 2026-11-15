-- name: AvailableStatPeriod :one
SELECT
    MIN(started_at)::timestamptz AS first_session_at,
    MAX(started_at)::timestamptz AS last_session_at
FROM workout_sessions
WHERE status = 'completed';
