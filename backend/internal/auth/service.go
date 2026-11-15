package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ioganvaise83/ironnote/internal/store"
)

// Tunables for brute-force resistance. Exported as a struct so tests can shrink
// the timings without sleeping for real lockout durations.
type Policy struct {
	// MaxFailures before the account locks, and for how long.
	MaxFailures  int
	LockDuration time.Duration
	// MaxFailuresPerIP within IPWindow, independent of the account.
	MaxFailuresPerIP int
	IPWindow         time.Duration
	// SessionLifetime is the absolute cap; IdleTimeout retires quiet sessions.
	SessionLifetime time.Duration
	IdleTimeout     time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		MaxFailures:      5,
		LockDuration:     15 * time.Minute,
		MaxFailuresPerIP: 10,
		IPWindow:         10 * time.Minute,
		SessionLifetime:  30 * 24 * time.Hour,
		IdleTimeout:      7 * 24 * time.Hour,
	}
}

// Service authenticates people and manages their accounts.
type Service struct {
	pool   *pgxpool.Pool
	q      *store.Queries
	log    *slog.Logger
	policy Policy
	// now is injectable so tests can move time without sleeping.
	now func() time.Time
}

func NewService(pool *pgxpool.Pool, logger *slog.Logger, policy Policy) *Service {
	return &Service{pool: pool, q: store.New(pool), log: logger, policy: policy, now: time.Now}
}

