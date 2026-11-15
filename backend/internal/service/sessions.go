package service

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/auth"
	"github.com/ioganvaise83/ironnote/internal/store"
)

// StartSessionInput begins a workout either empty or from a template.
type StartSessionInput struct {
	Name       string `json:"name"`
	TemplateID *int64 `json:"templateId"`
	Notes      string `json:"notes"`
}

// StartSession enforces one workout in flight. Starting from a template copies
// its exercises and planned volume into the session: the template is a starting
// gun, not a live link.
func (s *Service) StartSession(ctx context.Context, actor auth.Actor, in StartSessionInput) (Session, error) {
	if _, err := s.q.GetActiveSession(ctx, actor.UserID); err == nil {
		return Session{}, apperror.Conflict("Уже есть незавершённая тренировка — продолжите или завершите её")
	} else if !isNoRows(err) {
		return Session{}, apperror.Internal(err)
	}

	var name string
	var exercises []TemplateExercise
	if in.TemplateID != nil {
		tpl, err := s.GetTemplate(ctx, actor, *in.TemplateID)
		if err != nil {
			return Session{}, err
		}
		name = strings.TrimSpace(in.Name)
		if name == "" {
			name = tpl.Name
		}
		exercises = tpl.Exercises
	} else {
		name = strings.TrimSpace(in.Name)
	}
	if name == "" {
		name = "Тренировка"
	}

	var id int64
	err := s.withTx(ctx, func(q *store.Queries) error {
		created, err := q.CreateSession(ctx, store.CreateSessionParams{
			UserID:     actor.UserID,
			TemplateID: in.TemplateID,
			Name:       name,
			StartedAt:  pgTimestamptz(time.Now()),
			Notes:      in.Notes,
		})
		if err != nil {
			return translate(err, "Тренировка")
		}
		id = created.ID

		for i, ex := range exercises {
			added, err := q.AddSessionExercise(ctx, store.AddSessionExerciseParams{
				WorkoutSessionID: id,
				ExerciseID:       ex.ExerciseID,
				SortOrder:        int32(i + 1),
				UserID:           actor.UserID,
			})
			if err != nil {
				return translate(err, "Упражнение")
			}
			// A timed movement has no planned sets/reps: it is one planned effort
			// with a duration, recorded as a single set when the user starts.
			if ex.PlannedDurationSeconds != nil {
				if _, err := q.AddSet(ctx, store.AddSetParams{
					WorkoutExerciseID: added.ID,
					UserID:            actor.UserID,
					SetNumber:         1,
					Kind:              store.ExerciseKind(KindDuration),
					DurationSeconds:   ex.PlannedDurationSeconds,
					IsCompleted:       false,
				}); err != nil {
					return translate(err, "Подход")
				}
				continue
			}
			if ex.PlannedSets == nil {
				continue
			}
			count := *ex.PlannedSets
			if count > 20 {
				count = 20 // a template typo should not create 50 empty rows
			}
			for n := int32(1); n <= count; n++ {
				plannedReps := int16(derefInt32(ex.PlannedReps, 10))
				plannedKind := KindWeighted
				var plannedWeight *float64
				if ex.PlannedWeightKg.IsSet() {
					plannedWeight = ex.PlannedWeightKg.Ptr()
				} else {
					plannedKind = KindBodyweight
				}
				if _, err := q.AddSet(ctx, store.AddSetParams{
					WorkoutExerciseID: added.ID,
					UserID:            actor.UserID,
					SetNumber:         n,
					Kind:              store.ExerciseKind(plannedKind),
					WeightKg:          store.PtrToNumeric(plannedWeight),
					Repetitions:       &plannedReps,
					IsWarmup:          false,
					// Planned rows start uncompleted: they are a target, not a fact.
					IsCompleted: false,
				}); err != nil {
					return translate(err, "Подход")
				}
			}
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return s.GetSession(ctx, actor, id)
}

func (s *Service) GetActiveSession(ctx context.Context, actor auth.Actor) (*Session, error) {
	row, err := s.q.GetActiveSession(ctx, actor.UserID)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, apperror.Internal(err)
	}
	session, err := s.loadSession(ctx, actor, row.ID, false)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

type SessionFilter struct {
	Status string
	From   *time.Time
	To     *time.Time
	OnlyID *int64
}

// ListSessions returns full sessions with derived summary counters. The service
// computes every number, so the history list and the statistics page cannot
// disagree about volume.
func (s *Service) ListSessions(ctx context.Context, actor auth.Actor, f SessionFilter) ([]SessionSummary, error) {
	rows, err := s.q.ListSessionDetails(ctx, store.ListSessionDetailsParams{
		UserID:      actor.UserID,
		Status:      optionalString(f.Status),
		StartedFrom: pgTimestamptzPtr(f.From),
		StartedTo:   pgTimestamptzPtr(f.To),
		SessionID:   f.OnlyID,
	})
	if err != nil {
		return nil, apperror.Internal(err)
	}
	sessions := groupSessionDetails(rows)
	out := make([]SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, Summarize(session))
	}
	return out, nil
}

func (s *Service) GetSession(ctx context.Context, actor auth.Actor, id int64) (Session, error) {
	return s.loadSession(ctx, actor, id, true)
}

func (s *Service) loadSession(ctx context.Context, actor auth.Actor, id int64, withPrevious bool) (Session, error) {
	rows, err := s.q.ListSessionDetails(ctx, store.ListSessionDetailsParams{
		UserID: actor.UserID, SessionID: &id,
	})
	if err != nil {
		return Session{}, apperror.Internal(err)
	}
	sessions := groupSessionDetails(rows)
	if len(sessions) == 0 {
		return Session{}, apperror.NotFound("Тренировка")
	}
	session := sessions[0]

	if withPrevious {
		for i := range session.Exercises {
			prev, prevAt, err := s.previousSets(ctx, actor, session.Exercises[i].ExerciseID, session.ID)
			if err != nil {
				return Session{}, err
			}
			session.Exercises[i].PreviousSets = prev
			session.Exercises[i].PreviousAt = prevAt
		}
	}
	return session, nil
}

func (s *Service) previousSets(ctx context.Context, actor auth.Actor, exerciseID, excludeSessionID int64) ([]Set, *time.Time, error) {
	rows, err := s.q.PreviousExerciseSets(ctx, store.PreviousExerciseSetsParams{
		ExerciseID:       exerciseID,
		UserID:           actor.UserID,
		ExcludeSessionID: excludeSessionID,
	})
	if err != nil {
		return nil, nil, apperror.Internal(err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	out := make([]Set, 0, len(rows))
	var at time.Time
	for _, r := range rows {
		at = r.StartedAt.Time
		out = append(out, Set{
			SetNumber:   r.SetNumber,
			WeightKg:    numericToWeight(r.WeightKg),
			Repetitions: r.Repetitions,
			IsWarmup:    r.IsWarmup,
			IsCompleted: true,
		})
	}
	return out, &at, nil
}

// UpdateSessionInput edits the header of a session.
//
// StartedAt and DurationMinutes let a finished workout be corrected after the
// fact: people forget to press "finish" and remember the real duration later. The
// duration is applied as finished_at = started_at + duration, keeping the single
// source of truth (two timestamps) rather than adding a third field.
type UpdateSessionInput struct {
	Name      *string    `json:"name"`
	Notes     *string    `json:"notes"`
	StartedAt *time.Time `json:"startedAt"`
	// DurationMinutes replaces the recorded length; 0 is rejected because a
	// zero-length workout is not a workout.
	DurationMinutes *float64 `json:"durationMinutes"`
}

// UpdateSession keeps finished sessions editable, as required: notes and the
// time can be corrected after the fact.
func (s *Service) UpdateSession(ctx context.Context, actor auth.Actor, id int64, in UpdateSessionInput) (Session, error) {
	current, err := s.q.GetSession(ctx, store.GetSessionParams{ID: id, UserID: actor.UserID})
	if err != nil {
		return Session{}, translate(err, "Тренировка")
	}
	name := current.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" {
			return Session{}, apperror.Validation("Проверьте поля формы", map[string]string{"name": "Название обязательно"})
		}
	}
	notes := current.Notes
	if in.Notes != nil {
		notes = *in.Notes
	}
	startedAt := current.StartedAt.Time
	if in.StartedAt != nil {
		startedAt = *in.StartedAt
	}
	if startedAt.After(time.Now().Add(time.Minute)) {
		return Session{}, apperror.Validation("Начало тренировки не может быть в будущем",
			map[string]string{"startedAt": "Укажите прошедшее время"})
	}

	finishedAt := current.FinishedAt
	if in.DurationMinutes != nil {
		if *in.DurationMinutes <= 0 {
			return Session{}, apperror.Validation("Продолжительность должна быть больше нуля",
				map[string]string{"durationMinutes": "Укажите продолжительность"})
		}
		if *in.DurationMinutes > 24*60 {
			return Session{}, apperror.Validation("Продолжительность больше 24 часов",
				map[string]string{"durationMinutes": "Слишком долго"})
		}
		end := startedAt.Add(time.Duration(*in.DurationMinutes * float64(time.Minute)))
		if end.After(time.Now().Add(time.Minute)) {
			return Session{}, apperror.Validation("Окончание тренировки не может быть в будущем",
				map[string]string{"durationMinutes": "Укажите меньшую продолжительность"})
		}
		finishedAt = pgtype.Timestamptz{Time: end, Valid: true}
	}

	if finishedAt.Valid && startedAt.After(finishedAt.Time) {
		return Session{}, apperror.Rule("Начало не может быть позже завершения")
	}

	if _, err := s.q.UpdateSession(ctx, store.UpdateSessionParams{
		ID: id, Name: name, Notes: notes, StartedAt: pgTimestamptz(startedAt),
		UserID: actor.UserID,
	}); err != nil {
		return Session{}, translate(err, "Тренировка")
	}

	// Re-stamp the end time when the length was edited, through the same statement
	// that finishing uses, so the invariant "completed implies finished_at" holds.
	if finishedAt.Valid && (!current.FinishedAt.Valid || !current.FinishedAt.Time.Equal(finishedAt.Time)) {
		if _, err := s.q.FinishSession(ctx, store.FinishSessionParams{
			ID: id, FinishedAt: finishedAt, UserID: actor.UserID,
		}); err != nil {
			return Session{}, translate(err, "Тренировка")
		}
	}
	return s.GetSession(ctx, actor, id)
}

// FinishSession stamps the end time.
//
// A workout worth recording must contain at least one exercise with at least one
// completed, non-warmup set. Otherwise there is nothing to remember, so the
// session is discarded rather than stored as an empty row that would pollute
// history and statistics. The caller is told explicitly (ErrNothingToSave).
func (s *Service) FinishSession(ctx context.Context, actor auth.Actor, id int64) (Session, error) {
	current, err := s.q.GetSession(ctx, store.GetSessionParams{ID: id, UserID: actor.UserID})
	if err != nil {
		return Session{}, translate(err, "Тренировка")
	}
	if current.Status == StatusCompleted {
		return s.GetSession(ctx, actor, id)
	}

	loaded, err := s.loadSession(ctx, actor, id, false)
	if err != nil {
		return Session{}, err
	}
	if !hasRecordableWork(loaded) {
		// Nothing was actually performed: do not keep an empty session.
		if _, err := s.q.DeleteSession(ctx, store.DeleteSessionParams{ID: id, UserID: actor.UserID}); err != nil {
			return Session{}, apperror.Internal(err)
		}
		return Session{}, ErrNothingToSave
	}

	now := time.Now()
	if now.Before(current.StartedAt.Time) {
		now = current.StartedAt.Time
	}
	if _, err := s.q.FinishSession(ctx, store.FinishSessionParams{
		ID: id, FinishedAt: pgTimestamptz(now), UserID: actor.UserID,
	}); err != nil {
		return Session{}, translate(err, "Тренировка")
	}
	return s.GetSession(ctx, actor, id)
}

// hasRecordableWork reports whether a session contains anything worth keeping.
//
// An exercise counts when it has at least one completed set that is not a warmup,
// and that set carries a real measure (reps for strength, time for cardio). A
// session with exercises but no completed sets is as empty as one with none.
func hasRecordableWork(session Session) bool {
	for _, exercise := range session.Exercises {
		for _, set := range exercise.Sets {
			if !set.CountsAsWork() {
				continue
			}
			if set.Kind == KindDuration {
				if set.DurationSeconds != nil && *set.DurationSeconds > 0 {
					return true
				}
				continue
			}
			if set.Repetitions != nil && *set.Repetitions > 0 {
				return true
			}
		}
	}
	return false
}

// ReopenSession moves a finished workout back to in_progress so it can be edited
// in the active screen. It respects the single-active-session rule.
func (s *Service) ReopenSession(ctx context.Context, actor auth.Actor, id int64) (Session, error) {
	if _, err := s.q.GetActiveSession(ctx, actor.UserID); err == nil {
		return Session{}, apperror.Conflict("Сначала завершите текущую незавершённую тренировку")
	} else if !isNoRows(err) {
		return Session{}, apperror.Internal(err)
	}
	if _, err := s.q.ReopenSession(ctx, store.ReopenSessionParams{ID: id, UserID: actor.UserID}); err != nil {
		return Session{}, translate(err, "Тренировка")
	}
	return s.GetSession(ctx, actor, id)
}

func (s *Service) DeleteSession(ctx context.Context, actor auth.Actor, id int64) error {
	affected, err := s.q.DeleteSession(ctx, store.DeleteSessionParams{ID: id, UserID: actor.UserID})
	if err != nil {
		return translate(err, "Тренировка")
	}
	if affected == 0 {
		return apperror.NotFound("Тренировка")
	}
	return nil
}

// AddSessionExercise appends a movement to a workout.
func (s *Service) AddSessionExercise(ctx context.Context, actor auth.Actor, sessionID, exerciseID int64, notes string) (SessionExercise, error) {
	order, err := s.q.NextSessionExerciseSortOrder(ctx, store.NextSessionExerciseSortOrderParams{
		WorkoutSessionID: sessionID, UserID: actor.UserID,
	})
	if err != nil {
		return SessionExercise{}, apperror.Internal(err)
	}
	added, err := s.q.AddSessionExercise(ctx, store.AddSessionExerciseParams{
		WorkoutSessionID: sessionID,
		ExerciseID:       exerciseID,
		SortOrder:        order,
		Notes:            notes,
		UserID:           actor.UserID,
	})
	if err != nil {
		return SessionExercise{}, translate(err, "Упражнение")
	}
	ex, err := s.q.GetExercise(ctx, exerciseID)
	if err != nil {
		return SessionExercise{}, translate(err, "Упражнение")
	}
	return SessionExercise{
		ID: added.ID, ExerciseID: exerciseID, ExerciseName: ex.Name,
		Equipment: derefString(ex.Equipment), Kind: ParseKind(ex.Kind),
		SortOrder: added.SortOrder, Notes: added.Notes,
		Sets: []Set{},
	}, nil
}

// MoveSessionExercise renumbers siblings inside a transaction so sort_order
// stays unique at every step.
func (s *Service) MoveSessionExercise(ctx context.Context, actor auth.Actor, sessionID, sessionExerciseID int64, direction int) error {
	if direction != -1 && direction != 1 {
		return apperror.Validation("Направление должно быть -1 или 1", nil)
	}
	return s.withTx(ctx, func(q *store.Queries) error {
		rows, err := q.ListSessionExercises(ctx, store.ListSessionExercisesParams{
			WorkoutSessionID: sessionID, UserID: actor.UserID,
		})
		if err != nil {
			return apperror.Internal(err)
		}
		index := -1
		for i, r := range rows {
			if r.ID == sessionExerciseID {
				index = i
				break
			}
		}
		if index < 0 {
			return apperror.NotFound("Упражнение в тренировке")
		}
		target := index + direction
		if target < 0 || target >= len(rows) {
			return nil // already at the edge: not an error
		}
		// Two-phase renumber: move everything out of the way, then apply.
		for i, r := range rows {
			if err := q.SetSessionExerciseSortOrder(ctx, store.SetSessionExerciseSortOrderParams{
				ID: r.ID, SortOrder: int32(i + 1 + len(rows)), UserID: actor.UserID,
			}); err != nil {
				return apperror.Internal(err)
			}
		}
		rows[index], rows[target] = rows[target], rows[index]
		for i, r := range rows {
			if err := q.SetSessionExerciseSortOrder(ctx, store.SetSessionExerciseSortOrderParams{
				ID: r.ID, SortOrder: int32(i + 1), UserID: actor.UserID,
			}); err != nil {
				return apperror.Internal(err)
			}
		}
		return nil
	})
}

func (s *Service) UpdateSessionExerciseNotes(ctx context.Context, actor auth.Actor, sessionID, sessionExerciseID int64, notes string) error {
	if _, err := s.q.UpdateSessionExercise(ctx, store.UpdateSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID, Notes: notes,
		SortOrder: 0, UserID: actor.UserID,
	}); err != nil {
		return translate(err, "Упражнение в тренировке")
	}
	return nil
}

func (s *Service) DeleteSessionExercise(ctx context.Context, actor auth.Actor, sessionID, sessionExerciseID int64) error {
	affected, err := s.q.DeleteSessionExercise(ctx, store.DeleteSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID, UserID: actor.UserID,
	})
	if err != nil {
		return translate(err, "Упражнение в тренировке")
	}
	if affected == 0 {
		return apperror.NotFound("Упражнение в тренировке")
	}
	return nil
}

