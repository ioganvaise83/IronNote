package service

import (
	"context"
	"strings"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/auth"
	"github.com/ioganvaise83/ironnote/internal/store"
)

type TemplateExerciseInput struct {
	ExerciseID             int64    `json:"exerciseId"`
	PlannedSets            *int32   `json:"plannedSets"`
	PlannedReps            *int32   `json:"plannedReps"`
	PlannedWeightKg        *float64 `json:"plannedWeightKg"`
	PlannedDurationSeconds *int32   `json:"plannedDurationSeconds"`
}

type TemplateInput struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Exercises   []TemplateExerciseInput `json:"exercises"`
}

func (in TemplateInput) Validate() error {
	details := map[string]string{}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		details["name"] = "Название обязательно"
	}
	if len(in.Exercises) == 0 {
		details["exercises"] = "Добавьте хотя бы одно упражнение"
	}
	seen := map[int64]bool{}
	for _, ex := range in.Exercises {
		if ex.ExerciseID <= 0 {
			details["exercises"] = "Выберите упражнение"
			continue
		}
		if seen[ex.ExerciseID] {
			details["exercises"] = "Упражнение указано дважды"
		}
		seen[ex.ExerciseID] = true
		if ex.PlannedSets != nil && (*ex.PlannedSets < 1 || *ex.PlannedSets > 50) {
			details["plannedSets"] = "Количество подходов: от 1 до 50"
		}
		if ex.PlannedReps != nil && (*ex.PlannedReps < 1 || *ex.PlannedReps > 1000) {
			details["plannedReps"] = "Количество повторений: от 1 до 1000"
		}
		if ex.PlannedDurationSeconds != nil && (*ex.PlannedDurationSeconds < 1 || *ex.PlannedDurationSeconds > 86400) {
			details["plannedDurationSeconds"] = "Длительность: от 1 секунды до 24 часов"
		}
		if ex.PlannedWeightKg != nil && (*ex.PlannedWeightKg < 0 || *ex.PlannedWeightKg > 2000) {
			details["plannedWeightKg"] = "Вес: от 0 до 2000 кг"
		}
	}
	if len(details) > 0 {
		return apperror.Validation("Проверьте поля формы", details)
	}
	return nil
}

func (s *Service) ListTemplates(ctx context.Context, actor auth.Actor) ([]TemplateSummary, error) {
	rows, err := s.q.ListTemplates(ctx, actor.UserID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]TemplateSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, TemplateSummary{
			ID:            r.ID,
			Name:          r.Name,
			Description:   r.Description,
			ExerciseCount: r.ExerciseCount,
			PlannedSets:   r.PlannedSets,
			CreatedAt:     r.CreatedAt.Time,
			UpdatedAt:     r.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (s *Service) GetTemplate(ctx context.Context, actor auth.Actor, id int64) (Template, error) {
	t, err := s.q.GetTemplate(ctx, store.GetTemplateParams{ID: id, UserID: actor.UserID})
	if err != nil {
		return Template{}, translate(err, "Шаблон")
	}
	rows, err := s.q.ListTemplateExercises(ctx, store.ListTemplateExercisesParams{
		TemplateID: id, UserID: actor.UserID,
	})
	if err != nil {
		return Template{}, apperror.Internal(err)
	}
	out := Template{
		ID: t.ID, Name: t.Name, Description: t.Description,
		CreatedAt: t.CreatedAt.Time, UpdatedAt: t.UpdatedAt.Time,
		Exercises: make([]TemplateExercise, 0, len(rows)),
	}
	for _, r := range rows {
		out.Exercises = append(out.Exercises, TemplateExercise{
			ID:                     r.ID,
			ExerciseID:             r.ExerciseID,
			ExerciseName:           r.ExerciseName,
			ExerciseArchived:       r.ExerciseArchived,
			SortOrder:              r.SortOrder,
			PlannedSets:            r.PlannedSets,
			PlannedReps:            r.PlannedReps,
			PlannedWeightKg:        numericToWeight(r.PlannedWeightKg),
			PlannedDurationSeconds: r.PlannedDurationSeconds,
		})
	}
	return out, nil
}

func (s *Service) CreateTemplate(ctx context.Context, actor auth.Actor, in TemplateInput) (Template, error) {
	if err := in.Validate(); err != nil {
		return Template{}, err
	}
	var id int64
	err := s.withTx(ctx, func(q *store.Queries) error {
		created, err := q.CreateTemplate(ctx, store.CreateTemplateParams{
			UserID:      actor.UserID,
			Name:        strings.TrimSpace(in.Name),
			Description: in.Description,
		})
		if err != nil {
			return translate(err, "Шаблон")
		}
		id = created.ID
		return s.insertTemplateExercises(ctx, q, actor, id, in.Exercises)
	})
	if err != nil {
		return Template{}, err
	}
	return s.GetTemplate(ctx, actor, id)
}

func (s *Service) UpdateTemplate(ctx context.Context, actor auth.Actor, id int64, in TemplateInput) (Template, error) {
	if err := in.Validate(); err != nil {
		return Template{}, err
	}
	err := s.withTx(ctx, func(q *store.Queries) error {
		if _, err := q.UpdateTemplate(ctx, store.UpdateTemplateParams{
			ID:          id,
			UserID:      actor.UserID,
			Name:        strings.TrimSpace(in.Name),
			Description: in.Description,
		}); err != nil {
			return translate(err, "Шаблон")
		}
		if err := q.DeleteTemplateExercises(ctx, store.DeleteTemplateExercisesParams{
			TemplateID: id, UserID: actor.UserID,
		}); err != nil {
			return apperror.Internal(err)
		}
		return s.insertTemplateExercises(ctx, q, actor, id, in.Exercises)
	})
	if err != nil {
		return Template{}, err
	}
	return s.GetTemplate(ctx, actor, id)
}

func (s *Service) insertTemplateExercises(ctx context.Context, q *store.Queries, actor auth.Actor, templateID int64, exercises []TemplateExerciseInput) error {
	for i, ex := range exercises {
		if err := q.AddTemplateExercise(ctx, store.AddTemplateExerciseParams{
			TemplateID:             templateID,
			ExerciseID:             ex.ExerciseID,
			SortOrder:              int32(i + 1),
			PlannedSets:            ex.PlannedSets,
			PlannedReps:            ex.PlannedReps,
			PlannedWeightKg:        store.PtrToNumeric(ex.PlannedWeightKg),
			PlannedDurationSeconds: ex.PlannedDurationSeconds,
			UserID:                 actor.UserID,
		}); err != nil {
			return translate(err, "Упражнение")
		}
	}
	return nil
}

// DeleteTemplate removes the plan only. Sessions keep their copied rows, so
// finished history is untouched (template_id becomes NULL via ON DELETE SET NULL).
func (s *Service) DeleteTemplate(ctx context.Context, actor auth.Actor, id int64) error {
	affected, err := s.q.DeleteTemplate(ctx, store.DeleteTemplateParams{ID: id, UserID: actor.UserID})
	if err != nil {
		return translate(err, "Шаблон")
	}
	if affected == 0 {
		return apperror.NotFound("Шаблон")
	}
	return nil
}
