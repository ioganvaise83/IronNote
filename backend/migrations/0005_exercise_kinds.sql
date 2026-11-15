-- Exercise kinds and time-based sets.
--
-- Until now every set was "weight × repetitions". Two real cases did not fit:
--   * calisthenics (подтягивания, отжимания) have repetitions but no weight;
--   * cardio (бег, плавание, эллипсоид) has neither — only a duration, and
--     sometimes a distance.
--
-- The kind lives on the *exercise* because it is a property of the movement, and
-- it is denormalised onto the set so a workout stays self-describing if the
-- catalogue entry is later edited. History must not change meaning retroactively.

CREATE TYPE exercise_kind AS ENUM ('weighted', 'bodyweight', 'duration');

ALTER TABLE exercises
    ADD COLUMN kind exercise_kind NOT NULL DEFAULT 'weighted';

-- Existing rows were all recorded as weight × reps. Exercises with no weight ever
-- recorded are reclassified as bodyweight so the UI stops asking for a weight that
-- was never used. This is a data-preserving heuristic: no set is modified.
UPDATE exercises e
SET kind = 'bodyweight'
WHERE NOT EXISTS (
    SELECT 1
    FROM workout_session_exercises se
    JOIN workout_sets st ON st.workout_exercise_id = se.id
    WHERE se.exercise_id = e.id AND st.weight_kg IS NOT NULL
);

ALTER TABLE workout_sets
    -- Duration of a time-based set, in seconds. NULL for repetitions-based sets.
    ADD COLUMN duration_seconds INTEGER,
    -- Optional distance for cardio (metres). Never derived from anything.
    ADD COLUMN distance_meters NUMERIC(9, 1),
    -- Snapshot of the kind at recording time, so history renders the same even if
    -- the catalogue entry changes later.
    ADD COLUMN kind exercise_kind;

-- Backfill: sets that already have repetitions are weighted or bodyweight;
-- everything keeps rendering exactly as before.
UPDATE workout_sets st
SET kind = CASE
        WHEN st.weight_kg IS NULL THEN 'bodyweight'::exercise_kind
        ELSE 'weighted'::exercise_kind
    END
WHERE st.kind IS NULL;

ALTER TABLE workout_sets ALTER COLUMN kind SET NOT NULL;

-- Repetitions become optional: a cardio set has none. The old NOT NULL must go,
-- but at least one measure (reps or duration) is still required.
ALTER TABLE workout_sets ALTER COLUMN repetitions DROP NOT NULL;

ALTER TABLE workout_sets
    ADD CONSTRAINT workout_sets_measure_check CHECK (
        (repetitions IS NOT NULL AND repetitions BETWEEN 1 AND 1000)
        OR (duration_seconds IS NOT NULL AND duration_seconds BETWEEN 1 AND 86400)
    ),
    ADD CONSTRAINT workout_sets_reps_kind_check CHECK (
        kind = 'duration' OR repetitions IS NOT NULL
    ),
    ADD CONSTRAINT workout_sets_duration_kind_check CHECK (
        kind <> 'duration' OR duration_seconds IS NOT NULL
    ),
    ADD CONSTRAINT workout_sets_distance_check CHECK (
        distance_meters IS NULL OR distance_meters BETWEEN 0 AND 1000000
    );

-- Existing constraint allowed reps 1..1000 unconditionally; replace it so a
-- duration-only set is valid.
ALTER TABLE workout_sets DROP CONSTRAINT IF EXISTS workout_sets_reps_check;

-- Templates may plan a duration instead of sets/reps for cardio.
ALTER TABLE workout_template_exercises
    ADD COLUMN planned_duration_seconds INTEGER,
    ADD CONSTRAINT workout_template_exercises_planned_duration_check
        CHECK (planned_duration_seconds IS NULL OR planned_duration_seconds BETWEEN 1 AND 86400);

-- A set with neither reps nor duration would be meaningless and break statistics.
CREATE INDEX workout_sets_kind_idx ON workout_sets (kind);