// SetInput is a create/update payload for a set. Nil fields mean "not provided"
// on update, so a PATCH from a single mobile field changes only that field.
type SetInput struct {
	WeightKg *float64 `json:"weightKg"`
	// Repetitions and DurationSeconds are alternatives; which one is required
	// depends on the exercise kind (see validateSetForKind).
	Repetitions     *int32   `json:"repetitions"`
	DurationSeconds *int32   `json:"durationSeconds"`
	DistanceMeters  *float64 `json:"distanceMeters"`
	IsWarmup        *bool    `json:"isWarmup"`
	IsCompleted     *bool    `json:"isCompleted"`
	// CopyPrevious repeats the previous set's values when creating.
	CopyPrevious bool `json:"copyPrevious"`
}

// validateSetForKind enforces which measures a set must carry.
//
// The database has equivalent CHECK constraints; validating here produces a clear
// message naming the field instead of a 500 from a constraint violation.
func validateSetForKind(in SetInput, kind ExerciseKind, creating bool) error {
	details := map[string]string{}

	if in.WeightKg != nil && (*in.WeightKg < 0 || *in.WeightKg > 2000) {
		details["weightKg"] = "Вес: от 0 до 2000 кг"
	}
	if in.Repetitions != nil && (*in.Repetitions < 1 || *in.Repetitions > 1000) {
		details["repetitions"] = "Повторения: от 1 до 1000"
	}
	if in.DurationSeconds != nil && (*in.DurationSeconds < 1 || *in.DurationSeconds > 86400) {
		details["durationSeconds"] = "Длительность: от 1 секунды до 24 часов"
	}
	if in.DistanceMeters != nil && (*in.DistanceMeters < 0 || *in.DistanceMeters > 1_000_000) {
		details["distanceMeters"] = "Дистанция: от 0 до 1 000 000 м"
	}

	// A copied set inherits its measure from the previous one, so the requirement is
	// satisfied by CopyPrevious even without an explicit value.
	switch kind {
	case KindDuration:
		if creating && in.DurationSeconds == nil && !in.CopyPrevious {
			details["durationSeconds"] = "Укажите длительность"
		}
		if in.Repetitions != nil {
			details["repetitions"] = "Для упражнения на время повторения не указываются"
		}
	case KindWeighted, KindBodyweight:
		if creating && in.Repetitions == nil && !in.CopyPrevious {
			details["repetitions"] = "Укажите количество повторений"
		}
		if in.DurationSeconds != nil {
			details["durationSeconds"] = "Для силового упражнения длительность не указывается"
		}
	}

	if len(details) > 0 {
		return apperror.Validation("Проверьте поля формы", details)
	}
	return nil
}

