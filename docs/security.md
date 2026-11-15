# IronNote — security model

This application is now exposed on a home server with a public IP address. That
changes the threat model completely: it is no longer "one person on localhost".
This document states what is protected, against whom, and what is deliberately
*not* solved, so nobody mistakes a mitigation for a guarantee.

## Threat model

| Attacker | Capability assumed | Primary defences |
|---|---|---|
| Opportunistic internet scanner | Finds an open port, tries default credentials, known CVEs, common paths | No default credentials, login rate limiting + lockout, generic auth errors, no stack traces, security headers, no unused routes |
| Curious/scripting attacker ("schoolkid with a laptop") | Automates login attempts, looks for IDOR after reading the frontend JS, replays cookies | Argon2id password hashing, per-IP and per-account throttling, ownership checks on **every** object access, session cookies `HttpOnly`/`SameSite`, CSRF token on mutations |
| Authenticated low-privilege user (e.g. a family member's account) | Has a valid account, tries to read/modify other users' data or the catalogue | Server-side role checks, per-user data scoping in SQL, no client-side-only gating |
| Network eavesdropper | Can read plaintext traffic on the path | TLS termination (see "Transport" below); `Secure` cookies when TLS is enabled |
| Someone with full host access | Root on the server | **Out of scope.** No application-level defence is meaningful; protect the host instead. |

Explicitly **out of scope** for this iteration: email verification, password
reset by email, 2FA, OAuth/SSO, audit log shipping, WAF/DDoS protection beyond
connection limits, and per-tenant isolation beyond the three-role model.

## Roles

Three roles, strictly ordered. The role is stored per user and checked server-side
on every request; the frontend hides what a user cannot do, but that is cosmetic.

| Capability | admin | user |
|---|---|---|
| Sign in, sign out, view own profile | ✅ | ✅ |
| Change own password | ✅ | ✅ |
| Record workouts, sets, notes; see own history and stats | ✅ | ✅ |
| Create/edit/delete **own** templates | ✅ | ✅ |
| View the exercise and muscle-group catalogues | ✅ | ✅ |
| Create/edit/delete **muscle groups** | ✅ | ❌ |
| Create/edit/delete/archive **exercises** | ✅ | ❌ |
| Create/disable/delete **users**, reset their passwords | ✅ | ❌ |
| See other users' workouts | ❌ (not by design either) | ❌ |

`admin` is not a superuser over training data: an admin cannot open another
person's workout. Data isolation is a property of the data layer, not of the role.

### Bootstrap

The **first** admin is created from `.env` on startup, once:

- `IRONNOTE_ADMIN_USERNAME` (default `admin`)
- `IRONNOTE_ADMIN_PASSWORD` (required, no default)

The password is hashed with Argon2id and only the hash is stored. If the user
already exists, the env password is **not** reapplied — otherwise restarting with
an old `.env` would silently revert a password change. On first boot with a weak
or example password the server logs a loud warning and, if
`IRONNOTE_REQUIRE_STRONG_ADMIN_PASSWORD=true` (default), **refuses to start**.

Additional users are created by an admin inside the application, with a temporary
password the admin communicates out of band, and a forced change on first login.

## Credentials and sessions

### Passwords

- **Argon2id**, parameters `t=3, m=64 MiB, p=2`, 16-byte salt, 32-byte key —
  above the OWASP minimum and tunable in one place.
- Minimum length 12 characters; the API rejects the obvious weak values
  (top-1000 list checked by length + character-class + a small deny list).
  Passwords are never logged, never returned, and never included in error details.
- Verification is constant-time (`crypto/subtle`) and *always* performs a hash
  comparison even for unknown usernames, so timing cannot enumerate accounts.

### Session tokens

- A random 256-bit token from `crypto/rand`, base64url-encoded.
- The database stores **only the SHA-256 of the token**, so a database leak does
  not leak usable sessions.
- Delivered in an `HttpOnly`, `SameSite=Lax`, `Path=/` cookie; `Secure` when TLS
  is enabled. `Lax` (not `Strict`) so that following a link into the app keeps the
  user signed in; mutations are additionally protected by CSRF (below).
- Absolute lifetime 30 days, idle timeout 7 days, rotated on login. Logout deletes
  the row. Disabling a user deletes all their sessions immediately.
- Session lookup joins the user row, so a disabled or deleted user is rejected on
  the next request rather than at token expiry.

### CSRF

Cookie-authenticated state-changing requests need a second factor that a
cross-site form cannot produce:

- On login the server issues a random CSRF token, returned in the JSON body and
  stored in memory by the client (never in a cookie).
- Every `POST`/`PATCH`/`PUT`/`DELETE` under `/api` requires
  `X-CSRF-Token`; the server compares it in constant time with the value bound to
  the session row.
- `Origin`/`Sec-Fetch-Site` are also checked when present, as defence in depth.
  `Origin` is accepted when it names the same host the request arrived at (ports
  and proxied `$host` differences are normalised), or a configured
  `IRONNOTE_PUBLIC_ORIGIN` / CORS origin. A cross-site page cannot set `Origin` to
  our own hostname, so host equality is the property being tested — an allowlist
  is not required for a same-origin install.
- `SameSite=Lax` means a cross-site `POST` would not carry the cookie anyway.

### Brute force

- Per-account: after 5 failures the account is locked for 15 minutes (stored as
  `locked_until`), then a progressive delay up to 60 minutes.
- Per-IP: token bucket, 10 login attempts per minute, 100 requests/minute general.
  Behind a reverse proxy, `X-Forwarded-For` is trusted **only** from configured
  `TRUSTED_PROXIES`, otherwise a spoofed header would bypass the limit.
- Login responses are identical for unknown user, wrong password and locked
  account ("Неверный логин или пароль"), and take similar time, so the response
  cannot be used to discover valid usernames.
- Successful login clears the failure counter.

## Authorisation enforcement

Defence in depth, in this order — a bug in one layer is caught by the next:

1. **Routing**: `/api/admin/*` requires role admin (middleware), everything except
   `/api/auth/*` and health requires an authenticated session.
2. **Service layer**: writes to the catalogue require `actor.IsAdmin()`; session
   and template methods take the actor id and refuse to act on foreign rows.
3. **SQL**: every query that touches user data filters on `user_id = $actor`. There
   is no "get by id" query without an owner predicate — the scoped variant is the
   only one that exists, so forgetting the filter is a compile error, not a
   silent data leak.
4. **Database**: `user_id` is `NOT NULL` with a foreign key; a session cannot be
   orphaned.

Deleting a user keeps their training history? **No** — deleting a user deletes
their data in the same transaction (`ON DELETE CASCADE` from `users`), which is
the only interpretation that satisfies "see only their own workouts" after the
account is gone. Admins are therefore warned in the UI that deletion is permanent,
and the alternative **disable** (which keeps data but blocks login) is offered
first and is the default action.

## What the catalogue looks like to a normal user

Muscle groups and exercises are **shared reference data**: created and edited by
admins, read by everyone. A normal user sees the full catalogue (they must, to
pick exercises) but cannot modify it. Archived exercises disappear from pickers
while staying resolvable in history.

## Transport and deployment

- The application is designed to sit behind a TLS-terminating reverse proxy
  (the bundled `nginx`, with a documented Certbot/Let's Encrypt path or an outer
  proxy such as Caddy/Traefik). `docker-compose.yml` binds the published port to
  `127.0.0.1` by default so that nothing is exposed before TLS exists; opening it
  to `0.0.0.0` is an explicit change.
- `IRONNOTE_COOKIE_SECURE=true` must be set once TLS is on; the server then marks
  cookies `Secure` and refuses to start with `http://` public origin configured.
- Security headers (also emitted by nginx): `Content-Security-Policy` with no
  `unsafe-inline` for scripts, `X-Content-Type-Options`, `X-Frame-Options: DENY`,
  `Referrer-Policy: no-referrer`, `Cross-Origin-Opener-Policy`,
  `Strict-Transport-Security` when TLS is on.
- Request bodies are capped (1 MiB), server timeouts are set (read, write, idle),
  and the JSON decoder rejects unknown fields.
- No CORS by default: the UI and API share an origin. If `CORS_ALLOWED_ORIGINS` is
  set, credentials are still not allowed — cookie auth must remain same-origin.

## Data at rest

Passwords (Argon2id hashes), session hashes and training data live in PostgreSQL.
Database backups are the operator's responsibility; `.env` holds the bootstrap
password and must stay untracked (it is in `.gitignore`). The API never stores or
logs a plaintext password or a plaintext session token.

## Known limitations (stated, not hidden)

- Login rate limiting is per-process; a multi-replica deployment would need shared
  state. This is a single-container deployment by design.
- There is no password reset via email: an admin resets passwords. Losing the last
  admin password requires database access to recover.
- No 2FA. For a public IP with a handful of known users, a long unique password
  plus lockout is the pragmatic trade-off; TOTP is a sensible next step and the
  session schema does not need to change to add it.
- Timing equalisation is best-effort (always-hash on unknown users); it is not a
  formally verified constant-time login.
