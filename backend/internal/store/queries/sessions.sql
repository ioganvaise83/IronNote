-- name: GetActiveSession :one
-- Scoped to the owner: "the workout in progress" is per person, never global.
SELECT id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at
FROM workout_sessions
WHERE status = 'in_progress' AND user_id = $1
LIMIT 1;

-- name: ListSessionDetails :many
-- One row per (session, exercise, set). The service groups these into sessions.
--
-- This is deliberately the single source for history *and* statistics: every
-- derived number (volume, set counts, Epley 1RM) is computed once in Go from
-- these raw rows, so the two screens can never disagree. A personal log is small
-- enough that fetching sets joined to sessions is cheaper than several aggregate
-- queries that drift apart.
SELECT
    s.id                 AS session_id,
    s.template_id,
    s.name,
    s.started_at,
    s.finished_at,
    s.status,
    s.notes,
    s.created_at,
    s.updated_at,
    se.id                AS session_exercise_id,
    se.exercise_id,
    se.sort_order,
    se.notes             AS exercise_notes,
    e.name               AS exercise_name,
    e.equipment,
    COALESCE(e.kind::text, 'weighted') AS exercise_kind,
    e.is_archived        AS exercise_archived,
    st.id                AS set_id,
    st.set_number,
    COALESCE(st.kind::text, 'weighted') AS kind,
    st.weight_kg,
    st.repetitions,
    st.duration_seconds,
    st.distance_meters,
    COALESCE(st.is_warmup, false) AS is_warmup,
    COALESCE(st.is_completed, false) AS is_completed
FROM workout_sessions s
LEFT JOIN workout_session_exercises se ON se.workout_session_id = s.id
LEFT JOIN exercises e ON e.id = se.exercise_id
LEFT JOIN workout_sets st ON st.workout_exercise_id = se.id
WHERE
    s.user_id = sqlc.arg('user_id')::bigint
    AND (sqlc.narg('status')::text IS NULL OR s.status = sqlc.narg('status')::text)
    AND (sqlc.narg('started_from')::timestamptz IS NULL OR s.started_at >= sqlc.narg('started_from')::timestamptz)
    AND (sqlc.narg('started_to')::timestamptz IS NULL OR s.started_at < sqlc.narg('started_to')::timestamptz)
    AND (sqlc.narg('session_id')::bigint IS NULL OR s.id = sqlc.narg('session_id')::bigint)
ORDER BY s.started_at DESC, s.id DESC, se.sort_order, st.set_number;

-- name: GetSession :one
-- Always owner-scoped. There is deliberately no unscoped "get session by id":
-- forgetting the filter then becomes impossible rather than a data leak.
SELECT id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at
FROM workout_sessions
WHERE id = $1 AND user_id = $2;

-- name: CreateSession :one
INSERT INTO workout_sessions (user_id, template_id, name, started_at, status, notes)
VALUES ($1, $2, $3, $4, 'in_progress', $5)
RETURNING id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at;

-- name: UpdateSession :one
UPDATE workout_sessions
SET name = $2, notes = $3, started_at = $4
WHERE id = $1 AND user_id = $5
RETURNING id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at;

-- name: FinishSession :one
UPDATE workout_sessions
SET status = 'completed', finished_at = $2
WHERE id = $1 AND user_id = $3
RETURNING id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at;

-- name: ReopenSession :one
UPDATE workout_sessions
SET status = 'in_progress', finished_at = NULL
WHERE id = $1 AND user_id = $2
RETURNING id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at;

-- name: DeleteSession :execrows
DELETE FROM workout_sessions WHERE id = $1 AND user_id = $2;

-- name: ListSessionExercises :many
SELECT
    se.id,
    se.workout_session_id,
    se.exercise_id,
    se.sort_order,
    se.notes,
    e.name AS exercise_name,
    e.equipment,
    e.kind::text AS exercise_kind,
    e.is_archived
FROM workout_session_exercises se
JOIN exercises e ON e.id = se.exercise_id
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE se.workout_session_id = $1 AND s.user_id = $2
ORDER BY se.sort_order;