// kindOfSessionExercise resolves the kind of an exercise row inside a session.
func (s *Service) kindOfSessionExercise(ctx context.Context, actor auth.Actor, sessionID, sessionExerciseID int64) (ExerciseKind, error) {
	var kind string
	err := s.pool.QueryRow(ctx, `
		SELECT e.kind::text
		FROM workout_session_exercises se
		JOIN workout_sessions s ON s.id = se.workout_session_id
		JOIN exercises e ON e.id = se.exercise_id
		WHERE se.id = $1 AND se.workout_session_id = $2 AND s.user_id = $3`,
		sessionExerciseID, sessionID, actor.UserID).Scan(&kind)
	if err != nil {
		return "", translate(err, "Упражнение в тренировке")
	}
	return ParseKind(kind), nil
}

func (s *Service) AddSet(ctx context.Context, actor auth.Actor, sessionID, sessionExerciseID int64, in SetInput) (Set, error) {
	kind, err := s.kindOfSessionExercise(ctx, actor, sessionID, sessionExerciseID)
	if err != nil {
		return Set{}, err
	}
	if err := validateSetForKind(in, kind, true); err != nil {
		return Set{}, err
	}
	if _, err := s.q.GetSessionExercise(ctx, store.GetSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID, UserID: actor.UserID,
	}); err != nil {
		return Set{}, translate(err, "Упражнение в тренировке")
	}

	number, err := s.q.NextSetNumber(ctx, store.NextSetNumberParams{
		WorkoutExerciseID: sessionExerciseID, UserID: actor.UserID,
	})
	if err != nil {
		return Set{}, apperror.Internal(err)
	}

	var weight *float64
	var reps *int16
	var duration *int32
	var distance *float64
	warmup := false
	completed := true

	// Copying the previous set is the common case in the gym: same load, same reps.
	// Copied values are defaults: anything the caller sends explicitly wins, so the
	// inline editor can correct a single field of a copied set.
	if in.CopyPrevious {
		if last, err := s.q.LastSetForExercise(ctx, store.LastSetForExerciseParams{
			WorkoutExerciseID: sessionExerciseID, UserID: actor.UserID,
		}); err == nil {
			weight = store.NumericToPtr(last.WeightKg)
			if last.Repetitions != nil {
				v := *last.Repetitions
				reps = &v
			}
			if last.DurationSeconds != nil {
				v := *last.DurationSeconds
				duration = &v
			}
			distance = store.NumericToPtr(last.DistanceMeters)
			warmup = last.IsWarmup
		} else if !isNoRows(err) {
			return Set{}, apperror.Internal(err)
		}
	}

	if in.WeightKg != nil {
		weight = in.WeightKg
	}
	if in.Repetitions != nil {
		v := int16(*in.Repetitions)
		reps = &v
	}
	if in.DurationSeconds != nil {
		duration = in.DurationSeconds
	}
	if in.DistanceMeters != nil {
		distance = in.DistanceMeters
	}
	if in.IsWarmup != nil {
		warmup = *in.IsWarmup
	}
	if in.IsCompleted != nil {
		completed = *in.IsCompleted
	}

	// Normalise by kind so a bodyweight set never stores a stray weight and a
	// timed set never stores reps, whatever the client sent.
	switch kind {
	case KindBodyweight:
		weight = nil
		duration = nil
	case KindDuration:
		weight = nil
		reps = nil
	}
	if kind == KindDuration && duration == nil {
		// A timed set with no duration cannot exist; default to a minute rather
		// than failing the tap, and let the user correct it inline.
		mins := int32(60)
		duration = &mins
	}
	if kind != KindDuration && reps == nil {
		// Nothing to copy from (first set of the exercise): a sensible default keeps
		// the tap working; the user edits it before completing the set.
		ten := int16(10)
		reps = &ten
	}

	addRow, err := s.q.AddSet(ctx, store.AddSetParams{
		WorkoutExerciseID: sessionExerciseID,
		UserID:            actor.UserID,
		SetNumber:         number,
		Kind:              store.ExerciseKind(kind),
		WeightKg:          store.PtrToNumeric(weight),
		Repetitions:       reps,
		DurationSeconds:   duration,
		DistanceMeters:    store.PtrToNumeric(distance),
		IsWarmup:          warmup,
		IsCompleted:       completed,
	})
	if err != nil {
		return Set{}, translate(err, "Подход")
	}
	return setFromAddRow(addRow), nil
}

