-- name: ListTemplates :many
SELECT
    t.id,
    t.name,
    t.description,
    t.created_at,
    t.updated_at,
    COUNT(te.id)::bigint AS exercise_count,
    COALESCE(SUM(te.planned_sets), 0)::bigint AS planned_sets
FROM workout_templates t
LEFT JOIN workout_template_exercises te ON te.template_id = t.id
GROUP BY t.id
ORDER BY t.name;

-- name: GetTemplate :one
SELECT id, name, description, created_at, updated_at
FROM workout_templates
WHERE id = $1;

-- name: ListTemplateExercises :many
SELECT
    te.id,
    te.template_id,
    te.exercise_id,
    te.sort_order,
    te.planned_sets,
    te.planned_reps,
    te.planned_weight_kg,
    e.name AS exercise_name,
    e.is_archived AS exercise_archived
FROM workout_template_exercises te
JOIN exercises e ON e.id = te.exercise_id
WHERE te.template_id = $1
ORDER BY te.sort_order;

-- name: CreateTemplate :one
INSERT INTO workout_templates (name, description)
VALUES ($1, $2)
RETURNING id, name, description, created_at, updated_at;

-- name: UpdateTemplate :one
UPDATE workout_templates
SET name = $2, description = $3
WHERE id = $1
RETURNING id, name, description, created_at, updated_at;

-- name: DeleteTemplate :execrows
DELETE FROM workout_templates WHERE id = $1;

-- name: DeleteTemplateExercises :exec
DELETE FROM workout_template_exercises WHERE template_id = $1;

-- name: AddTemplateExercise :exec
INSERT INTO workout_template_exercises
    (template_id, exercise_id, sort_order, planned_sets, planned_reps, planned_weight_kg)
VALUES ($1, $2, $3, $4, $5, $6);
