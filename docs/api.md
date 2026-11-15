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
| `internal_error` | 500 | Unexpected failure; details are logged, never returned |

Health: `GET /healthz` (process alive) and `GET /readyz` (database reachable).

## Muscle groups

```
GET    /muscle-groups                     → { items: MuscleGroup[] }
POST   /muscle-groups                     { name, description? } → 201 MuscleGroup
GET    /muscle-groups/{id}
PATCH  /muscle-groups/{id}                { name, description? }
DELETE /muscle-groups/{id}                → 204 | 409 when used by an exercise
```

## Exercises

```
GET    /exercises?search=&muscleGroupId=&includeArchived=
POST   /exercises    { name, description?, equipment?, muscleGroups: [{muscleGroupId, isPrimary}] }
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
PATCH  /sessions/{id}       { name?, notes?, startedAt? }
POST   /sessions/{id}/finish             stamps finished_at, status=completed
POST   /sessions/{id}/reopen             back to in_progress (respects one-at-a-time rule)
DELETE /sessions/{id}                    → 204

POST   /sessions/{id}/exercises                     { exerciseId, notes? }
PATCH  /sessions/{id}/exercises/{exId}              { direction: -1 | 1 } → reorders
PATCH  /sessions/{id}/exercises/{exId}/notes        { notes }
DELETE /sessions/{id}/exercises/{exId}              → 204

POST   /sessions/{id}/exercises/{exId}/sets         { weightKg?, repetitions, isWarmup?, isCompleted?, copyPrevious? }
PATCH  /sessions/{id}/exercises/{exId}/sets/{setId} partial: only the fields sent change
DELETE /sessions/{id}/exercises/{exId}/sets/{setId} → 204, remaining sets renumbered 1..n
```

`/sessions/active` returns `{"session": null}` rather than 404 so a client can poll
it without treating "no workout in progress" as an error.

## Statistics

```
GET /stats/summary?from=&to=              totals, monthly session counts
GET /stats/exercises?from=&to=            per-exercise bests for the period
GET /stats/exercises/{id}?from=&to=       series, records, recent sets
```

Rules, applied in one place (`internal/service`):

- **Volume** = `weight_kg × repetitions`, only for completed, non-warmup sets with
  a recorded weight.
- **Warmups** are excluded from headline numbers by default.
- **Actual 1RM** — the heaviest completed **single** (repetitions = 1), or `null`.
- **Estimated 1RM** — Epley, `weight × (1 + reps / 30)`, best set of the period.
  Reported separately from the actual single; never merged.
- Dates are half-open `[from, to)`; a bare `YYYY-MM-DD` in `to` includes that day.
