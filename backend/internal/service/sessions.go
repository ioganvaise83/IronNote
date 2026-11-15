package service

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ioganvaise83/ironnote/internal/apperror"
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
func (s *Service) StartSession(ctx context.Context, in StartSessionInput) (Session, error) {
	if _, err := s.q.GetActiveSession(ctx); err == nil {
		return Session{}, apperror.Conflict("Уже есть незавершённая тренировка — продолжите или завершите её")
	} else if !isNoRows(err) {
		return Session{}, apperror.Internal(err)
	}

	var name string
	var exercises []TemplateExercise
	if in.TemplateID != nil {
		tpl, err := s.GetTemplate(ctx, *in.TemplateID)
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
			})
			if err != nil {
				return translate(err, "Упражнение")
			}
			if ex.PlannedSets == nil {
				continue
			}
			count := *ex.PlannedSets
			if count > 20 {
				count = 20 // a template typo should not create 50 empty rows
			}
			for n := int32(1); n <= count; n++ {
				if _, err := q.AddSet(ctx, store.AddSetParams{
					WorkoutExerciseID: added.ID,
					SetNumber:         n,
					WeightKg:          store.PtrToNumeric(ex.PlannedWeightKg.Ptr()),
					Repetitions:       int16(derefInt32(ex.PlannedReps, 10)),
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
	return s.GetSession(ctx, id)
}

func (s *Service) GetActiveSession(ctx context.Context) (*Session, error) {
	row, err := s.q.GetActiveSession(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, apperror.Internal(err)
	}
	session, err := s.loadSession(ctx, row.ID, false)
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
func (s *Service) ListSessions(ctx context.Context, f SessionFilter) ([]SessionSummary, error) {
	rows, err := s.q.ListSessionDetails(ctx, store.ListSessionDetailsParams{
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

func (s *Service) GetSession(ctx context.Context, id int64) (Session, error) {
	return s.loadSession(ctx, id, true)
}

func (s *Service) loadSession(ctx context.Context, id int64, withPrevious bool) (Session, error) {
	rows, err := s.q.ListSessionDetails(ctx, store.ListSessionDetailsParams{SessionID: &id})
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
			prev, prevAt, err := s.previousSets(ctx, session.Exercises[i].ExerciseID, session.ID)
			if err != nil {
				return Session{}, err
			}
			session.Exercises[i].PreviousSets = prev
			session.Exercises[i].PreviousAt = prevAt
		}
	}
	return session, nil
}

func (s *Service) previousSets(ctx context.Context, exerciseID, excludeSessionID int64) ([]Set, *time.Time, error) {
	rows, err := s.q.PreviousExerciseSets(ctx, store.PreviousExerciseSetsParams{
		ExerciseID:       exerciseID,
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

// UpdateSessionInput edits the header of a session, optionally re-finishing it.
type UpdateSessionInput struct {
	Name      *string    `json:"name"`
	Notes     *string    `json:"notes"`
	StartedAt *time.Time `json:"startedAt"`
}

// UpdateSession keeps finished sessions editable, as required: notes and the
// time can be corrected after the fact.
func (s *Service) UpdateSession(ctx context.Context, id int64, in UpdateSessionInput) (Session, error) {
	current, err := s.q.GetSession(ctx, id)
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
	if current.FinishedAt.Valid && startedAt.After(current.FinishedAt.Time) {
		return Session{}, apperror.Rule("Начало не может быть позже завершения")
	}
	if _, err := s.q.UpdateSession(ctx, store.UpdateSessionParams{
		ID: id, Name: name, Notes: notes, StartedAt: pgTimestamptz(startedAt),
	}); err != nil {
		return Session{}, translate(err, "Тренировка")
	}
	return s.GetSession(ctx, id)
}

// FinishSession stamps the end time. A workout with no completed set is still
// finishable — the user may have logged nothing worth keeping — but the UI warns.
func (s *Service) FinishSession(ctx context.Context, id int64) (Session, error) {
	current, err := s.q.GetSession(ctx, id)
	if err != nil {
		return Session{}, translate(err, "Тренировка")
	}
	if current.Status == StatusCompleted {
		return s.GetSession(ctx, id)
	}
	now := time.Now()
	if now.Before(current.StartedAt.Time) {
		now = current.StartedAt.Time
	}
	if _, err := s.q.FinishSession(ctx, store.FinishSessionParams{ID: id, FinishedAt: pgTimestamptz(now)}); err != nil {
		return Session{}, translate(err, "Тренировка")
	}
	return s.GetSession(ctx, id)
}

// ReopenSession moves a finished workout back to in_progress so it can be edited
// in the active screen. It respects the single-active-session rule.
func (s *Service) ReopenSession(ctx context.Context, id int64) (Session, error) {
	if _, err := s.q.GetActiveSession(ctx); err == nil {
		return Session{}, apperror.Conflict("Сначала завершите текущую незавершённую тренировку")
	} else if !isNoRows(err) {
		return Session{}, apperror.Internal(err)
	}
	if _, err := s.q.ReopenSession(ctx, id); err != nil {
		return Session{}, translate(err, "Тренировка")
	}
	return s.GetSession(ctx, id)
}

func (s *Service) DeleteSession(ctx context.Context, id int64) error {
	affected, err := s.q.DeleteSession(ctx, id)
	if err != nil {
		return translate(err, "Тренировка")
	}
	if affected == 0 {
		return apperror.NotFound("Тренировка")
	}
	return nil
}

// AddSessionExercise appends a movement to a workout.
func (s *Service) AddSessionExercise(ctx context.Context, sessionID, exerciseID int64, notes string) (SessionExercise, error) {
	order, err := s.q.NextSessionExerciseSortOrder(ctx, sessionID)
	if err != nil {
		return SessionExercise{}, apperror.Internal(err)
	}
	added, err := s.q.AddSessionExercise(ctx, store.AddSessionExerciseParams{
		WorkoutSessionID: sessionID,
		ExerciseID:       exerciseID,
		SortOrder:        order,
		Notes:            notes,
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
		Equipment: derefString(ex.Equipment), SortOrder: added.SortOrder, Notes: added.Notes,
		Sets: []Set{},
	}, nil
}

// MoveSessionExercise renumbers siblings inside a transaction so sort_order
// stays unique at every step.
func (s *Service) MoveSessionExercise(ctx context.Context, sessionID, sessionExerciseID int64, direction int) error {
	if direction != -1 && direction != 1 {
		return apperror.Validation("Направление должно быть -1 или 1", nil)
	}
	return s.withTx(ctx, func(q *store.Queries) error {
		rows, err := q.ListSessionExercises(ctx, sessionID)
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
				ID: r.ID, SortOrder: int32(i + 1 + len(rows)),
			}); err != nil {
				return apperror.Internal(err)
			}
		}
		rows[index], rows[target] = rows[target], rows[index]
		for i, r := range rows {
			if err := q.SetSessionExerciseSortOrder(ctx, store.SetSessionExerciseSortOrderParams{
				ID: r.ID, SortOrder: int32(i + 1),
			}); err != nil {
				return apperror.Internal(err)
			}
		}
		return nil
	})
}

func (s *Service) UpdateSessionExerciseNotes(ctx context.Context, sessionID, sessionExerciseID int64, notes string) error {
	if _, err := s.q.UpdateSessionExercise(ctx, store.UpdateSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID, Notes: notes,
		SortOrder: 0,
	}); err != nil {
		return translate(err, "Упражнение в тренировке")
	}
	return nil
}

func (s *Service) DeleteSessionExercise(ctx context.Context, sessionID, sessionExerciseID int64) error {
	affected, err := s.q.DeleteSessionExercise(ctx, store.DeleteSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID,
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
	WeightKg    *float64 `json:"weightKg"`
	Repetitions *int32   `json:"repetitions"`
	IsWarmup    *bool    `json:"isWarmup"`
	IsCompleted *bool    `json:"isCompleted"`
	// CopyPrevious repeats the previous set's weight and reps when creating.
	CopyPrevious bool `json:"copyPrevious"`
}

func validateSet(in SetInput, creating bool) error {
	details := map[string]string{}
	if in.WeightKg != nil && (*in.WeightKg < 0 || *in.WeightKg > 2000) {
		details["weightKg"] = "Вес: от 0 до 2000 кг"
	}
	if in.Repetitions != nil && (*in.Repetitions < 1 || *in.Repetitions > 1000) {
		details["repetitions"] = "Повторения: от 1 до 1000"
	}
	if creating && in.Repetitions == nil {
		details["repetitions"] = "Укажите количество повторений"
	}
	if len(details) > 0 {
		return apperror.Validation("Проверьте поля формы", details)
	}
	return nil
}

func (s *Service) AddSet(ctx context.Context, sessionID, sessionExerciseID int64, in SetInput) (Set, error) {
	if err := validateSet(in, true); err != nil {
		return Set{}, err
	}
	if _, err := s.q.GetSessionExercise(ctx, store.GetSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID,
	}); err != nil {
		return Set{}, translate(err, "Упражнение в тренировке")
	}

	number, err := s.q.NextSetNumber(ctx, sessionExerciseID)
	if err != nil {
		return Set{}, apperror.Internal(err)
	}

	var weight *float64
	reps := int16(*in.Repetitions)
	warmup := false
	completed := true
	if in.CopyPrevious {
		if last, err := s.q.LastSetForExercise(ctx, sessionExerciseID); err == nil {
			weight = store.NumericToPtr(last.WeightKg)
			reps = last.Repetitions
			warmup = last.IsWarmup
		} else if !isNoRows(err) {
			return Set{}, apperror.Internal(err)
		}
	}
	if in.WeightKg != nil {
		weight = in.WeightKg
	}
	if in.IsWarmup != nil {
		warmup = *in.IsWarmup
	}
	if in.IsCompleted != nil {
		completed = *in.IsCompleted
	}

	row, err := s.q.AddSet(ctx, store.AddSetParams{
		WorkoutExerciseID: sessionExerciseID,
		SetNumber:         number,
		WeightKg:          store.PtrToNumeric(weight),
		Repetitions:       reps,
		IsWarmup:          warmup,
		IsCompleted:       completed,
	})
	if err != nil {
		return Set{}, translate(err, "Подход")
	}
	return setFromRow(row), nil
}

func (s *Service) UpdateSet(ctx context.Context, sessionID, sessionExerciseID, setID int64, in SetInput) (Set, error) {
	if err := validateSet(in, false); err != nil {
		return Set{}, err
	}
	if _, err := s.q.GetSessionExercise(ctx, store.GetSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID,
	}); err != nil {
		return Set{}, translate(err, "Упражнение в тренировке")
	}
	current, err := s.q.GetSet(ctx, setID)
	if err != nil {
		return Set{}, translate(err, "Подход")
	}
	if current.WorkoutExerciseID != sessionExerciseID {
		return Set{}, apperror.NotFound("Подход")
	}

	weight := current.WeightKg
	if in.WeightKg != nil {
		weight = store.PtrToNumeric(in.WeightKg)
	}
	reps := current.Repetitions
	if in.Repetitions != nil {
		reps = int16(*in.Repetitions)
	}
	warmup := current.IsWarmup
	if in.IsWarmup != nil {
		warmup = *in.IsWarmup
	}
	completed := current.IsCompleted
	if in.IsCompleted != nil {
		completed = *in.IsCompleted
	}

	row, err := s.q.UpdateSet(ctx, store.UpdateSetParams{
		ID: setID, WeightKg: weight, Repetitions: reps, IsWarmup: warmup, IsCompleted: completed,
	})
	if err != nil {
		return Set{}, translate(err, "Подход")
	}
	return setFromRow(row), nil
}

// DeleteSet removes a set and renumbers the rest so numbering stays 1..n.
func (s *Service) DeleteSet(ctx context.Context, sessionID, sessionExerciseID, setID int64) error {
	if _, err := s.q.GetSessionExercise(ctx, store.GetSessionExerciseParams{
		ID: sessionExerciseID, WorkoutSessionID: sessionID,
	}); err != nil {
		return translate(err, "Упражнение в тренировке")
	}
	return s.withTx(ctx, func(q *store.Queries) error {
		affected, err := q.DeleteSet(ctx, setID)
		if err != nil {
			return apperror.Internal(err)
		}
		if affected == 0 {
			return apperror.NotFound("Подход")
		}
		rows, err := q.ListSessionSets(ctx, sessionExerciseID)
		if err != nil {
			return apperror.Internal(err)
		}
		for i, r := range rows {
			expected := int32(i + 1)
			if r.SetNumber == expected {
				continue
			}
			// Offset first to avoid colliding with a number still in use.
			if err := q.SetSetNumber(ctx, store.SetSetNumberParams{ID: r.ID, SetNumber: expected + 100}); err != nil {
				return apperror.Internal(err)
			}
		}
		rows, err = q.ListSessionSets(ctx, sessionExerciseID)
		if err != nil {
			return apperror.Internal(err)
		}
		for i, r := range rows {
			if err := q.SetSetNumber(ctx, store.SetSetNumberParams{ID: r.ID, SetNumber: int32(i + 1)}); err != nil {
				return apperror.Internal(err)
			}
		}
		return nil
	})
}

func setFromRow(r store.WorkoutSet) Set {
	return Set{
		ID:          r.ID,
		SetNumber:   r.SetNumber,
		WeightKg:    numericToWeight(r.WeightKg),
		Repetitions: r.Repetitions,
		IsWarmup:    r.IsWarmup,
		IsCompleted: r.IsCompleted,
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
			ID:          *r.SetID,
			SetNumber:   derefInt32(r.SetNumber, 0),
			WeightKg:    numericToWeight(r.WeightKg),
			Repetitions: derefInt16(r.Repetitions),
			IsWarmup:    r.IsWarmup != nil && *r.IsWarmup,
			IsCompleted: r.IsCompleted != nil && *r.IsCompleted,
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

func derefInt16(p *int16) int16 {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
