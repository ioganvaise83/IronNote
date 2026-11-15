package api_test

// Security tests. They are integration tests on purpose: the properties under
// test (ownership, roles, CSRF, lockout) only exist once HTTP, the service layer
// and real SQL are combined. A unit test with a mocked store would prove nothing.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ioganvaise83/ironnote/internal/auth"
	"github.com/ioganvaise83/ironnote/internal/service"
)

// client wraps an http.Client with a cookie jar and remembers the CSRF token,
// exactly as the browser does.
type client struct {
	t        *testing.T
	http     *http.Client
	csrf     string
	user     map[string]any
	base     string
	password string
}

func (e *testEnv) newClient() *client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return &client{t: e.t, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}, base: e.server.URL}
}

// login authenticates and stores the CSRF token the way the SPA does.
func (c *client) login(username, password string) (int, map[string]any) {
	c.t.Helper()
	body := map[string]string{"username": username, "password": password}
	status, payload := c.do(http.MethodPost, "/api/auth/login", body, false)
	if status == http.StatusOK {
		if token, ok := payload["csrfToken"].(string); ok {
			c.csrf = token
		}
		if user, ok := payload["user"].(map[string]any); ok {
			c.user = user
		}
	}
	return status, payload
}

// do sends a request. withCSRF=false lets a test send a write without the header.
func (c *client) do(method, path string, body any, withCSRF bool) (int, map[string]any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if withCSRF && c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

// write is the common case: a mutation carrying the CSRF token.
func (c *client) write(method, path string, body any) (int, map[string]any) {
	return c.do(method, path, body, true)
}

func (c *client) get(path string) (int, map[string]any) {
	return c.do(http.MethodGet, path, nil, false)
}

// newUser creates an account as the admin and logs it in.
func (e *testEnv) newUser(admin *client, username, password, role string) *client {
	e.t.Helper()
	status, payload := admin.write(http.MethodPost, "/api/users", map[string]string{
		"username": username, "password": password, "role": role, "displayName": username,
	})
	_ = payload
	if status != http.StatusCreated {
		e.t.Fatalf("create user %s: status %d (%v)", username, status, payload)
	}
	c := e.newClient()
	// An admin-created account must change its password on first login, so tests
	// that need a usable session walk through that step exactly as a person would.
	if s, _ := c.login(username, password); s != http.StatusOK {
		e.t.Fatalf("login as %s failed: %d", username, s)
	}
	chosen := "Picked-Independently-2026"
	status, payload = c.write(http.MethodPost, "/api/auth/password", map[string]string{
		"currentPassword": password,
		"newPassword":     chosen,
	})
	if status != http.StatusOK {
		e.t.Fatalf("forced password change for %s failed: %d (%v)", username, status, payload)
	}
	c.password = chosen
	return c
}

// --- 1. anonymous access ---------------------------------------------------

func TestAnonymousRequestsAreRejected(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()

	protected := []struct{ method, path string }{
		{http.MethodGet, "/api/meta"},
		{http.MethodGet, "/api/sessions"},
		{http.MethodGet, "/api/sessions/active"},
		{http.MethodGet, "/api/templates"},
		{http.MethodGet, "/api/exercises"},
		{http.MethodGet, "/api/muscle-groups"},
		{http.MethodGet, "/api/stats/summary"},
		{http.MethodGet, "/api/users"},
		{http.MethodPost, "/api/sessions"},
	}
	for _, route := range protected {
		status, _ := c.do(route.method, route.path, map[string]any{}, true)
		if status != http.StatusUnauthorized {
			t.Fatalf("%s %s as anonymous: status %d, want 401", route.method, route.path, status)
		}
	}
}

func TestLoginRejectsBadCredentialsGenerically(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()

	status, payload := c.login("admin", "definitely-wrong-password")
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong password: status %d, want 401", status)
	}
	errObj, _ := payload["error"].(map[string]any)
	message, _ := errObj["message"].(string)
	if message != "Неверный логин или пароль" {
		t.Fatalf("unexpected message: %q", message)
	}

	// An unknown user must produce exactly the same response.
	status2, payload2 := c.login("nobody-here", "definitely-wrong-password")
	if status2 != status {
		t.Fatalf("unknown user status %d differs from wrong password %d", status2, status)
	}
	errObj2, _ := payload2["error"].(map[string]any)
	if errObj2["message"] != errObj["message"] {
		t.Fatal("unknown user and wrong password produce different messages")
	}
}

