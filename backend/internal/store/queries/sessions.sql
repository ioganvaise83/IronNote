-- name: GetActiveSession :one
SELECT id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at
FROM workout_sessions
WHERE status = 'in_progress'
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
    e.is_archived        AS exercise_archived,
    st.id                AS set_id,
    st.set_number,
    st.weight_kg,
    st.repetitions,
    st.is_warmup,
    st.is_completed
FROM workout_sessions s
LEFT JOIN workout_session_exercises se ON se.workout_session_id = s.id
LEFT JOIN exercises e ON e.id = se.exercise_id
LEFT JOIN workout_sets st ON st.workout_exercise_id = se.id
WHERE
    (sqlc.narg('status')::text IS NULL OR s.status = sqlc.narg('status')::text)
    AND (sqlc.narg('started_from')::timestamptz IS NULL OR s.started_at >= sqlc.narg('started_from')::timestamptz)
    AND (sqlc.narg('started_to')::timestamptz IS NULL OR s.started_at < sqlc.narg('started_to')::timestamptz)
    AND (sqlc.narg('session_id')::bigint IS NULL OR s.id = sqlc.narg('session_id')::bigint)
ORDER BY s.started_at DESC, s.id DESC, se.sort_order, st.set_number;

-- name: GetSession :one
SELECT id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at
FROM workout_sessions
WHERE id = $1;

-- name: CreateSession :one
INSERT INTO workout_sessions (template_id, name, started_at, status, notes)
VALUES ($1, $2, $3, 'in_progress', $4)
RETURNING id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at;

-- name: UpdateSession :one
UPDATE workout_sessions
SET name = $2, notes = $3, started_at = $4
WHERE id = $1
RETURNING id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at;

-- name: FinishSession :one
UPDATE workout_sessions
SET status = 'completed', finished_at = $2
WHERE id = $1
RETURNING id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at;

-- name: ReopenSession :one
UPDATE workout_sessions
SET status = 'in_progress', finished_at = NULL
WHERE id = $1
RETURNING id, template_id, name, started_at, finished_at, status, notes, created_at, updated_at;

-- name: DeleteSession :execrows
DELETE FROM workout_sessions WHERE id = $1;

-- name: ListSessionExercises :many
SELECT
    se.id,
    se.workout_session_id,
    se.exercise_id,
    se.sort_order,
    se.notes,
    e.name AS exercise_name,
    e.equipment,
    e.is_archived
FROM workout_session_exercises se
JOIN exercises e ON e.id = se.exercise_id
WHERE se.workout_session_id = $1
ORDER BY se.sort_order;

-- name: GetSessionExercise :one
SELECT id, workout_session_id, exercise_id, sort_order, notes
FROM workout_session_exercises
WHERE id = $1 AND workout_session_id = $2;

-- name: FindSessionExerciseByID :one
SELECT id, workout_session_id, exercise_id, sort_order, notes
FROM workout_session_exercises
WHERE id = $1;

-- name: AddSessionExercise :one
INSERT INTO workout_session_exercises (workout_session_id, exercise_id, sort_order, notes)
VALUES ($1, $2, $3, $4)
RETURNING id, workout_session_id, exercise_id, sort_order, notes;

-- name: UpdateSessionExercise :one
UPDATE workout_session_exercises
SET notes = $3, sort_order = $4
WHERE id = $1 AND workout_session_id = $2
RETURNING id, workout_session_id, exercise_id, sort_order, notes;

-- name: SetSessionExerciseSortOrder :exec
UPDATE workout_session_exercises
SET sort_order = $2
WHERE id = $1;

-- name: DeleteSessionExercise :execrows
DELETE FROM workout_session_exercises
WHERE id = $1 AND workout_session_id = $2;

-- name: NextSessionExerciseSortOrder :one
SELECT COALESCE(MAX(sort_order), 0) + 1 AS next_order
FROM workout_session_exercises
WHERE workout_session_id = $1;

-- name: ListSessionSets :many
SELECT id, workout_exercise_id, set_number, weight_kg, repetitions, is_warmup, is_completed
FROM workout_sets
WHERE workout_exercise_id = $1
ORDER BY set_number;

-- name: ListSetsForSession :many
SELECT st.id, st.workout_exercise_id, st.set_number, st.weight_kg, st.repetitions, st.is_warmup, st.is_completed
FROM workout_sets st
JOIN workout_session_exercises se ON se.id = st.workout_exercise_id
WHERE se.workout_session_id = $1
ORDER BY se.sort_order, st.set_number;

-- name: GetSet :one
SELECT id, workout_exercise_id, set_number, weight_kg, repetitions, is_warmup, is_completed
FROM workout_sets
WHERE id = $1;

-- name: NextSetNumber :one
SELECT COALESCE(MAX(set_number), 0) + 1 AS next_number
FROM workout_sets
WHERE workout_exercise_id = $1;

-- name: AddSet :one
INSERT INTO workout_sets (workout_exercise_id, set_number, weight_kg, repetitions, is_warmup, is_completed)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, workout_exercise_id, set_number, weight_kg, repetitions, is_warmup, is_completed;

-- name: UpdateSet :one
UPDATE workout_sets
SET weight_kg = $2, repetitions = $3, is_warmup = $4, is_completed = $5
WHERE id = $1
RETURNING id, workout_exercise_id, set_number, weight_kg, repetitions, is_warmup, is_completed;

-- name: DeleteSet :execrows
DELETE FROM workout_sets WHERE id = $1;

-- name: SetSetNumber :exec
-- Keeps numbering contiguous after a deletion.
UPDATE workout_sets
SET set_number = $2
WHERE id = $1;

-- name: LastSetForExercise :one
SELECT id, workout_exercise_id, set_number, weight_kg, repetitions, is_warmup, is_completed
FROM workout_sets
WHERE workout_exercise_id = $1
ORDER BY set_number DESC
LIMIT 1;

-- name: PreviousExerciseSets :many
-- "Last time" for one exercise: the most recent *other* completed session that
-- contains it, so an in-progress workout can prefill the next attempt.
SELECT
    st.set_number,
    st.weight_kg,
    st.repetitions,
    st.is_warmup,
    s.started_at
FROM workout_sets st
JOIN workout_session_exercises se ON se.id = st.workout_exercise_id
JOIN workout_sessions s ON s.id = se.workout_session_id
WHERE se.exercise_id = $1
  AND s.status = 'completed'
  AND s.id <> sqlc.arg('exclude_session_id')::bigint
  AND s.started_at = (
      SELECT MAX(s2.started_at)
      FROM workout_sessions s2
      JOIN workout_session_exercises se2 ON se2.workout_session_id = s2.id
      WHERE se2.exercise_id = $1 AND s2.status = 'completed' AND s2.id <> $2
  )
ORDER BY st.set_number;
