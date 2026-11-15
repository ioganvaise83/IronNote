package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/auth"
	"github.com/ioganvaise83/ironnote/internal/store"
)

// ExerciseGroupRef links an exercise to a muscle group in requests.
type ExerciseGroupRef struct {
	MuscleGroupID int64 `json:"muscleGroupId"`
	IsPrimary     bool  `json:"isPrimary"`
}

type ExerciseInput struct {
	Name         string
	Description  string
	Equipment    string
	Kind         ExerciseKind
	MuscleGroups []ExerciseGroupRef
}

func (in ExerciseInput) Validate() error {
	details := map[string]string{}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		details["name"] = "Название обязательно"
	} else if utf8.RuneCountInString(name) > maxNameLength {
		details["name"] = "Название длиннее 120 символов"
	}
	if utf8.RuneCountInString(in.Description) > 4000 {
		details["description"] = "Описание длиннее 4000 символов"
	}
	if utf8.RuneCountInString(in.Equipment) > 120 {
		details["equipment"] = "Снаряжение длиннее 120 символов"
	}
	primary := 0
	seen := map[int64]bool{}
	for _, g := range in.MuscleGroups {
		if g.MuscleGroupID <= 0 {
			details["muscleGroups"] = "Некорректная группа мышц"
		}
		if seen[g.MuscleGroupID] {
			details["muscleGroups"] = "Группа мышц указана дважды"
		}
		seen[g.MuscleGroupID] = true
		if g.IsPrimary {
			primary++
		}
	}
	if primary > 1 {
		details["muscleGroups"] = "Основной может быть только одна группа мышц"
	}
	if !validKind(in.Kind) {
		details["kind"] = "Неизвестный тип упражнения"
	}
	if len(details) > 0 {
		return apperror.Validation("Проверьте поля формы", details)
	}
	return nil
}

func validKind(k ExerciseKind) bool {
	switch k {
	case KindWeighted, KindBodyweight, KindDuration:
		return true
	}
	return false
}

type ExerciseFilter struct {
	Search          string
	MuscleGroupID   *int64
	IncludeArchived bool
}

func (s *Service) ListExercises(ctx context.Context, f ExerciseFilter) ([]Exercise, error) {
	params := store.ListExercisesParams{IncludeArchived: &f.IncludeArchived, MuscleGroupID: f.MuscleGroupID}
	if q := strings.TrimSpace(f.Search); q != "" {
		params.Search = &q
	}
	rows, err := s.q.ListExercises(ctx, params)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]Exercise, 0, len(rows))
	for _, r := range rows {
		ex, err := s.exerciseFromRow(r.ID, r.Name, r.Description, r.Equipment, ParseKind(r.Kind),
			r.IsArchived, r.CreatedAt.Time, r.UpdatedAt.Time, r.MuscleGroups)
		if err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, nil
}

func (s *Service) GetExercise(ctx context.Context, id int64) (Exercise, error) {
	r, err := s.q.GetExercise(ctx, id)
	if err != nil {
		return Exercise{}, translate(err, "Упражнение")
	}
	return s.exerciseFromRow(r.ID, r.Name, r.Description, r.Equipment, ParseKind(r.Kind),
		r.IsArchived, r.CreatedAt.Time, r.UpdatedAt.Time, r.MuscleGroups)
}

// muscleGroupJSON mirrors the jsonb_build_object shape in the query.
type muscleGroupJSON struct {
	MuscleGroupID   int64  `json:"muscleGroupId"`
	MuscleGroupName string `json:"muscleGroupName"`
	IsPrimary       bool   `json:"isPrimary"`
}