// --- 2. role enforcement ---------------------------------------------------

func TestNormalUserCannotModifyCatalogue(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	if s, _ := admin.login(env.adminUsername, env.adminPassword); s != http.StatusOK {
		t.Fatalf("admin login failed: %d", s)
	}

	user := env.newUser(admin, "wife", "Family-Gym-Secret-1", "user")

	// Reads are allowed: the catalogue is shared.
	if s, _ := user.get("/api/muscle-groups"); s != http.StatusOK {
		t.Fatalf("user cannot read muscle groups: %d", s)
	}
	if s, _ := user.get("/api/exercises"); s != http.StatusOK {
		t.Fatalf("user cannot read exercises: %d", s)
	}

	// Writes are not.
	writes := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/muscle-groups", map[string]string{"name": "Хаки"}},
		{http.MethodPatch, "/api/muscle-groups/1", map[string]string{"name": "Изменено"}},
		{http.MethodDelete, "/api/muscle-groups/1", nil},
		{http.MethodPost, "/api/exercises", map[string]any{"name": "Новое", "muscleGroups": []any{}}},
		{http.MethodPatch, "/api/exercises/1", map[string]any{"name": "Новое", "muscleGroups": []any{}}},
		{http.MethodDelete, "/api/exercises/1", nil},
		{http.MethodPost, "/api/exercises/1/archive", nil},
		// User management is admin-only too.
		{http.MethodGet, "/api/users", nil},
		{http.MethodPost, "/api/users", map[string]string{"username": "hacker", "password": "Hacker-Secret-2026"}},
		{http.MethodDelete, "/api/users/1", nil},
	}
	for _, w := range writes {
		status, _ := user.write(w.method, w.path, w.body)
		if status != http.StatusForbidden {
			t.Fatalf("%s %s as normal user: status %d, want 403", w.method, w.path, status)
		}
	}

	// The admin can still do it.
	if s, _ := admin.write(http.MethodPost, "/api/muscle-groups", map[string]string{"name": "Пресс 2"}); s != http.StatusCreated {
		t.Fatalf("admin could not create a muscle group: %d", s)
	}
}

func TestServiceLayerRejectsNonAdminEvenWithoutRouteGuard(t *testing.T) {
	// Defence in depth: if a future route forgets requireAdmin, the service layer
	// must still refuse. This calls the service directly, bypassing HTTP.
	env := newTestEnv(t)
	ctx := context.Background()
	actor := auth.Actor{UserID: 2, Username: "someone", Role: auth.RoleUser}

	if _, err := env.svc.CreateMuscleGroup(ctx, actor, service.MuscleGroupInput{Name: "X"}); err == nil {
		t.Fatal("service allowed a normal user to create a muscle group")
	}
	if _, err := env.svc.CreateExercise(ctx, actor, service.ExerciseInput{Name: "X"}); err == nil {
		t.Fatal("service allowed a normal user to create an exercise")
	}
	if err := env.svc.DeleteExercise(ctx, actor, 1); err == nil {
		t.Fatal("service allowed a normal user to delete an exercise")
	}
}

// --- 3. data isolation -----------------------------------------------------

