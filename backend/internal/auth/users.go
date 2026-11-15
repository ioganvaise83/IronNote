package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ioganvaise83/ironnote/internal/store"
)

// User management. Every mutating method rejects changes that would leave the
// application with no usable administrator, because losing the last admin is
// unrecoverable without direct database access.

func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.q.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(rows))
	for _, r := range rows {
		out = append(out, userFromRow(r.ID, r.Username, r.DisplayName, r.Role,
			r.IsActive, r.MustChangePassword, r.LastLoginAt, r.CreatedAt, r.UpdatedAt))
	}
	return out, nil
}

func (s *Service) GetUser(ctx context.Context, id int64) (User, error) {
	r, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	return userFromRow(r.ID, r.Username, r.DisplayName, r.Role, r.IsActive,
		r.MustChangePassword, r.LastLoginAt, r.CreatedAt, r.UpdatedAt), nil
}

type CreateUserInput struct {
	Username    string
	DisplayName string
	Password    string
	Role        Role
}

// CreateUser is the only way an account appears after bootstrap.
func (s *Service) CreateUser(ctx context.Context, actor Actor, in CreateUserInput) (User, error) {
	if !actor.IsAdmin() {
		return User{}, ErrForbidden
	}
	in.Username = strings.TrimSpace(in.Username)
	if err := ValidateUsername(in.Username); err != nil {
		return User{}, err
	}
	if err := ValidatePassword(in.Password, in.Username); err != nil {
		return User{}, err
	}
	if in.Role != RoleAdmin {
		in.Role = RoleUser
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, err
	}
	// A password set by an administrator is temporary: force a change on first
	// login so the admin does not keep knowing it.
	id, err := s.q.CreateUser(ctx, store.CreateUserParams{
		Username: in.Username, DisplayName: strings.TrimSpace(in.DisplayName),
		PasswordHash: hash, Role: string(in.Role), MustChangePassword: true,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, fmt.Errorf("Пользователь с таким логином уже существует")
		}
		return User{}, err
	}
	s.log.Info("user created", "by", actor.Username, "user_id", id, "role", in.Role)
	return s.GetUser(ctx, id)
}

type UpdateUserInput struct {
	DisplayName *string
	Role        *Role
	IsActive    *bool
}

// UpdateUser changes profile, role or activity. It refuses to demote or disable
// the last active admin, and refuses to disable the caller's own account (which
// would be an instant self-lockout).
func (s *Service) UpdateUser(ctx context.Context, actor Actor, id int64, in UpdateUserInput) (User, error) {
	if !actor.IsAdmin() {
		return User{}, ErrForbidden
	}
	current, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, fmt.Errorf("Пользователь не найден")
		}
		return User{}, err
	}

	role := ParseRole(current.Role)
	active := current.IsActive
	displayName := current.DisplayName
	if in.Role != nil {
		role = *in.Role
	}
	if in.IsActive != nil {
		active = *in.IsActive
	}
	if in.DisplayName != nil {
		displayName = strings.TrimSpace(*in.DisplayName)
	}

	losesAdmin := current.Role == string(RoleAdmin) && role != RoleAdmin
	disables := current.IsActive && !active
	if (losesAdmin || disables) && current.Role == string(RoleAdmin) {
		admins, err := s.q.CountActiveAdmins(ctx)
		if err != nil {
			return User{}, err
		}
		if admins <= 1 {
			return User{}, ErrLastAdmin
		}
	}
	if !active && id == actor.UserID {
		return User{}, fmt.Errorf("Нельзя отключить собственную учётную запись")
	}

	if _, err := s.q.UpdateUserProfile(ctx, store.UpdateUserProfileParams{
		ID: id, DisplayName: displayName, Role: string(role), IsActive: active,
	}); err != nil {
		return User{}, err
	}

	if !active {
		// Disabling must take effect immediately, not at token expiry.
		if err := s.q.DeleteSessionsForUser(ctx, id); err != nil {
			s.log.Warn("could not revoke sessions for disabled user", "user_id", id, "error", err)
		}
	}
	s.log.Info("user updated", "by", actor.Username, "user_id", id,
		"role", role, "active", active)
	return s.GetUser(ctx, id)
}

// ResetPassword lets an admin recover an account (including a locked-out one).
func (s *Service) ResetPassword(ctx context.Context, actor Actor, id int64, password string) error {
	if !actor.IsAdmin() {
		return ErrForbidden
	}
	target, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		return err
	}
	if err := ValidatePassword(password, target.Username); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := s.q.SetUserPassword(ctx, store.SetUserPasswordParams{
		ID: id, PasswordHash: hash, MustChangePassword: true,
	}); err != nil {
		return err
	}
	// A reset invalidates existing sessions: a compromise may be why it happened.
	if err := s.q.DeleteSessionsForUser(ctx, id); err != nil {
		return err
	}
	s.log.Info("password reset by admin", "by", actor.Username, "user_id", id)
	return nil
}

// DeleteUser removes an account and, by database cascade, all of its training
// data. Refuses to delete the last admin or the caller.
func (s *Service) DeleteUser(ctx context.Context, actor Actor, id int64) error {
	if !actor.IsAdmin() {
		return ErrForbidden
	}
	if id == actor.UserID {
		return fmt.Errorf("Нельзя удалить собственную учётную запись")
	}
	target, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("Пользователь не найден")
		}
		return err
	}
	if target.Role == string(RoleAdmin) && target.IsActive {
		admins, err := s.q.CountActiveAdmins(ctx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	affected, err := s.q.DeleteUser(ctx, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("Пользователь не найден")
	}
	s.log.Info("user deleted", "by", actor.Username, "user_id", id, "username", target.Username)
	return nil
}

// BootstrapAdmin creates the first administrator from configuration.
//
// It is idempotent and it never overwrites an existing password: re-running with
// an old .env must not silently revert a password the user changed in the app.
func (s *Service) BootstrapAdmin(ctx context.Context, username, password string) (bool, error) {
	count, err := s.q.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	if err := ValidateUsername(username); err != nil {
		return false, fmt.Errorf("IRONNOTE_ADMIN_USERNAME: %w", err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return false, err
	}
	if _, err := s.q.CreateUser(ctx, store.CreateUserParams{
		Username: username, DisplayName: "Администратор",
		PasswordHash: hash, Role: string(RoleAdmin), MustChangePassword: false,
	}); err != nil {
		return false, err
	}
	s.log.Info("bootstrap admin created", "username", username)
	return true, nil
}

// touchSessionUsage is kept small on purpose: pgtype is an implementation detail
// of the store and should not leak into callers.
var _ = pgtype.Timestamptz{}
