# IronNote — architecture

IronNote is a personal strength-training diary: a place to record what was lifted,
not a coach. It stores facts the user enters and derives statistics from them. It
never generates programs, recommends exercises, or interprets anything.

## Shape of the system

```text
browser ──HTTP──► frontend (nginx, static React build)
                       │  /api/* proxied (same origin, no CORS in production)
                       ▼
                  backend (Go, chi) ──pgx──► postgres
```

Three services, deliberately. No message broker, no cache tier, no orchestration:
the workload is a single user and a few thousand rows, so those components would
add failure modes without removing work.

| Service    | Image / runtime      | Responsibility                                   |
|------------|----------------------|--------------------------------------------------|
| `frontend` | node build → nginx   | React SPA, static assets, `/api` reverse proxy    |
| `backend`  | golang build → distroless-ish alpine | REST API, validation, statistics |
| `postgres` | postgres:16-alpine   | durable storage, constraints, indexes             |

### Layering on the backend

```text
cmd/api          wiring, config, shutdown
internal/api     HTTP: routing, decode, validate, error envelope, logging
internal/service business rules: volume math, 1RM, archive decisions, transactions
internal/store   SQL (sqlc-generated + a few hand-written queries), pgx pool
migrations       schema + seed, applied by an embedded migrate runner on boot
```

Access to data is separated from business logic. `store` only knows SQL;
`service` only knows rules and interfaces; `api` only knows HTTP. The authenticated
caller is resolved once per request by middleware into `auth.Actor` and passed
explicitly into the service layer, so no handler can accidentally act without an
owner. See [security.md](security.md) for the authorisation model.

## Data model

Nine tables. Everything historical is protected from cascading deletes of
reference data.

```text
users                    auth_sessions             login_attempts
  id (pk)                  id (pk)                   id (pk)
  username (ci-unique)     user_id FK CASCADE        attempt_key (hashed)
  password_hash (argon2id) token_hash BYTEA           attempted_at
  role admin|user          csrf_token                succeeded
  is_active                expires_at / revoked_at
  must_change_password     ip_address, user_agent
  failed_login_count, locked_until

muscle_groups            exercises                 exercise_muscle_groups
  id (pk)                  id (pk)                   exercise_id  ─┐ FK RESTRICT
  name (unique)            name                      muscle_group_id ─┘ FK CASCADE
  description              description               is_primary
  created_at/updated_at    equipment                 pk(exercise_id, muscle_group_id)
                           is_archived  ◄── soft delete when history exists
                           created_at/updated_at

workout_templates            workout_template_exercises
  id (pk)                      id (pk)
  name, description            template_id  FK CASCADE (a template is not history)
  created_at/updated_at        exercise_id  FK RESTRICT
                               sort_order, planned_sets, planned_reps,
                               planned_weight_kg NUMERIC(7,3) NULL

workout_sessions             workout_session_exercises      workout_sets
  id (pk)                      id (pk)                        id (pk)
  user_id FK CASCADE ◄── owner workout_session_id FK CASCADE  workout_exercise_id FK CASCADE
  template_id FK SET NULL      exercise_id FK RESTRICT        set_number
  name                                                        weight_kg NUMERIC(7,3)
  started_at timestamptz       sort_order                     weight_kg NUMERIC(7,3)
  finished_at timestamptz      notes                          repetitions  smallint
  status in_progress|completed                                is_warmup boolean
  notes                                                       is_completed boolean
```

Decisions that matter:

- **NUMERIC, never FLOAT.** `NUMERIC(7,3)` handles 0.5 kg plates and 1.25 kg jumps
  exactly; floating point would make "60.1 kg × 12" statistics drift.
- **`timestamptz` everywhere**, so the client always receives an instant.
- **Deleting a workout session cascades** to its exercises and sets: that is a
  deliberate act on the user's own record. **Deleting reference data never
  cascades into history**: `workout_session_exercises.exercise_id` and
  `workout_template_exercises.exercise_id` are `ON DELETE RESTRICT`, and the API
  refuses to delete an exercise that has history, offering archive instead.
- **Snapshots.** A completed session keeps its own `name` and its own rows; a
  template is a starting point copied into the session when it begins. Editing or
  deleting a template later cannot alter a finished workout.
- **Set identity.** `set_number` is a display order within an exercise, unique per
  exercise (`UNIQUE (workout_exercise_id, set_number)`); renumbering happens in a
  transaction when a set is deleted.
- **Three kinds of movement.** `exercises.kind` is `weighted` (weight × reps),
  `bodyweight` (reps only) or `duration` (seconds, optional metres). The kind is
  copied to `workout_sets.kind` so history is self-describing, and the database
  enforces which measure each row must carry: a timed set needs a duration, a
  strength set needs repetitions.
- **Weight may be NULL** for bodyweight work; volume sums only completed,
  non-warmup sets that have a weight, exactly as the requirements state. Timed
  sets contribute time and distance, never volume.
- **One in-flight session per user** is allowed by a partial unique index on
  `(user_id) WHERE status = 'in_progress'`, so "resume where you left off" stays
  unambiguous for each person independently.