func (s *Service) UpdateSet(ctx context.Context, actor auth.Actor, sessionID, sessionExerciseID, setID int64, in SetInput) (Set, error) {
	kind, err := s.kindOfSessionExercise(ctx, actor, sessionID, sessionExerciseID)
	if err != nil {
		return Set{}, err
	}
	if err := validateSetForKind(in, kind, false); err != nil {
		return Set{}, err
	}
	if _, err := s.q.GetSessionExercise(ctx, store.GetSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID, UserID: actor.UserID,
	}); err != nil {
		return Set{}, translate(err, "Упражнение в тренировке")
	}
	current, err := s.q.GetSet(ctx, store.GetSetParams{ID: setID, UserID: actor.UserID})
	if err != nil {
		return Set{}, translate(err, "Подход")
	}
	if current.WorkoutExerciseID != sessionExerciseID {
		return Set{}, apperror.NotFound("Подход")
	}

	weight := current.WeightKg
	reps := current.Repetitions
	duration := current.DurationSeconds
	distance := current.DistanceMeters

	if in.WeightKg != nil {
		weight = store.PtrToNumeric(in.WeightKg)
	}
	if in.Repetitions != nil {
		v := int16(*in.Repetitions)
		reps = &v
	}
	if in.DurationSeconds != nil {
		duration = in.DurationSeconds
	}
	if in.DistanceMeters != nil {
		distance = store.PtrToNumeric(in.DistanceMeters)
	}
	warmup := current.IsWarmup
	if in.IsWarmup != nil {
		warmup = *in.IsWarmup
	}
	completed := current.IsCompleted
	if in.IsCompleted != nil {
		completed = *in.IsCompleted
	}

	// Keep the row consistent with its kind after the edit.
	switch kind {
	case KindBodyweight:
		weight = store.PtrToNumeric(nil)
		duration = nil
	case KindDuration:
		weight = store.PtrToNumeric(nil)
		reps = nil
	}

	updRow, err := s.q.UpdateSet(ctx, store.UpdateSetParams{
		ID:              setID,
		WeightKg:        weight,
		Repetitions:     reps,
		DurationSeconds: duration,
		DistanceMeters:  distance,
		IsWarmup:        warmup,
		IsCompleted:     completed,
		UserID:          actor.UserID,
	})
	if err != nil {
		return Set{}, translate(err, "Подход")
	}
	return setFromUpdateRow(updRow), nil
}

