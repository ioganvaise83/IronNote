# IronNote REST API

Base URL: `/api`. All bodies are JSON, all timestamps are RFC 3339 instants in
UTC. Errors share one envelope:

```json
{ "error": { "code": "validation_error", "message": "Проверьте поля формы",
             "details": { "name": "Название обязательно" } } }
```

| Code | Status | Meaning |
|---|---|---|
| `validation_error` | 400 | Malformed body, bad id, or a failed field rule |
| `not_found` | 404 | No such record |
| `conflict` | 409 | Duplicate name, exercise in use, workout already in progress |
| `rule_violation` | 422 | A business rule refused the change (e.g. finish before start) |
| `unauthorized` | 401 | No session, or bad credentials |
| `forbidden` | 403 | Role or CSRF failure, or a pending password change |
| `too_many_requests` | 429 | Rate limited (login or general) |
| `internal_error` | 500 | Unexpected failure; details are logged, never returned |

Health: `GET /healthz` (process alive) and `GET /readyz` (database reachable).

## Authentication

The API is cookie-authenticated. `POST /auth/login` sets `ironnote_session`
(`HttpOnly`, `SameSite=Lax`, `Secure` when TLS is configured) and returns a CSRF
token that must be sent as `X-CSRF-Token` on every `POST`/`PATCH`/`PUT`/`DELETE`.
Requests authenticated with `Authorization: Bearer <token>` do not need the CSRF
header — a cross-site form cannot set a header.

```
POST /auth/login        { username, password } → { user, csrfToken } + Set-Cookie
POST /auth/logout       → 204, revokes the session and clears the cookie
GET  /auth/me           → { user, csrfToken }   (used to restore state on reload)
POST /auth/password     { currentPassword, newPassword }
```

Errors never reveal whether an account exists: an unknown user and a wrong
password both return `401` with the same message. A locked account is reported only
after the password matched, and rate limiting returns `429`.

### Roles

| Role | Can |
|---|---|
| `user` | Read the catalogue; record, view and analyse **own** workouts and templates |
| `admin` | Everything a user can do, plus catalogue writes and account management |

An admin is **not** a superuser over training data: an admin cannot read another
user's workout. Ownership is enforced in SQL, not only in the handler.

## Admin-only endpoints

```
GET    /users                                → { items: User[] }
POST   /users           { username, displayName, password, role }
PATCH  /users/{id}      { displayName?, role?, isActive? }
DELETE /users/{id}                            removes the account and its training data
POST   /users/{id}/password  { newPassword }  forces a change on next login,
                                              revokes existing sessions
```

The server refuses any change that would leave no active administrator, that
disables the caller's own account, or that deletes the last admin.

Catalogue writes (`POST/PATCH/DELETE /muscle-groups`, `/exercises`,
`/exercises/{id}/archive|unarchive`) are admin-only; catalogue reads are open to
every signed-in user.

## Muscle groups

```
GET    /muscle-groups                     → { items: MuscleGroup[] }
POST   /muscle-groups                     { name, description? } → 201 MuscleGroup
GET    /muscle-groups/{id}
PATCH  /muscle-groups/{id}                { name, description? }
DELETE /muscle-groups/{id}                → 204 | 409 when used by an exercise
```

## Exercise kinds

Every exercise has a `kind` that decides which measures its sets carry:

| kind | Measures | Examples | Volume |
|---|---|---|---|
| `weighted` | weight + repetitions | жим штанги, приседания | counts |
| `bodyweight` | repetitions only | подтягивания, отжимания | none (no external load) |
| `duration` | seconds (+ optional metres) | бег, плавание, эллипсоид | none |

The kind is copied onto each recorded set, so history keeps its meaning if the
catalogue entry is edited later. `duration` exercises have no weight, no
repetitions and no 1RM; their statistics use time and distance instead.

## Exercises

```
GET    /exercises?search=&muscleGroupId=&includeArchived=
POST   /exercises    { name, description?, equipment?, kind?, muscleGroups: [{muscleGroupId, isPrimary}] }
GET    /exercises/{id}
PATCH  /exercises/{id}                    same body as POST
DELETE /exercises/{id}                    → 204 | 409 when history exists (archive instead)
POST   /exercises/{id}/archive
POST   /exercises/{id}/unarchive
```

