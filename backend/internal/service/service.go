package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/store"
)

// Service applies IronNote's rules. It owns transactions; the store only runs
// single statements.
type Service struct {
	pool *pgxpool.Pool
	q    *store.Queries
	log  *slog.Logger
	loc  *time.Location
}

func New(pool *pgxpool.Pool, logger *slog.Logger, loc *time.Location) *Service {
	return &Service{pool: pool, q: store.New(pool), log: logger, loc: loc}
}

// Timezone is the IANA zone used for month bucketing in statistics.
func (s *Service) Timezone() *time.Location { return s.loc }

// ErrNothingToSave is returned when a workout contains no completed set, so it is
// discarded instead of being recorded as an empty entry.
var ErrNothingToSave = errors.New("в тренировке нет ни одного выполненного подхода")

// withTx runs fn inside a transaction and translates errors afterwards.
func (s *Service) withTx(ctx context.Context, fn func(*store.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return apperror.Internal(fmt.Errorf("begin transaction: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(store.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return apperror.Internal(fmt.Errorf("commit transaction: %w", err))
	}
	return nil
}

// translate converts driver errors into API errors. Unique violations become
// conflicts with a human message; missing rows become 404.
func translate(err error, notFound string) error {
	if err == nil {
		return nil
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.NotFound(notFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return apperror.Conflict(uniqueMessage(pgErr.ConstraintName))
		case "23503": // foreign_key_violation
			return apperror.Rule("Запись ссылается на несуществующий объект или используется в других данных")
		case "23514": // check_violation
			return apperror.Validation("Значение не проходит проверку ограничений", map[string]string{
				"constraint": pgErr.ConstraintName,
			})
		case "23502": // not_null_violation
			return apperror.Validation("Обязательное поле не заполнено", map[string]string{
				"column": pgErr.ColumnName,
			})
		}
	}
	return err // unexpected: the API layer logs it and returns 500
}

func uniqueMessage(constraint string) string {
	switch constraint {
	case "muscle_groups_name_key":
		return "Группа мышц с таким названием уже существует"
	case "exercises_name_key":
		return "Упражнение с таким названием уже существует"
	case "workout_sets_number_key":
		return "Подход с таким номером уже существует"
	case "workout_sessions_single_active_key":
		return "Уже есть незавершённая тренировка"
	case "exercise_muscle_groups_primary_key":
		return "У упражнения может быть только одна основная группа мышц"
	case "workout_template_exercises_sort_order_key", "workout_session_exercises_sort_order_key":
		return "Порядок упражнений конфликтует с существующим"
	case "muscle_groups_pkey", "exercises_pkey":
		return "Запись с таким идентификатором уже существует"
	}
	return "Запись с такими данными уже существует"
}

func numericToWeight(n pgtype.Numeric) Weight {
	if p := store.NumericToPtr(n); p != nil {
		return NewWeight(*p)
	}
	return Weight{}
}

func timestamptzToPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