func (s *Service) DeleteSet(ctx context.Context, actor auth.Actor, sessionID, sessionExerciseID, setID int64) error {
	if _, err := s.q.GetSessionExercise(ctx, store.GetSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID, UserID: actor.UserID,
	}); err != nil {
		return translate(err, "Упражнение в тренировке")
	}
	return s.withTx(ctx, func(q *store.Queries) error {
		affected, err := q.DeleteSet(ctx, store.DeleteSetParams{ID: setID, UserID: actor.UserID})
		if err != nil {
			return apperror.Internal(err)
		}
		if affected == 0 {
			return apperror.NotFound("Подход")
		}
		rows, err := q.ListSessionSets(ctx, store.ListSessionSetsParams{
			WorkoutExerciseID: sessionExerciseID, UserID: actor.UserID,
		})
		if err != nil {
			return apperror.Internal(err)
		}
		for i, r := range rows {
			expected := int32(i + 1)
			if r.SetNumber == expected {
				continue
			}
			// Offset first to avoid colliding with a number still in use.
			if err := q.SetSetNumber(ctx, store.SetSetNumberParams{
				ID: r.ID, SetNumber: expected + 100, UserID: actor.UserID,
			}); err != nil {
				return apperror.Internal(err)
			}
		}
		rows, err = q.ListSessionSets(ctx, store.ListSessionSetsParams{
			WorkoutExerciseID: sessionExerciseID, UserID: actor.UserID,
		})
		if err != nil {
			return apperror.Internal(err)
		}
		for i, r := range rows {
			if err := q.SetSetNumber(ctx, store.SetSetNumberParams{
				ID: r.ID, SetNumber: int32(i + 1), UserID: actor.UserID,
			}); err != nil {
				return apperror.Internal(err)
			}
		}
		return nil
	})
}