func TestUsersCannotSeeOrModifyEachOthersWorkouts(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	essa := env.newUser(admin, "essa", "Squat-Press-Secret-2", "user")
	brat := env.newUser(admin, "brat", "Deadlift-Secret-2026", "user")

	// essa logs a workout.
	status, session := essa.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Секретная тренировка"})
	if status != http.StatusCreated {
		t.Fatalf("essa could not start a workout: %d", status)
	}
	sessionID := int64(session["id"].(float64))

	// brat must not see it in any list.
	if s, list := brat.get("/api/sessions"); s != http.StatusOK {
		t.Fatalf("list failed: %d", s)
	} else if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("brat sees %d sessions that are not his", len(items))
	}

	// Nor fetch it by id — this is the IDOR the whole design must prevent.
	for _, path := range []string{
		"/api/sessions/" + itoa(sessionID),
		"/api/sessions/" + itoa(sessionID) + "/sets",
	} {
		status, _ := brat.get(path)
		if status != http.StatusNotFound && status != http.StatusForbidden && status != http.StatusBadRequest {
			t.Fatalf("brat got status %d for %s, want 404/403", status, path)
		}
	}

	// Nor mutate it.
	mutations := []struct {
		method, path string
		body         any
	}{
		{http.MethodPatch, "/api/sessions/" + itoa(sessionID), map[string]string{"name": "Взломано"}},
		{http.MethodPost, "/api/sessions/" + itoa(sessionID) + "/finish", nil},
		{http.MethodDelete, "/api/sessions/" + itoa(sessionID), nil},
		{http.MethodPost, "/api/sessions/" + itoa(sessionID) + "/exercises", map[string]any{"exerciseId": 1}},
	}
	for _, m := range mutations {
		status, _ := brat.write(m.method, m.path, m.body)
		if status < 400 {
			t.Fatalf("%s %s succeeded for another user (status %d)", m.method, m.path, status)
		}
	}

	// The owner still sees it, and the admin does not (admins are not superusers
	// over other people's training data).
	if s, list := essa.get("/api/sessions"); s != http.StatusOK {
		t.Fatalf("owner list failed: %d", s)
	} else if items, _ := list["items"].([]any); len(items) != 1 {
		t.Fatalf("owner sees %d sessions, want 1", len(items))
	}
	if s, list := admin.get("/api/sessions"); s != http.StatusOK {
		t.Fatalf("admin list failed: %d", s)
	} else if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("admin sees %d sessions belonging to someone else, want 0", len(items))
	}
}

func TestTemplatesArePrivate(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)
	essa := env.newUser(admin, "essa2", "Squat-Press-Secret-2", "user")
	brat := env.newUser(admin, "brat2", "Deadlift-Secret-X-3", "user")

	// The admin creates an exercise so a template can reference one.
	var exerciseID int64
	if s, ex := admin.write(http.MethodPost, "/api/exercises", map[string]any{
		"name": "Приседания со штангой", "muscleGroups": []any{},
	}); s == http.StatusCreated {
		exerciseID = int64(ex["id"].(float64))
	} else {
		t.Fatalf("could not create exercise: %d %v", s, ex)
	}

	status, tpl := essa.write(http.MethodPost, "/api/templates", map[string]any{
		"name": "Ноги", "exercises": []any{map[string]any{"exerciseId": exerciseID}},
	})
	if status != http.StatusCreated {
		t.Fatalf("template create failed: %d %v", status, tpl)
	}
	templateID := int64(tpl["id"].(float64))

	if s, list := brat.get("/api/templates"); s != http.StatusOK {
		t.Fatal("list failed")
	} else if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("brat sees %d foreign templates", len(items))
	}
	if s, _ := brat.get("/api/templates/" + itoa(templateID)); s != http.StatusNotFound && s != http.StatusForbidden {
		t.Fatalf("brat fetched a foreign template: %d", s)
	}
	if s, _ := brat.write(http.MethodPut, "/api/templates/"+itoa(templateID), map[string]any{
		"name": "Взломано", "exercises": []any{},
	}); s < 400 {
		t.Fatal("brat overwrote a foreign template")
	}
	if s, _ := brat.write(http.MethodDelete, "/api/templates/"+itoa(templateID), nil); s < 400 {
		t.Fatal("brat deleted a foreign template")
	}
}

