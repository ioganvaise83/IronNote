package api_test

// Integration tests: they talk to the real HTTP stack and a real PostgreSQL.
// Set TEST_DATABASE_URL to run them; without it they skip instead of pretending
// to pass. `go test ./...` stays green on a machine with no database, while CI
// or `make test-integration` exercises the whole path.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ioganvaise83/ironnote/internal/api"
	"github.com/ioganvaise83/ironnote/internal/auth"
	"github.com/ioganvaise83/ironnote/internal/database"
	"github.com/ioganvaise83/ironnote/internal/service"
)

type testEnv struct {
	t             *testing.T
	server        *httptest.Server
	pool          *pgxpool.Pool
	auth          *auth.Service
	svc           *service.Service
	adminUsername string
	adminPassword string
	// admin is a signed-in administrator; do() uses its cookie and CSRF token so
	// the older workflow tests read as before, now as a real authenticated client.
	admin *client
}

// newTestEnv creates a throwaway database per test so tests cannot collide and
// each starts from a genuinely fresh schema (migrations run, seed applied).
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set: skipping integration tests")
	}

	ctx := context.Background()
	dbName := "ironnote_test_" + sanitize(t.Name()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)

	admin, err := database.Connect(ctx, baseURL)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	admin.Close()

	url := replaceDatabase(baseURL, dbName)
	pool, err := database.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Same bootstrap ordering as production: the admin must exist before the
	// ownership migration backfills history.
	adminUsername := "admin"
	adminPassword := "Test-Admin-Password-2026"
	authSvc := auth.NewService(pool, logger, auth.DefaultPolicy())
	migrator := database.Migrator{
		Logger: logger,
		AfterMigration: map[string]func(context.Context, *pgxpool.Pool) error{
			"0003_auth": func(hookCtx context.Context, hookPool *pgxpool.Pool) error {
				_, err := auth.NewService(hookPool, logger, auth.DefaultPolicy()).
					BootstrapAdmin(hookCtx, adminUsername, adminPassword)
				return err
			},
		},
	}
	if err := database.MigrateWithHooks(ctx, pool, migrator); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := authSvc.BootstrapAdmin(ctx, adminUsername, adminPassword); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

	// Drop the database after the test, from a separate connection because the
	// pool itself still holds sessions on it.
	t.Cleanup(func() {
		pool.Close()
		cleanup, err := database.Connect(ctx, baseURL)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
	})
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.UTC
	}
	svc := service.New(pool, logger, loc)
	srv := api.NewServer(api.Options{
		Service: svc, Auth: authSvc, Logger: logger, Pool: pool, Version: "test",
	})
	httpServer := httptest.NewServer(srv.Router())
	t.Cleanup(httpServer.Close)

	env := &testEnv{
		t: t, server: httpServer, pool: pool, auth: authSvc, svc: svc,
		adminUsername: adminUsername, adminPassword: adminPassword,
	}
	env.admin = env.newClient()
	if status, payload := env.admin.login(adminUsername, adminPassword); status != http.StatusOK {
		t.Fatalf("admin login: %d (%v)", status, payload)
	}
	return env
}

// do performs a request and decodes the JSON body into out (if non-nil).
func (e *testEnv) do(method, path string, body any, out any) (*http.Response, []byte) {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
	if err != nil {
		e.t.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Authenticated as the bootstrap admin, with the CSRF token on writes.
	if e.admin != nil {
		if e.admin.csrf != "" {
			req.Header.Set("X-CSRF-Token", e.admin.csrf)
		}
		for _, cookie := range e.admin.http.Jar.Cookies(mustParseURL(e.t, e.server.URL)) {
			req.AddCookie(cookie)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("do: %v", err)
	}
	payload, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		e.t.Fatalf("read body: %v", err)
	}
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			e.t.Fatalf("decode %s %s: %v (body %s)", method, path, err, payload)
		}
	}
	return resp, payload
}

// sanitize makes a test name safe for a database identifier.
func sanitize(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		}
	}
	return string(out)
}

// replaceDatabase swaps the database name in a postgres URL.
func replaceDatabase(raw, dbName string) string {
	idx := strings.LastIndex(raw, "/")
	base := raw[:idx+1]
	tail := raw[idx+1:]
	if q := strings.Index(tail, "?"); q >= 0 {
		return base + dbName + tail[q:]
	}
	return base + dbName
}

func bytesReader(s string) io.Reader { return bytes.NewBufferString(s) }

func ioReadAll(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}

func (e *testEnv) expect(resp *http.Response, want int, payload []byte) {
	e.t.Helper()
	if resp.StatusCode != want {
		e.t.Fatalf("%s %s: status %d, want %d (body %s)", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, payload)
	}
}