func setFromRow(r store.WorkoutSet) Set {
	return Set{
		ID:              r.ID,
		SetNumber:       r.SetNumber,
		Kind:            ParseKind(string(r.Kind)),
		WeightKg:        numericToWeight(r.WeightKg),
		Repetitions:     r.Repetitions,
		DurationSeconds: r.DurationSeconds,
		DistanceMeters:  numericToWeight(r.DistanceMeters),
		IsWarmup:        r.IsWarmup,
		IsCompleted:     r.IsCompleted,
	}
}

// setFromAddRow and setFromUpdateRow adapt the per-query row types sqlc generates
// to the same domain value; without them every call site would repeat the mapping.
func setFromAddRow(r store.AddSetRow) Set {
	return Set{
		ID: r.ID, SetNumber: r.SetNumber, Kind: ParseKind(r.Kind),
		WeightKg: numericToWeight(r.WeightKg), Repetitions: r.Repetitions,
		DurationSeconds: r.DurationSeconds, DistanceMeters: numericToWeight(r.DistanceMeters),
		IsWarmup: r.IsWarmup, IsCompleted: r.IsCompleted,
	}
}

func setFromUpdateRow(r store.UpdateSetRow) Set {
	return Set{
		ID: r.ID, SetNumber: r.SetNumber, Kind: ParseKind(r.Kind),
		WeightKg: numericToWeight(r.WeightKg), Repetitions: r.Repetitions,
		DurationSeconds: r.DurationSeconds, DistanceMeters: numericToWeight(r.DistanceMeters),
		IsWarmup: r.IsWarmup, IsCompleted: r.IsCompleted,
	}
}