## Templates

```
GET    /templates                         → { items: TemplateSummary[] }
POST   /templates    { name, description?, exercises: [{exerciseId, plannedSets?, plannedReps?, plannedWeightKg?}] }
GET    /templates/{id}                    → Template with ordered exercises
PUT    /templates/{id}                    replaces name/description/exercises atomically
DELETE /templates/{id}                    sessions keep their copied rows (template_id → NULL)
POST   /templates/{id}/start              → 201 Session (copies planned sets, uncompleted)
```

## Sessions

```
GET    /sessions?status=&from=&to=        → { items: SessionSummary[] }
POST   /sessions            { name?, templateId?, notes? } → 201 Session
GET    /sessions/active                  → { session: Session | null }
GET    /sessions/{id}                    → Session with exercises, sets and previousSets
PATCH  /sessions/{id}       { name?, notes?, startedAt?, durationMinutes? }
POST   /sessions/{id}/finish             stamps finished_at, status=completed
                                         → 422 when nothing was performed (see below)
POST   /sessions/{id}/reopen             back to in_progress (respects one-at-a-time rule)
DELETE /sessions/{id}                    → 204

POST   /sessions/{id}/exercises                     { exerciseId, notes? }
PATCH  /sessions/{id}/exercises/{exId}              { direction: -1 | 1 } → reorders
PATCH  /sessions/{id}/exercises/{exId}/notes        { notes }
DELETE /sessions/{id}/exercises/{exId}              → 204

POST   /sessions/{id}/exercises/{exId}/sets
       weighted:  { weightKg?, repetitions, isWarmup?, isCompleted?, copyPrevious? }
       bodyweight:{ repetitions, ... }            (a weight sent anyway is dropped)
       duration:  { durationSeconds, distanceMeters?, ... }
PATCH  /sessions/{id}/exercises/{exId}/sets/{setId} partial: only the fields sent change
DELETE /sessions/{id}/exercises/{exId}/sets/{setId} → 204, remaining sets renumbered 1..n
```

`/sessions/active` returns `{"session": null}` rather than 404 so a client can poll
it without treating "no workout in progress" as an error.

### Finishing and discarding

A workout is recorded only if it contains at least one exercise with at least one
completed, non-warmup set carrying a real measure (repetitions or seconds).
Otherwise `POST /sessions/{id}/finish` returns **422** with
`{"error":{"code":"rule_violation","message":"Тренировка не сохранена: ..."}}`
and the session row is deleted, so nothing empty reaches history or statistics.

### Editing a finished workout

`PATCH /sessions/{id}` accepts `startedAt` (an RFC 3339 instant) and
`durationMinutes`. The duration is applied as `finished_at = started_at + duration`,
so there is still exactly one source of truth. Both are validated: a start in the
future, a duration ≤ 0, more than 24 hours, or an end in the future are rejected
with `400`.

`DELETE /sessions/{id}` removes the workout, its exercises and its sets. It never
touches the exercise catalogue.

## Statistics

```
GET /stats/summary?from=&to=              totals, monthly session counts
GET /stats/exercises?from=&to=            per-exercise bests for the period
GET /stats/exercises/{id}?from=&to=       series, records, recent sets
```

Rules, applied in one place (`internal/service`):

- **Volume** = `weight_kg × repetitions`, only for completed, non-warmup sets with
  a recorded weight. Timed and bodyweight sets have no volume by definition.
- **Cardio** aggregates `durationSeconds` and `distanceMeters` from completed,
  non-warmup timed sets; these are reported separately from volume and never mixed
  into it.
- **Warmups** are excluded from headline numbers by default.
- **Actual 1RM** — the heaviest completed **single** (repetitions = 1), or `null`.
- **Estimated 1RM** — Epley, `weight × (1 + reps / 30)`, best set of the period.
  Reported separately from the actual single; never merged.
- Dates are half-open `[from, to)`; a bare `YYYY-MM-DD` in `to` includes that day.