// User is the public view of an account: no password hash, no lock counters.
type User struct {
	ID                 int64      `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"displayName"`
	Role               Role       `json:"role"`
	IsActive           bool       `json:"isActive"`
	MustChangePassword bool       `json:"mustChangePassword"`
	LastLoginAt        *time.Time `json:"lastLoginAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func userFromRow(id int64, username, displayName string, role string, active, mustChange bool,
	lastLogin pgtype.Timestamptz, created, updated pgtype.Timestamptz) User {
	return User{
		ID: id, Username: username, DisplayName: displayName, Role: ParseRole(role),
		IsActive: active, MustChangePassword: mustChange,
		LastLoginAt: timestamptzPtr(lastLogin),
		CreatedAt:   created.Time, UpdatedAt: updated.Time,
	}
}

func timestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// LoginResult carries everything the HTTP layer needs to set cookies.
type LoginResult struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	User      User
}

// Login verifies credentials and opens a session.
//
// Order of operations is deliberate:
//  1. per-IP throttle (cheap, does not touch the user table),
//  2. user lookup,
//  3. a hash comparison that happens even when the user does not exist,
//  4. lockout and disabled checks only *after* the password matched,
//  5. a single generic error for every failure that is not a rate limit.
func (s *Service) Login(ctx context.Context, username, password, ip, userAgent string) (LoginResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}
	if len(password) > MaxPasswordLength {
		return LoginResult{}, ErrInvalidCredentials
	}

	ipKey := "ip:" + hashKey(ip)
	if s.ipThrottled(ctx, ipKey) {
		s.log.Warn("login blocked by ip throttle", "ip_key", ipKey)
		return LoginResult{}, ErrTooManyAttempts
	}

	row, err := s.q.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Spend the same time as a real verification, then fail identically.
			DummyVerify(password)
			s.recordAttempt(ctx, ipKey, false)
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, fmt.Errorf("lookup user: %w", err)
	}

	ok, err := VerifyPassword(password, row.PasswordHash)
	if err != nil {
		// A corrupted hash must not authenticate anyone; log it as an incident.
		s.log.Error("stored password hash is unusable", "user_id", row.ID, "error", err)
		return LoginResult{}, ErrInvalidCredentials
	}
	if !ok {
		s.registerFailure(ctx, row.ID, ipKey)
		return LoginResult{}, ErrInvalidCredentials
	}

	// Credentials are correct from here on, so specific messages are safe.
	if !row.IsActive {
		s.recordAttempt(ctx, ipKey, false)
		return LoginResult{}, ErrAccountDisabled
	}
	if row.LockedUntil.Valid && row.LockedUntil.Time.After(s.now()) {
		return LoginResult{}, ErrAccountLocked
	}

	// Transparently upgrade a hash made with older parameters.
	needsRehash := NeedsRehash(row.PasswordHash)
	if needsRehash {
		if newHash, err := HashPassword(password); err == nil {
			if _, err := s.q.SetUserPassword(ctx, store.SetUserPasswordParams{
				ID: row.ID, PasswordHash: newHash, MustChangePassword: row.MustChangePassword,
			}); err != nil {
				s.log.Warn("could not upgrade password hash", "user_id", row.ID, "error", err)
			}
		}
	}

	token, hash, err := NewSessionID()
	if err != nil {
		return LoginResult{}, err
	}
	csrf, err := NewToken()
	if err != nil {
		return LoginResult{}, err
	}
	expires := s.now().Add(s.policy.SessionLifetime)

	if err := s.q.CreateAuthSession(ctx, store.CreateAuthSessionParams{
		UserID:    row.ID,
		TokenHash: hash,
		CsrfToken: csrf,
		ExpiresAt: pgTypeTimestamptz(expires),
		IpAddress: optionalText(ip),
		UserAgent: optionalText(truncate(userAgent, 256)),
	}); err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}

	if err := s.q.ClearFailedLogins(ctx, row.ID); err != nil {
		s.log.Warn("could not clear failed logins", "user_id", row.ID, "error", err)
	}
	s.recordAttempt(ctx, ipKey, true)
	s.log.Info("login succeeded", "user_id", row.ID, "username", row.Username)

	return LoginResult{
		Token: token, CSRFToken: csrf, ExpiresAt: expires,
		User: userFromRow(row.ID, row.Username, row.DisplayName, row.Role,
			row.IsActive, row.MustChangePassword, row.LastLoginAt, row.CreatedAt, row.UpdatedAt),
	}, nil
}

// Authenticate resolves a bearer token into an Actor.
//
// Every request re-reads the user row, so disabling an account or changing a role
// takes effect immediately rather than at the next token refresh.
func (s *Service) Authenticate(ctx context.Context, token string) (Actor, error) {
	if token == "" {
		return Actor{}, ErrUnauthenticated
	}
	if len(token) > 128 {
		return Actor{}, ErrUnauthenticated
	}
	hash := HashToken(token)
	row, err := s.q.GetAuthSession(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Actor{}, ErrUnauthenticated
		}
		return Actor{}, fmt.Errorf("lookup session: %w", err)
	}

	now := s.now()
	if row.RevokedAt.Valid {
		return Actor{}, ErrUnauthenticated
	}
	if !row.ExpiresAt.Time.After(now) {
		return Actor{}, ErrUnauthenticated
	}
	if now.Sub(row.LastSeenAt.Time) > s.policy.IdleTimeout {
		// Idle too long: retire it so the row does not linger as a valid token.
		_ = s.q.RevokeAuthSession(ctx, row.ID)
		return Actor{}, ErrUnauthenticated
	}
	if !row.IsActive {
		return Actor{}, ErrUnauthenticated
	}

	// Refresh last_seen_at at most once a minute: a write per request would be
	// pointless load on a single-user-per-minute workload.
	if now.Sub(row.LastSeenAt.Time) > time.Minute {
		_ = s.q.TouchAuthSession(ctx, row.ID)
	}

	return Actor{
		UserID:             row.UserID,
		Username:           row.Username,
		DisplayName:        row.DisplayName,
		Role:               ParseRole(row.Role),
		SessionID:          row.ID,
		MustChangePassword: row.MustChangePassword,
	}, nil
}

// CSRFToken returns the token bound to a session, for comparison on writes.
func (s *Service) CSRFToken(ctx context.Context, sessionID int64) (string, error) {
	var token string
	err := s.pool.QueryRow(ctx, `SELECT csrf_token FROM auth_sessions WHERE id = $1`, sessionID).Scan(&token)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) Logout(ctx context.Context, sessionID int64) error {
	return s.q.RevokeAuthSession(ctx, sessionID)
}

func (s *Service) registerFailure(ctx context.Context, userID int64, ipKey string) {
	s.recordAttempt(ctx, ipKey, false)
	if _, err := s.q.RegisterFailedLogin(ctx, store.RegisterFailedLoginParams{
		ID:        userID,
		Threshold: int32(s.policy.MaxFailures),
		LockFor:   pgTypeInterval(s.policy.LockDuration),
	}); err != nil {
		s.log.Error("could not register failed login", "user_id", userID, "error", err)
	}
}

func (s *Service) ipThrottled(ctx context.Context, ipKey string) bool {
	count, err := s.q.CountRecentLoginFailures(ctx, store.CountRecentLoginFailuresParams{
		AttemptKey: ipKey,
		Window:     pgTypeInterval(s.policy.IPWindow),
	})
	if err != nil {
		// Fail closed: if the counter cannot be read, refuse rather than let an
		// unlimited stream of guesses through.
		s.log.Error("could not read login throttle", "error", err)
		return true
	}
	return count >= int64(s.policy.MaxFailuresPerIP)
}

func (s *Service) recordAttempt(ctx context.Context, key string, succeeded bool) {
	if err := s.q.RecordLoginAttempt(ctx, store.RecordLoginAttemptParams{
		AttemptKey: key, Succeeded: succeeded,
	}); err != nil {
		s.log.Error("could not record login attempt", "error", err)
	}
}

// ChangeOwnPassword verifies the current password before changing it, so a stolen
// session alone cannot lock the owner out of their account.
func (s *Service) ChangeOwnPassword(ctx context.Context, actor Actor, current, next string) error {
	row, err := s.q.GetUserByID(ctx, actor.UserID)
	if err != nil {
		return err
	}
	ok, err := VerifyPassword(current, row.PasswordHash)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	if err := ValidatePassword(next, row.Username); err != nil {
		return err
	}
	if current == next {
		return fmt.Errorf("Новый пароль совпадает с текущим")
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if _, err := s.q.SetUserPassword(ctx, store.SetUserPasswordParams{
		ID: actor.UserID, PasswordHash: hash, MustChangePassword: false,
	}); err != nil {
		return err
	}
	// Other sessions keep running; only the current one survives a password
	// change, which is what a user expects after a suspected compromise.
	return s.revokeOtherSessions(ctx, actor.UserID, actor.SessionID)
}

func (s *Service) revokeOtherSessions(ctx context.Context, userID, keepSessionID int64) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM auth_sessions WHERE user_id = $1 AND id <> $2`, userID, keepSessionID)
	return err
}

func hashKey(v string) string {
	// Hashing keeps the attempts table from becoming a log of raw IPs and
	// usernames someone tried.
	return fmt.Sprintf("%x", HashToken(v))
}

func optionalText(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n]
}

func pgTypeTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func pgTypeInterval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}
