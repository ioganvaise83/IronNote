package auth

import "context"

// Role is a coarse capability level. There is no finer permission model because
// the application has exactly three capabilities (catalogue, user management,
// own training) and inventing a permission table would add indirection without
// removing any risk.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

func (r Role) IsAdmin() bool { return r == RoleAdmin }

func ParseRole(raw string) Role {
	if Role(raw) == RoleAdmin {
		return RoleAdmin
	}
	return RoleUser
}

// Actor is the authenticated caller, resolved once per request by middleware and
// passed explicitly into the service layer.
//
// It is a value, not an interface, and it is always constructed from a database
// lookup — never from a request header or body. That is what stops a client from
// asserting its own role.
type Actor struct {
	UserID      int64
	Username    string
	DisplayName string
	Role        Role
	// SessionID is the auth_sessions row, used for logout and CSRF binding.
	SessionID int64
	// MustChangePassword limits what the session may do until it is changed.
	MustChangePassword bool
}

func (a Actor) IsAdmin() bool { return a.Role.IsAdmin() }

type actorKey struct{}

// WithActor stores the actor on the context. Unexported key: only this package
// can put a value there, so no other code can forge an authenticated context.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// ActorFrom returns the authenticated actor. The second value is false when the
// request is anonymous; callers must treat that as "no access", never as root.
func ActorFrom(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(Actor)
	return actor, ok
}

// MustActor returns the actor or an error, for service methods that require one.
func MustActor(ctx context.Context) (Actor, error) {
	actor, ok := ActorFrom(ctx)
	if !ok {
		return Actor{}, ErrUnauthenticated
	}
	return actor, nil
}
