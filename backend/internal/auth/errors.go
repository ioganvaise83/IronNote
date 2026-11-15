package auth

import "errors"

var (
	// ErrUnauthenticated means no valid session was presented.
	ErrUnauthenticated = errors.New("auth: not authenticated")
	// ErrInvalidCredentials is deliberately identical for "no such user" and
	// "wrong password" so the response cannot enumerate accounts.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrAccountDisabled is returned only after a successful password check, so
	// it cannot be used to discover which accounts exist.
	ErrAccountDisabled = errors.New("auth: account disabled")
	// ErrAccountLocked is likewise only revealed to someone who proved they know
	// a password, or after the lockout is already visible in the UI.
	ErrAccountLocked = errors.New("auth: account locked")
	// ErrTooManyAttempts is per-IP throttling, safe to expose (it says nothing
	// about whether the account exists).
	ErrTooManyAttempts = errors.New("auth: too many attempts")
	// ErrForbidden is a role failure.
	ErrForbidden = errors.New("auth: forbidden")
	// ErrLastAdmin prevents locking every administrator out of the application.
	ErrLastAdmin = errors.New("auth: operation would leave no active admin")
)