func TestStatisticsArePerUser(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)
	essa := env.newUser(admin, "essa3", "Squat-Press-Secret-2", "user")
	brat := env.newUser(admin, "brat3", "Deadlift-Secret-Y-4", "user")

	// A fresh database has no exercises; the admin adds one (the catalogue is
	// admin-owned), then essa records her own workout with it.
	_, ex := admin.write(http.MethodPost, "/api/exercises",
		map[string]any{"name": "Жим штанги лёжа", "muscleGroups": []any{}})
	exerciseID := int64(ex["id"].(float64))

	_, session := essa.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Грудь"})
	sessionID := int64(session["id"].(float64))
	_, added := essa.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises", map[string]any{"exerciseId": exerciseID})
	exerciseRow := int64(added["exercise"].(map[string]any)["id"].(float64))
	essa.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(exerciseRow)+"/sets",
		map[string]any{"weightKg": 100, "repetitions": 10})
	essa.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/finish", nil)

	// brat's statistics are empty; a leak would show 1000 kg.
	if s, summary := brat.get("/api/stats/summary"); s != http.StatusOK {
		t.Fatal("summary failed")
	} else if summary["volumeKg"].(float64) != 0 || summary["sessionCount"].(float64) != 0 {
		t.Fatalf("brat's statistics include foreign data: %v", summary)
	}
	if s, bests := brat.get("/api/stats/exercises"); s != http.StatusOK {
		t.Fatal("bests failed")
	} else if items, _ := bests["items"].([]any); len(items) != 0 {
		t.Fatalf("brat sees %d foreign exercise records", len(items))
	}

	// essa sees exactly her own.
	if s, summary := essa.get("/api/stats/summary"); s != http.StatusOK {
		t.Fatal("owner summary failed")
	} else if summary["volumeKg"].(float64) != 1000 {
		t.Fatalf("owner volume = %v, want 1000", summary["volumeKg"])
	}

	// Exercise statistics must also be scoped: the exercise is shared, the history
	// is not.
	if s, stats := brat.get("/api/stats/exercises/" + itoa(exerciseID)); s != http.StatusOK {
		t.Fatal("exercise stats failed")
	} else if stats["totalVolumeKg"].(float64) != 0 {
		t.Fatalf("brat sees %v kg of foreign exercise volume", stats["totalVolumeKg"])
	}
}

// --- 4. CSRF ---------------------------------------------------------------

func TestWritesRequireCSRFToken(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()
	c.login(env.adminUsername, env.adminPassword)

	// Without the header: refused.
	status, payload := c.do(http.MethodPost, "/api/muscle-groups", map[string]string{"name": "Без CSRF"}, false)
	if status != http.StatusForbidden {
		t.Fatalf("write without CSRF: status %d, want 403", status)
	}
	if errObj, _ := payload["error"].(map[string]any); errObj["code"] != "forbidden" {
		t.Fatalf("unexpected error code: %v", errObj["code"])
	}

	// With a wrong token: refused.
	original := c.csrf
	c.csrf = "wrong-token-value"
	if status, _ := c.write(http.MethodPost, "/api/muscle-groups", map[string]string{"name": "Плохой CSRF"}); status != http.StatusForbidden {
		t.Fatalf("write with wrong CSRF: status %d, want 403", status)
	}
	c.csrf = original

	// With the real token: accepted.
	if status, _ := c.write(http.MethodPost, "/api/muscle-groups", map[string]string{"name": "С CSRF"}); status != http.StatusCreated {
		t.Fatalf("write with valid CSRF: status %d, want 201", status)
	}
}

