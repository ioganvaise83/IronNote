-- name: ListMuscleGroups :many
SELECT id, name, description, created_at, updated_at
FROM muscle_groups
ORDER BY name;

-- name: GetMuscleGroup :one
SELECT id, name, description, created_at, updated_at
FROM muscle_groups
WHERE id = $1;

-- name: CreateMuscleGroup :one
INSERT INTO muscle_groups (name, description)
VALUES ($1, $2)
RETURNING id, name, description, created_at, updated_at;

-- name: UpdateMuscleGroup :one
UPDATE muscle_groups
SET name = $2, description = $3
WHERE id = $1
RETURNING id, name, description, created_at, updated_at;

-- name: DeleteMuscleGroup :execrows
DELETE FROM muscle_groups WHERE id = $1;

-- name: CountMuscleGroupUsage :one
-- An exercise or template referencing this group would break if it vanished.
SELECT COUNT(*)::bigint AS usage_count
FROM exercise_muscle_groups
WHERE muscle_group_id = $1;