-- name: GetSessionExercise :one
-- Ownership is checked through the parent session, so a guessed exercise id from
-- another user's workout does not resolve.
SELECT se.id, se.workout_session_id, se.exercise_id, se.sort_order, se.notes
FROM workout_session_exercises se
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE se.id = $1 AND se.workout_session_id = $2 AND s.user_id = $3;

-- name: FindSessionExerciseByID :one
SELECT se.id, se.workout_session_id, se.exercise_id, se.sort_order, se.notes
FROM workout_session_exercises se
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE se.id = $1 AND s.user_id = $2;

-- name: AddSessionExercise :one
-- The SELECT guard makes the insert fail (no rows) when the session belongs to
-- somebody else, so an unowned write is impossible even if a caller forgets.
INSERT INTO workout_session_exercises (workout_session_id, exercise_id, sort_order, notes)
SELECT $1, $2, $3, $4
WHERE EXISTS (
    SELECT 1 FROM workout_sessions
    WHERE id = $1 AND user_id = $5
)
RETURNING id, workout_session_id, exercise_id, sort_order, notes;

-- name: UpdateSessionExercise :one
UPDATE workout_session_exercises se
SET notes = $3, sort_order = $4
FROM workout_sessions s
WHERE se.id = $1 AND se.workout_session_id = $2 AND s.id = se.workout_session_id
  AND s.user_id = $5
RETURNING se.id, se.workout_session_id, se.exercise_id, se.sort_order, se.notes;

-- name: SetSessionExerciseSortOrder :exec
UPDATE workout_session_exercises se
SET sort_order = $2
FROM workout_sessions s
WHERE se.id = $1 AND s.id = se.workout_session_id AND s.user_id = $3;

-- name: DeleteSessionExercise :execrows
DELETE FROM workout_session_exercises se
USING workout_sessions s
WHERE se.id = $1 AND se.workout_session_id = $2
  AND s.id = se.workout_session_id AND s.user_id = $3;

-- name: NextSessionExerciseSortOrder :one
SELECT COALESCE(MAX(se.sort_order), 0) + 1 AS next_order
FROM workout_session_exercises se
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE se.workout_session_id = $1 AND s.user_id = $2;

-- name: ListSessionSets :many
SELECT st.id, st.workout_exercise_id, st.set_number, st.kind::text AS kind, st.weight_kg, st.repetitions,
       st.duration_seconds, st.distance_meters,
       st.is_warmup, st.is_completed
FROM workout_sets st
JOIN workout_session_exercises se ON se.id = st.workout_exercise_id
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE st.workout_exercise_id = $1 AND s.user_id = $2
ORDER BY st.set_number;

-- name: ListSetsForSession :many
SELECT st.id, st.workout_exercise_id, st.set_number, st.kind::text AS kind, st.weight_kg, st.repetitions,
       st.duration_seconds, st.distance_meters, st.is_warmup, st.is_completed
FROM workout_sets st
JOIN workout_session_exercises se ON se.id = st.workout_exercise_id
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE se.workout_session_id = $1 AND s.user_id = $2
ORDER BY se.sort_order, st.set_number;

-- name: GetSet :one
SELECT st.id, st.workout_exercise_id, st.set_number, st.kind::text AS kind, st.weight_kg, st.repetitions,
       st.duration_seconds, st.distance_meters,
       st.is_warmup, st.is_completed
FROM workout_sets st
JOIN workout_session_exercises se ON se.id = st.workout_exercise_id
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE st.id = $1 AND s.user_id = $2;

-- name: NextSetNumber :one
SELECT COALESCE(MAX(st.set_number), 0) + 1 AS next_number
FROM workout_sets st
JOIN workout_session_exercises se ON se.id = st.workout_exercise_id
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE st.workout_exercise_id = $1 AND s.user_id = $2;