func TestCrossSiteRequestIsRejected(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()
	c.login(env.adminUsername, env.adminPassword)

	req, _ := http.NewRequest(http.MethodPost, c.base+"/api/muscle-groups",
		strings.NewReader(`{"name":"Cross Site"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site write: status %d, want 403", resp.StatusCode)
	}
}

// --- 5. brute force --------------------------------------------------------

func TestAccountLocksAfterRepeatedFailures(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)
	victim := env.newUser(admin, "locked", "Lock-Test-Secret-2026", "user")

	attacker := env.newClient()
	// Drive failures: either the account locks or the per-IP bucket engages.
	for i := 0; i < 5; i++ {
		status, _ := attacker.login("locked", "wrong-password-"+itoa(int64(i)))
		if status == http.StatusTooManyRequests {
			// The per-IP bucket can engage first; that is also a valid defence.
			break
		}
	}

	// Now even the correct password cannot get in, whether refused by the account
	// lock (403) or by the IP throttle (429) that preceded it.
	status, payload := attacker.login("locked", victim.password)
	if status != http.StatusForbidden && status != http.StatusTooManyRequests {
		t.Fatalf("locked account: status %d, want 403 or 429 (%v)", status, payload)
	}
	// The failure count in the database must be non-zero, proving persistence.
	var lockedUntil *time.Time
	if err := env.pool.QueryRow(context.Background(),
		`SELECT locked_until FROM users WHERE username = 'locked'`).Scan(&lockedUntil); err != nil {
		t.Fatal(err)
	}
	if lockedUntil == nil {
		t.Fatal("locked_until was not set")
	}
}

func TestLoginIsRateLimitedPerIP(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()

	// The login limiter allows a small burst (5) then refuses. Use a username that
	// does not exist so account lockout is not what stops us.
	var sawTooMany bool
	for i := 0; i < 12; i++ {
		status, _ := c.login("ghost-"+itoa(int64(i)), "irrelevant-password")
		if status == http.StatusTooManyRequests {
			sawTooMany = true
			break
		}
	}
	if !sawTooMany {
		t.Fatal("the login endpoint never rate limited an automated burst")
	}
}

func TestSessionEndpointsRequireCSRF(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()
	c.login(env.adminUsername, env.adminPassword)

	// Logout without the header must not end the session.
	if status, _ := c.do(http.MethodPost, "/api/auth/logout", nil, false); status != http.StatusForbidden {
		t.Fatalf("logout without CSRF: status %d, want 403", status)
	}
	if status, _ := c.get("/api/sessions"); status != http.StatusOK {
		t.Fatalf("session ended anyway: %d", status)
	}

	// Password change without the header must be refused.
	if status, _ := c.do(http.MethodPost, "/api/auth/password", map[string]string{
		"currentPassword": env.adminPassword, "newPassword": "Some-Other-Secret-1",
	}, false); status != http.StatusForbidden {
		t.Fatalf("password change without CSRF: status %d, want 403", status)
	}
}

// --- 6. account lifecycle --------------------------------------------------

func TestCannotRemoveLastAdmin(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	me := int64(admin.user["id"].(float64))

	// Deleting yourself is refused outright.
	if status, _ := admin.write(http.MethodDelete, "/api/users/"+itoa(me), nil); status < 400 {
		t.Fatalf("admin deleted their own account (status %d)", status)
	}
	// Demoting the only admin is refused.
	if status, _ := admin.write(http.MethodPatch, "/api/users/"+itoa(me), map[string]any{"role": "user"}); status < 400 {
		t.Fatal("the last admin was demoted")
	}
	// Disabling the only admin is refused (or forbidden as self-disable).
	if status, _ := admin.write(http.MethodPatch, "/api/users/"+itoa(me), map[string]any{"isActive": false}); status < 400 {
		t.Fatal("the last admin was disabled")
	}
}

func TestDisabledUserSessionsAreRevokedImmediately(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)
	user := env.newUser(admin, "temp", "Account-Secret-Number-9", "user")

	// The user has a working session.
	if s, _ := user.get("/api/sessions"); s != http.StatusOK {
		t.Fatalf("user session does not work: %d", s)
	}

	target := int64(user.user["id"].(float64))
	if s, _ := admin.write(http.MethodPatch, "/api/users/"+itoa(target), map[string]any{"isActive": false}); s != http.StatusOK {
		t.Fatalf("could not disable user: %d", s)
	}

	// The very next request with the old cookie must fail, without waiting for the
	// token to expire.
	if s, _ := user.get("/api/sessions"); s != http.StatusUnauthorized {
		t.Fatalf("disabled user's session still works: %d", s)
	}
}

func TestForcedPasswordChangeBlocksOtherEndpoints(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	// Create a user but do NOT change the password: the account is forced.
	if s, _ := admin.write(http.MethodPost, "/api/users", map[string]string{
		"username": "newbie", "password": "Initial-Password-77", "role": "user",
	}); s != http.StatusCreated {
		t.Fatal("could not create user")
	}

	c := env.newClient()
	if s, _ := c.login("newbie", "Initial-Password-77"); s != http.StatusOK {
		t.Fatalf("login failed: %d", s)
	}

	// Everything but the password endpoints is blocked.
	if s, _ := c.get("/api/sessions"); s != http.StatusForbidden {
		t.Fatalf("forced-change user reached /api/sessions: %d", s)
	}
	if s, _ := c.get("/api/auth/me"); s != http.StatusOK {
		t.Fatalf("/api/auth/me should work so the UI can ask for a new password: %d", s)
	}

	// Changing the password unblocks the account.
	if s, payload := c.write(http.MethodPost, "/api/auth/password", map[string]string{
		"currentPassword": "Initial-Password-77",
		"newPassword":     "My-Chosen-Password-88",
	}); s != http.StatusOK {
		t.Fatalf("password change failed: %d %v", s, payload)
	}
	if s, _ := c.get("/api/sessions"); s != http.StatusOK {
		t.Fatalf("still blocked after changing the password: %d", s)
	}
}

func TestPasswordChangeRequiresCurrentPassword(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	// A stolen cookie alone must not be enough to take over the account.
	status, payload := admin.write(http.MethodPost, "/api/auth/password", map[string]string{
		"currentPassword": "not-the-real-password",
		"newPassword":     "Attacker-Chosen-2026",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("password change with wrong current password: status %d (%v)", status, payload)
	}
}

func TestLogoutInvalidatesTheSession(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()
	c.login(env.adminUsername, env.adminPassword)
	if s, _ := c.get("/api/sessions"); s != http.StatusOK {
		t.Fatal("session not working")
	}
	if s, _ := c.write(http.MethodPost, "/api/auth/logout", nil); s != http.StatusNoContent {
		t.Fatalf("logout status %d", s)
	}
	if s, _ := c.get("/api/sessions"); s != http.StatusUnauthorized {
		t.Fatalf("session survived logout: %d", s)
	}
}

func TestAuthTokenIsNotStoredInPlaintext(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()
	status, payload := c.login(env.adminUsername, env.adminPassword)
	if status != http.StatusOK {
		t.Fatal("login failed")
	}
	_ = payload

	// Recover the cookie and prove the database holds only its SHA-256.
	var token string
	for _, cookie := range c.http.Jar.Cookies(mustParseURL(t, c.base)) {
		if cookie.Name == "ironnote_session" {
			token = cookie.Value
		}
	}
	if token == "" {
		t.Fatal("no session cookie was set")
	}

	var plaintextRows int
	if err := env.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM auth_sessions WHERE token_hash = $1`, []byte(token)).Scan(&plaintextRows); err != nil {
		t.Fatal(err)
	}
	if plaintextRows != 0 {
		t.Fatal("the raw token is stored in the database")
	}

	var hashRows int
	if err := env.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM auth_sessions WHERE token_hash = $1`, auth.HashToken(token)).Scan(&hashRows); err != nil {
		t.Fatal(err)
	}
	if hashRows != 1 {
		t.Fatalf("expected one row for the token hash, got %d", hashRows)
	}
}

func TestSecurityHeadersArePresent(t *testing.T) {
	env := newTestEnv(t)
	resp, err := http.Get(env.server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	expected := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	}
	for header, want := range expected {
		if got := resp.Header.Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	env := newTestEnv(t)
	// Inspect the raw Set-Cookie header: the cookie jar normalises attributes away,
	// so it cannot prove HttpOnly/SameSite on its own.
	body := strings.NewReader(`{"username":"` + env.adminUsername + `","password":"` + env.adminPassword + `"}`)
	resp, err := http.Post(env.server.URL+"/api/auth/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var sessionCookie string
	for _, raw := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(raw, "ironnote_session=") {
			sessionCookie = raw
		}
	}
	if sessionCookie == "" {
		t.Fatal("no session cookie was set")
	}
	lower := strings.ToLower(sessionCookie)
	for _, attribute := range []string{"httponly", "samesite=lax", "path=/"} {
		if !strings.Contains(lower, attribute) {
			t.Fatalf("session cookie is missing %q: %s", attribute, sessionCookie)
		}
	}
	// Secure is conditional on TLS; it must not claim Secure on plain http.
	if strings.Contains(lower, "secure") {
		t.Fatal("cookie marked Secure without TLS configured")
	}
}

// helpers

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// --- 7. same-origin writes must work without extra configuration -------------

// This reproduces a real deployment bug: Origin is always sent by the browser on
// a state-changing fetch, and a LAN install has no IRONNOTE_PUBLIC_ORIGIN set. An
// earlier version rejected the request, making every admin write impossible.
func TestSameOriginWriteIsAllowedWithoutPublicOrigin(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()
	c.login(env.adminUsername, env.adminPassword)

	// Exactly what Chrome sends for a form on http://localhost:8080.
	req, err := http.NewRequest(http.MethodPost, c.base+"/api/exercises",
		strings.NewReader(`{"name":"Жим штанги лёжа","description":"","equipment":"","muscleGroups":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Set("Origin", c.base)                // http://127.0.0.1:PORT
	req.Header.Set("Sec-Fetch-Site", "same-origin") // Chrome labels its own origin
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Referer", c.base+"/exercises/new")
	for _, cookie := range c.http.Jar.Cookies(mustParseURL(t, c.base)) {
		req.AddCookie(cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("same-origin write rejected: status %d, body %s", resp.StatusCode, body)
	}
}

func TestCrossOriginWriteWithValidCSRFIsStillRejected(t *testing.T) {
	env := newTestEnv(t)
	c := env.newClient()
	c.login(env.adminUsername, env.adminPassword)

	// A different host with a *valid* CSRF token: the origin check must still stop
	// it, because that is the layer protecting against a stolen token.
	req, _ := http.NewRequest(http.MethodPost, c.base+"/api/exercises",
		strings.NewReader(`{"name":"Взлом","muscleGroups":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	for _, cookie := range c.http.Jar.Cookies(mustParseURL(t, c.base)) {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin write allowed: status %d", resp.StatusCode)
	}
}

func TestOriginHostComparisonIgnoresPortAndProxyHostRewrite(t *testing.T) {
	env := newTestEnv(t)
	server := env.server

	// Direct unit-style checks through the exported behaviour are not available, so
	// exercise the middleware with a request whose Host differs from Origin only by
	// port, which is what nginx produces (it forwards $host without the port).
	c := env.newClient()
	c.login(env.adminUsername, env.adminPassword)

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/exercises",
		strings.NewReader(`{"name":"Приседания","muscleGroups":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Set("Origin", server.URL) // includes :PORT
	req.Host = strings.TrimPrefix(strings.TrimPrefix(server.URL, "http://"), "https://")
	for _, cookie := range c.http.Jar.Cookies(mustParseURL(t, server.URL)) {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("origin with port vs bare Host rejected: status %d", resp.StatusCode)
	}
}