// groupSessionDetails folds the flat join into sessions → exercises → sets.
func groupSessionDetails(rows []store.ListSessionDetailsRow) []Session {
	type key struct{ session, exercise int64 }
	sessions := []Session{}
	sessionIndex := map[int64]int{}
	exerciseIndex := map[key]int{}

	for _, r := range rows {
		si, ok := sessionIndex[r.SessionID]
		if !ok {
			sessions = append(sessions, Session{
				ID:         r.SessionID,
				TemplateID: r.TemplateID,
				Name:       r.Name,
				StartedAt:  r.StartedAt.Time,
				FinishedAt: timestamptzToPtr(r.FinishedAt),
				Status:     r.Status,
				Notes:      r.Notes,
				CreatedAt:  r.CreatedAt.Time,
				UpdatedAt:  r.UpdatedAt.Time,
				Exercises:  []SessionExercise{},
			})
			si = len(sessions) - 1
			sessionIndex[r.SessionID] = si
		}
		if r.SessionExerciseID == nil {
			continue // session with no exercises yet
		}
		k := key{r.SessionID, *r.SessionExerciseID}
		ei, ok := exerciseIndex[k]
		if !ok {
			sessions[si].Exercises = append(sessions[si].Exercises, SessionExercise{
				ID:               *r.SessionExerciseID,
				ExerciseID:       derefInt64(r.ExerciseID),
				ExerciseName:     derefString(r.ExerciseName),
				Equipment:        derefString(r.Equipment),
				Kind:             ParseKind(r.ExerciseKind.(string)),
				ExerciseArchived: r.ExerciseArchived != nil && *r.ExerciseArchived,
				SortOrder:        derefInt32(r.SortOrder, 0),
				Notes:            derefString(r.ExerciseNotes),
				Sets:             []Set{},
			})
			ei = len(sessions[si].Exercises) - 1
			exerciseIndex[k] = ei
		}
		if r.SetID == nil {
			continue
		}
		sessions[si].Exercises[ei].Sets = append(sessions[si].Exercises[ei].Sets, Set{
			ID:              *r.SetID,
			SetNumber:       derefInt32(r.SetNumber, 0),
			Kind:            ParseKind(r.Kind.(string)),
			WeightKg:        numericToWeight(r.WeightKg),
			Repetitions:     r.Repetitions,
			DurationSeconds: r.DurationSeconds,
			DistanceMeters:  numericToWeight(r.DistanceMeters),
			IsWarmup:        r.IsWarmup,
			IsCompleted:     r.IsCompleted,
		})
	}
	return sessions
}

func isNoRows(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows in result set")
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func pgTimestamptzPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func derefInt32(p *int32, fallback int32) int32 {
	if p == nil {
		return fallback
	}
	return *p
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
