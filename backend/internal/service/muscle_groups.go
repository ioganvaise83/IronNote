package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/auth"
	"github.com/ioganvaise83/ironnote/internal/store"
)

const maxNameLength = 120

func (s *Service) ListMuscleGroups(ctx context.Context) ([]MuscleGroup, error) {
	rows, err := s.q.ListMuscleGroups(ctx)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]MuscleGroup, 0, len(rows))
	for _, r := range rows {
		out = append(out, MuscleGroupFromRow(r))
	}
	return out, nil
}

func (s *Service) GetMuscleGroup(ctx context.Context, id int64) (MuscleGroup, error) {
	r, err := s.q.GetMuscleGroup(ctx, id)
	if err != nil {
		return MuscleGroup{}, translate(err, "Группа мышц")
	}
	return MuscleGroupFromRow(r), nil
}

type MuscleGroupInput struct {
	Name        string
	Description string
}

func (in MuscleGroupInput) Validate() error {
	details := map[string]string{}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		details["name"] = "Название обязательно"
	} else if utf8.RuneCountInString(name) > maxNameLength {
		details["name"] = "Название длиннее 120 символов"
	}
	if utf8.RuneCountInString(in.Description) > 1000 {
		details["description"] = "Описание длиннее 1000 символов"
	}
	if len(details) > 0 {
		return apperror.Validation("Проверьте поля формы", details)
	}
	return nil
}

func (s *Service) CreateMuscleGroup(ctx context.Context, actor auth.Actor, in MuscleGroupInput) (MuscleGroup, error) {
	if !actor.IsAdmin() {
		return MuscleGroup{}, apperror.Forbidden("Только администратор может изменять группы мышц")
	}
	if err := in.Validate(); err != nil {
		return MuscleGroup{}, err
	}
	var description *string
	if d := strings.TrimSpace(in.Description); d != "" {
		description = &d
	}
	r, err := s.q.CreateMuscleGroup(ctx, store.CreateMuscleGroupParams{
		Name:        strings.TrimSpace(in.Name),
		Description: description,
	})
	if err != nil {
		return MuscleGroup{}, translate(err, "Группа мышц")
	}
	return MuscleGroupFromRow(r), nil
}

func (s *Service) UpdateMuscleGroup(ctx context.Context, actor auth.Actor, id int64, in MuscleGroupInput) (MuscleGroup, error) {
	if !actor.IsAdmin() {
		return MuscleGroup{}, apperror.Forbidden("Только администратор может изменять группы мышц")
	}
	if err := in.Validate(); err != nil {
		return MuscleGroup{}, err
	}
	var description *string
	if d := strings.TrimSpace(in.Description); d != "" {
		description = &d
	}
	r, err := s.q.UpdateMuscleGroup(ctx, store.UpdateMuscleGroupParams{
		ID:          id,
		Name:        strings.TrimSpace(in.Name),
		Description: description,
	})
	if err != nil {
		return MuscleGroup{}, translate(err, "Группа мышц")
	}
	return MuscleGroupFromRow(r), nil
}

// DeleteMuscleGroup refuses to remove a group still attached to exercises: the
// link table cascades, which would silently strip muscle groups from the
// catalogue. The user is told what to detach first.
func (s *Service) DeleteMuscleGroup(ctx context.Context, actor auth.Actor, id int64) error {
	if !actor.IsAdmin() {
		return apperror.Forbidden("Только администратор может удалять группы мышц")
	}
	usage, err := s.q.CountMuscleGroupUsage(ctx, id)
	if err != nil {
		return apperror.Internal(err)
	}
	if usage > 0 {
		return apperror.Conflict("Группа мышц используется в упражнениях — сначала уберите её из них")
	}
	affected, err := s.q.DeleteMuscleGroup(ctx, id)
	if err != nil {
		return translate(err, "Группа мышц")
	}
	if affected == 0 {
		return apperror.NotFound("Группа мышц")
	}
	return nil
}