func (s *Service) exerciseFromRow(id int64, name, description string, equipment *string,
	kind ExerciseKind, archived bool, createdAt, updatedAt time.Time, raw string) (Exercise, error) {
	groups, err := decodeGroups(raw)
	if err != nil {
		return Exercise{}, err
	}
	return Exercise{
		ID:           id,
		Name:         name,
		Description:  description,
		Equipment:    derefString(equipment),
		Kind:         kind,
		IsArchived:   archived,
		MuscleGroups: groups,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

func (s *Service) CreateExercise(ctx context.Context, actor auth.Actor, in ExerciseInput) (Exercise, error) {
	if !actor.IsAdmin() {
		return Exercise{}, apperror.Forbidden("Только администратор может изменять справочник упражнений")
	}
	if err := in.Validate(); err != nil {
		return Exercise{}, err
	}
	var id int64
	err := s.withTx(ctx, func(q *store.Queries) error {
		created, err := q.CreateExercise(ctx, store.CreateExerciseParams{
			Name:        strings.TrimSpace(in.Name),
			Description: in.Description,
			Equipment:   optionalString(in.Equipment),
			Kind:        store.ExerciseKind(in.Kind),
		})
		if err != nil {
			return translate(err, "Упражнение")
		}
		id = created
		return s.replaceGroups(ctx, q, id, in.MuscleGroups)
	})
	if err != nil {
		return Exercise{}, err
	}
	return s.GetExercise(ctx, id)
}

func (s *Service) UpdateExercise(ctx context.Context, actor auth.Actor, id int64, in ExerciseInput) (Exercise, error) {
	if !actor.IsAdmin() {
		return Exercise{}, apperror.Forbidden("Только администратор может изменять справочник упражнений")
	}
	if err := in.Validate(); err != nil {
		return Exercise{}, err
	}
	err := s.withTx(ctx, func(q *store.Queries) error {
		if _, err := q.UpdateExercise(ctx, store.UpdateExerciseParams{
			ID:          id,
			Name:        strings.TrimSpace(in.Name),
			Description: in.Description,
			Equipment:   optionalString(in.Equipment),
			Kind:        store.ExerciseKind(in.Kind),
		}); err != nil {
			return translate(err, "Упражнение")
		}
		return s.replaceGroups(ctx, q, id, in.MuscleGroups)
	})
	if err != nil {
		return Exercise{}, err
	}
	return s.GetExercise(ctx, id)
}

func (s *Service) replaceGroups(ctx context.Context, q *store.Queries, exerciseID int64, groups []ExerciseGroupRef) error {
	if err := q.ReplaceExerciseMuscleGroups(ctx, exerciseID); err != nil {
		return apperror.Internal(err)
	}
	for _, g := range groups {
		if err := q.AddExerciseMuscleGroup(ctx, store.AddExerciseMuscleGroupParams{
			ExerciseID:    exerciseID,
			MuscleGroupID: g.MuscleGroupID,
			IsPrimary:     g.IsPrimary,
		}); err != nil {
			return translate(err, "Группа мышц")
		}
	}
	return nil
}

// DeleteExercise removes an exercise only while it has no history. Once it is
// used by a workout or template the historical record must keep its name, so the
// user is steered to archive instead.
func (s *Service) DeleteExercise(ctx context.Context, actor auth.Actor, id int64) error {
	if !actor.IsAdmin() {
		return apperror.Forbidden("Только администратор может удалять упражнения")
	}
	usage, err := s.q.CountExerciseHistory(ctx, id)
	if err != nil {
		return apperror.Internal(err)
	}
	if usage > 0 {
		return apperror.Conflict("Упражнение уже используется в тренировках или шаблонах — архивируйте его, чтобы сохранить историю")
	}
	affected, err := s.q.DeleteExercise(ctx, id)
	if err != nil {
		return translate(err, "Упражнение")
	}
	if affected == 0 {
		return apperror.NotFound("Упражнение")
	}
	return nil
}

func (s *Service) SetExerciseArchived(ctx context.Context, actor auth.Actor, id int64, archived bool) (Exercise, error) {
	if !actor.IsAdmin() {
		return Exercise{}, apperror.Forbidden("Только администратор может архивировать упражнения")
	}
	if _, err := s.q.SetExerciseArchived(ctx, store.SetExerciseArchivedParams{ID: id, IsArchived: archived}); err != nil {
		return Exercise{}, translate(err, "Упражнение")
	}
	return s.GetExercise(ctx, id)
}

func optionalString(v string) *string {
	t := strings.TrimSpace(v)
	if t == "" {
		return nil
	}
	return &t
}

func decodeGroups(raw string) ([]ExerciseGroup, error) {
	if raw == "" {
		return []ExerciseGroup{}, nil
	}
	var parsed []muscleGroupJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]ExerciseGroup, 0, len(parsed))
	for _, g := range parsed {
		out = append(out, ExerciseGroup{
			MuscleGroupID:   g.MuscleGroupID,
			MuscleGroupName: g.MuscleGroupName,
			IsPrimary:       g.IsPrimary,
		})
	}
	return out, nil
}
