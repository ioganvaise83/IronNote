-- IronNote initial schema.
-- Timestamps are timestamptz: the database stores instants, the browser renders
-- them in the user's locale. Weights are NUMERIC so that 1.25 kg increments stay
-- exact through every aggregate (FLOAT would drift in sums).

CREATE TABLE muscle_groups (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT muscle_groups_name_not_blank CHECK (btrim(name) <> '')
);

-- Case-insensitive uniqueness: "Грудь" and "грудь" are the same reference entry.
CREATE UNIQUE INDEX muscle_groups_name_key ON muscle_groups (lower(name));

CREATE TABLE exercises (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    equipment   TEXT,
    -- Archive instead of delete once an exercise appears in history; history must
    -- never lose the exercise it refers to.
    is_archived BOOLEAN     NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT exercises_name_not_blank CHECK (btrim(name) <> '')
);

CREATE UNIQUE INDEX exercises_name_key ON exercises (lower(name));
CREATE INDEX exercises_archived_idx ON exercises (is_archived);

CREATE TABLE exercise_muscle_groups (
    exercise_id     BIGINT  NOT NULL REFERENCES exercises (id) ON DELETE CASCADE,
    muscle_group_id BIGINT  NOT NULL REFERENCES muscle_groups (id) ON DELETE CASCADE,
    is_primary      BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (exercise_id, muscle_group_id)
);

-- Filtering the exercise catalogue by muscle group.
CREATE INDEX exercise_muscle_groups_group_idx
    ON exercise_muscle_groups (muscle_group_id, exercise_id);
-- At most one primary muscle group per exercise.
CREATE UNIQUE INDEX exercise_muscle_groups_primary_key
    ON exercise_muscle_groups (exercise_id) WHERE is_primary;

CREATE TABLE workout_templates (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT workout_templates_name_not_blank CHECK (btrim(name) <> '')
);

CREATE TABLE workout_template_exercises (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    template_id      BIGINT  NOT NULL REFERENCES workout_templates (id) ON DELETE CASCADE,
    exercise_id      BIGINT  NOT NULL REFERENCES exercises (id) ON DELETE RESTRICT,
    sort_order       INTEGER NOT NULL,
    planned_sets     INTEGER,
    planned_reps     INTEGER,
    planned_weight_kg NUMERIC(7, 3),
    CONSTRAINT workout_template_exercises_sort_order_key UNIQUE (template_id, sort_order),
    CONSTRAINT workout_template_exercises_planned_sets_check
        CHECK (planned_sets IS NULL OR planned_sets BETWEEN 1 AND 50),
    CONSTRAINT workout_template_exercises_planned_reps_check
        CHECK (planned_reps IS NULL OR planned_reps BETWEEN 1 AND 1000),
    CONSTRAINT workout_template_exercises_weight_check
        CHECK (planned_weight_kg IS NULL OR planned_weight_kg BETWEEN 0 AND 2000)
);

CREATE INDEX workout_template_exercises_template_idx
    ON workout_template_exercises (template_id, sort_order);
CREATE INDEX workout_template_exercises_exercise_idx
    ON workout_template_exercises (exercise_id);

CREATE TABLE workout_sessions (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    template_id BIGINT REFERENCES workout_templates (id) ON DELETE SET NULL,
    name        TEXT        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    status      TEXT        NOT NULL DEFAULT 'in_progress',
    notes       TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT workout_sessions_status_check CHECK (status IN ('in_progress', 'completed')),
    CONSTRAINT workout_sessions_finished_check
        CHECK (status <> 'completed' OR finished_at IS NOT NULL),
    CONSTRAINT workout_sessions_time_check
        CHECK (finished_at IS NULL OR finished_at >= started_at),
    CONSTRAINT workout_sessions_name_not_blank CHECK (btrim(name) <> '')
);

-- History is read newest-first.
CREATE INDEX workout_sessions_started_idx ON workout_sessions (started_at DESC);
CREATE INDEX workout_sessions_status_idx ON workout_sessions (status, started_at DESC);
-- One workout in flight at a time: makes "resume" unambiguous and prevents two
-- half-finished logs.
CREATE UNIQUE INDEX workout_sessions_single_active_key
    ON workout_sessions (status) WHERE status = 'in_progress';
CREATE INDEX workout_sessions_template_idx ON workout_sessions (template_id);

CREATE TABLE workout_session_exercises (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workout_session_id BIGINT  NOT NULL REFERENCES workout_sessions (id) ON DELETE CASCADE,
    exercise_id        BIGINT  NOT NULL REFERENCES exercises (id) ON DELETE RESTRICT,
    sort_order         INTEGER NOT NULL,
    notes              TEXT    NOT NULL DEFAULT '',
    CONSTRAINT workout_session_exercises_sort_order_key
        UNIQUE (workout_session_id, sort_order)
);

CREATE INDEX workout_session_exercises_session_idx
    ON workout_session_exercises (workout_session_id, sort_order);
CREATE INDEX workout_session_exercises_exercise_idx
    ON workout_session_exercises (exercise_id);

CREATE TABLE workout_sets (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workout_exercise_id BIGINT      NOT NULL REFERENCES workout_session_exercises (id) ON DELETE CASCADE,
    set_number          INTEGER     NOT NULL,
    -- NULL means bodyweight / not recorded; it is excluded from volume rather
    -- than treated as zero.
    weight_kg           NUMERIC(7, 3),
    repetitions         SMALLINT    NOT NULL,
    is_warmup           BOOLEAN     NOT NULL DEFAULT false,
    is_completed        BOOLEAN     NOT NULL DEFAULT true,
    CONSTRAINT workout_sets_number_key UNIQUE (workout_exercise_id, set_number),
    CONSTRAINT workout_sets_number_check CHECK (set_number >= 1),
    CONSTRAINT workout_sets_reps_check CHECK (repetitions BETWEEN 1 AND 1000),
    CONSTRAINT workout_sets_weight_check
        CHECK (weight_kg IS NULL OR weight_kg BETWEEN 0 AND 2000)
);

CREATE INDEX workout_sets_exercise_idx ON workout_sets (workout_exercise_id, set_number);

-- Keep updated_at honest without relying on every query remembering it.
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER muscle_groups_set_updated_at BEFORE UPDATE ON muscle_groups
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER exercises_set_updated_at BEFORE UPDATE ON exercises
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER workout_templates_set_updated_at BEFORE UPDATE ON workout_templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER workout_sessions_set_updated_at BEFORE UPDATE ON workout_sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