-- name: AddSet :one
-- The EXISTS guard rejects a write to an exercise row the actor does not own.
INSERT INTO workout_sets (
    workout_exercise_id, set_number, kind, weight_kg, repetitions,
    duration_seconds, distance_meters, is_warmup, is_completed
)
SELECT sqlc.arg('workout_exercise_id'), sqlc.arg('set_number'), sqlc.arg('kind')::exercise_kind,
       sqlc.arg('weight_kg'), sqlc.arg('repetitions'),
       sqlc.arg('duration_seconds'), sqlc.arg('distance_meters'),
       sqlc.arg('is_warmup'), sqlc.arg('is_completed')
WHERE EXISTS (
    SELECT 1
    FROM workout_session_exercises se
    JOIN workout_sessions s ON s.id = se.workout_session_id
    WHERE se.id = sqlc.arg('workout_exercise_id') AND s.user_id = sqlc.arg('user_id')
)
RETURNING id, workout_exercise_id, set_number, kind::text AS kind, weight_kg, repetitions,
          duration_seconds, distance_meters, is_warmup, is_completed;

-- name: UpdateSet :one
UPDATE workout_sets st
SET weight_kg = sqlc.arg('weight_kg'),
    repetitions = sqlc.arg('repetitions'),
    duration_seconds = sqlc.arg('duration_seconds'),
    distance_meters = sqlc.arg('distance_meters'),
    is_warmup = sqlc.arg('is_warmup'),
    is_completed = sqlc.arg('is_completed')
FROM workout_session_exercises se, workout_sessions s
WHERE st.id = sqlc.arg('id')
  AND se.id = st.workout_exercise_id
  AND s.id = se.workout_session_id
  AND s.user_id = sqlc.arg('user_id')
RETURNING st.id, st.workout_exercise_id, st.set_number, st.kind::text AS kind, st.weight_kg,
          st.repetitions, st.duration_seconds, st.distance_meters, st.is_warmup, st.is_completed;

-- name: DeleteSet :execrows
DELETE FROM workout_sets st
USING workout_session_exercises se, workout_sessions s
WHERE st.id = $1
  AND se.id = st.workout_exercise_id
  AND s.id = se.workout_session_id
  AND s.user_id = $2;

-- name: SetSetNumber :exec
-- Keeps numbering contiguous after a deletion.
UPDATE workout_sets st
SET set_number = $2
FROM workout_session_exercises se, workout_sessions s
WHERE st.id = $1
  AND se.id = st.workout_exercise_id
  AND s.id = se.workout_session_id
  AND s.user_id = $3;

-- name: LastSetForExercise :one
SELECT st.id, st.workout_exercise_id, st.set_number, st.kind::text AS kind, st.weight_kg, st.repetitions,
       st.duration_seconds, st.distance_meters,
       st.is_warmup, st.is_completed
FROM workout_sets st
JOIN workout_session_exercises se ON se.id = st.workout_exercise_id
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE st.workout_exercise_id = $1 AND s.user_id = $2
ORDER BY st.set_number DESC
LIMIT 1;

-- name: PreviousExerciseSets :many
-- "Last time" for one exercise: the most recent *other* completed session that
-- contains it, so an in-progress workout can prefill the next attempt.
SELECT
    st.set_number,
    COALESCE(st.kind::text, 'weighted') AS kind,
    st.weight_kg,
    st.repetitions,
    st.duration_seconds,
    st.distance_meters,
    COALESCE(st.is_warmup, false) AS is_warmup,
    s.started_at
FROM workout_sets st
JOIN workout_session_exercises se ON se.id = st.workout_exercise_id
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE se.exercise_id = sqlc.arg('exercise_id')::bigint
  AND s.user_id = sqlc.arg('user_id')::bigint
  AND s.status = 'completed'
  AND s.id <> sqlc.arg('exclude_session_id')::bigint
  AND s.started_at = (
      SELECT MAX(s2.started_at)
      FROM workout_sessions s2
      JOIN workout_session_exercises se2 ON se2.workout_session_id = s2.id
      WHERE se2.exercise_id = sqlc.arg('exercise_id')::bigint
        AND s2.user_id = sqlc.arg('user_id')::bigint
        AND s2.status = 'completed'
        AND s2.id <> sqlc.arg('exclude_session_id')::bigint
  )
ORDER BY st.set_number;