- **Ownership is a column, not a convention.** `workout_sessions.user_id` and
  `workout_templates.user_id` are `NOT NULL` with a foreign key; the queries that
  read them always filter by the acting user, and there is no unscoped
  "get by id" query to forget the filter on.

## Statistics definitions

These are computed in `internal/service` (pure functions, unit-tested) so the
numbers cannot diverge between endpoints:

- **Volume** of a set = `weight_kg × repetitions`, counted only when
  `is_completed = true`, `is_warmup = false`, `weight_kg IS NOT NULL`.
- **Warmups** are excluded from every headline figure by default; a query flag can
  include them.
- **Actual 1RM** = the heaviest weight ever performed for exactly 1 repetition in a
  completed working set, or *no data* (never a 0).
- **Estimated 1RM** = Epley, `weight × (1 + reps / 30)`, taken per set and reported
  as the best value. It is labelled "estimated" in the UI and is never merged with
  the actual figure.

## REST API

Base path `/api`. Errors use one envelope: `{"error":{"code":"...","message":"..."}}`
with `400` validation, `404` missing, `409` conflict (in use / already in progress),
`422` business rule, `500` unexpected. Health: `GET /healthz` (liveness),
`GET /readyz` (database reachable).

| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/muscle-groups` | list / create |
| GET/PATCH/DELETE | `/muscle-groups/{id}` | read / update / delete (409 if referenced) |
| GET/POST | `/exercises?search=&muscleGroupId=&includeArchived=` | list / create |
| GET/PATCH/DELETE | `/exercises/{id}` | read / update / delete-or-archive |
| POST | `/exercises/{id}/archive`, `/unarchive` | explicit archive toggle |
| GET/POST | `/templates` | list / create (with ordered exercises) |
| GET/PUT/DELETE | `/templates/{id}` | read / replace / delete |
| POST | `/templates/{id}/start` | create a session from the template |
| GET | `/sessions?status=&from=&to=` | history list with summary counters |
| POST | `/sessions` | start empty or from `templateId` |
| GET/PATCH/DELETE | `/sessions/{id}` | read / rename, notes / delete |
| POST | `/sessions/{id}/finish`, `/reopen` | complete / unlock a finished session |
| POST | `/sessions/{id}/exercises` | add exercise |
| PATCH/DELETE | `/sessions/{id}/exercises/{exId}` | reorder / notes, remove |
| POST | `/sessions/{id}/exercises/{exId}/sets` | add set (optionally copied values) |
| PATCH/DELETE | `/sessions/{id}/exercises/{exId}/sets/{setId}` | edit / remove |
| GET | `/stats/summary?from=&to=&includeWarmup=` | totals, monthly sessions |
| GET | `/stats/exercises?from=&to=` | per-exercise bests |
| GET | `/stats/exercises/{id}?from=&to=` | series: volume, best set, est. 1RM |

Set updates are per-field `PATCH`es so the mobile UI sends one small request when a
field loses focus; nothing is recalculated client-side.

## Screens and navigation

Mobile-first, bottom navigation (safe-area aware): **Тренировки · История ·
Упражнения · Статистика · Настройки**. At `lg` and above the same five become a
fixed sidebar. The active-workout screen intentionally hides the bottom bar on
mobile: it is a focused, one-handed task with its own sticky action bar.

1. **Workouts** — resume banner if a session is in flight, "start empty", template list.
2. **Active workout** — inline numeric inputs per set, one tap to add a set copied
   from the previous one, complete toggle, exercise reorder, finish.
3. **Exercises** — search, muscle-group filter, card detail, create/edit/archive.
4. **History** — reverse-chronological list with date filter, per-session detail,
   edit and delete.
5. **Statistics** — period selector, headline totals, monthly bars, per-exercise
   charts (Recharts).
6. **Settings** — muscle-group management, exercise management, app info.

## Assumptions made (simplest reasonable choice)

1. **Single user, no auth** — stated by the requirements; no user table in the MVP.
2. **One in-progress workout at a time.** Starting a second one returns `409` with
   the id of the current session, rather than silently discarding work.
3. **Times are stored in UTC (`timestamptz`) and rendered in the browser locale**;
   the API accepts RFC 3339 instants and exposes date-only filters as `from`/`to`
   days interpreted in the client's timezone.
4. **Deleting a session is allowed** (it is the user's own record) while deleting a
   referenced exercise is not — archive is offered instead.
5. **`template_id` on a session is informational.** Sessions copy exercises and
   planned sets at start; later template edits deliberately do not propagate.
6. **Planned weight in a template is a default**, pre-filled into the first set of
   the started session, and always editable.
7. **Rest times, body measurements, 1RM percentages and photos are out of scope.**
8. **The `/api` prefix is served from the same origin** through nginx, so the
   frontend needs no build-time API URL and no CORS in production; in dev, Vite
   proxies to the backend.

## Repository layout

```text
IronNote/
├── backend/            Go API (cmd, internal, migrations embed, sqlc queries)
├── frontend/           Vite React SPA
├── docker-compose.yml
├── .env.example
├── app.yaml            workspace manifest
└── README.md           how to run, test, verify
```
