-- name: ListExercises :many
-- The catalogue is small enough that joining the muscle groups in one query keeps
-- the UI from doing N+1 round trips.
SELECT
    e.id,
    e.name,
    e.description,
    e.equipment,
    e.kind::text AS kind,
    e.is_archived,
    e.created_at,
    e.updated_at,
    COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'muscleGroupId', emg.muscle_group_id,
                'muscleGroupName', mg.name,
                'isPrimary', emg.is_primary
            ) ORDER BY emg.is_primary DESC, mg.name
        ) FILTER (WHERE emg.muscle_group_id IS NOT NULL),
        '[]'::jsonb
    )::text AS muscle_groups
FROM exercises e
LEFT JOIN exercise_muscle_groups emg ON emg.exercise_id = e.id
LEFT JOIN muscle_groups mg ON mg.id = emg.muscle_group_id
WHERE
    (sqlc.narg('include_archived')::boolean IS TRUE OR e.is_archived = false)
    AND (sqlc.narg('muscle_group_id')::bigint IS NULL
         OR EXISTS (
             SELECT 1 FROM exercise_muscle_groups x
             WHERE x.exercise_id = e.id AND x.muscle_group_id = sqlc.narg('muscle_group_id')::bigint
         ))
    AND (sqlc.narg('search')::text IS NULL OR e.name ILIKE '%' || sqlc.narg('search')::text || '%')
GROUP BY e.id
ORDER BY e.name;

-- name: GetExercise :one
SELECT
    e.id,
    e.name,
    e.description,
    e.equipment,
    e.kind::text AS kind,
    e.is_archived,
    e.created_at,
    e.updated_at,
    COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'muscleGroupId', emg.muscle_group_id,
                'muscleGroupName', mg.name,
                'isPrimary', emg.is_primary
            ) ORDER BY emg.is_primary DESC, mg.name
        ) FILTER (WHERE emg.muscle_group_id IS NOT NULL),
        '[]'::jsonb
    )::text AS muscle_groups
FROM exercises e
LEFT JOIN exercise_muscle_groups emg ON emg.exercise_id = e.id
LEFT JOIN muscle_groups mg ON mg.id = emg.muscle_group_id
WHERE e.id = $1
GROUP BY e.id;

-- name: CreateExercise :one
INSERT INTO exercises (name, description, equipment, kind)
VALUES ($1, $2, $3, sqlc.arg('kind')::exercise_kind)
RETURNING id;

-- name: UpdateExercise :one
UPDATE exercises
SET name = $2, description = $3, equipment = $4, kind = sqlc.arg('kind')::exercise_kind
WHERE id = $1
RETURNING id;

-- name: SetExerciseArchived :one
UPDATE exercises
SET is_archived = $2
WHERE id = $1
RETURNING id, is_archived;

-- name: DeleteExercise :execrows
DELETE FROM exercises WHERE id = $1;

-- name: CountExerciseHistory :one
-- Anything that would lose meaning if the exercise row disappeared.
SELECT
    (SELECT COUNT(*) FROM workout_session_exercises wse WHERE wse.exercise_id = $1)::bigint
  + (SELECT COUNT(*) FROM workout_template_exercises wte WHERE wte.exercise_id = $1)::bigint
    AS usage_count;

-- name: ReplaceExerciseMuscleGroups :exec
DELETE FROM exercise_muscle_groups WHERE exercise_id = $1;

-- name: AddExerciseMuscleGroup :exec
INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_id, is_primary)
VALUES ($1, $2, $3)
ON CONFLICT (exercise_id, muscle_group_id) DO UPDATE SET is_primary = EXCLUDED.is_primary;
