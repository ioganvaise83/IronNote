package api_test

// Integration tests for the four behaviours added in this iteration:
// deleting workouts, editing start time and duration, refusing to store an empty
// workout, and timed exercises (бег, плавание) without weight or repetitions.

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func (e *testEnv) exerciseID(admin *client, name, kind string) int64 {
	e.t.Helper()
	status, payload := admin.write(http.MethodPost, "/api/exercises", map[string]any{
		"name": name, "kind": kind, "muscleGroups": []any{},
	})
	if status != http.StatusCreated {
		e.t.Fatalf("create exercise %s: %d (%v)", name, status, payload)
	}
	return int64(payload["id"].(float64))
}

// --- 1. timed exercises ----------------------------------------------------

func TestTimedExerciseAcceptsDurationWithoutWeightOrReps(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	runID := env.exerciseID(admin, "Бег на дорожке", "duration")
	swimID := env.exerciseID(admin, "Плавание", "duration")

	status, session := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Кардио"})
	if status != http.StatusCreated {
		t.Fatalf("start: %d", status)
	}
	sessionID := int64(session["id"].(float64))

	for _, exID := range []int64{runID, swimID} {
		if s, p := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises",
			map[string]any{"exerciseId": exID}); s != http.StatusCreated {
			t.Fatalf("add exercise: %d (%v)", s, p)
		}
	}

	detail := getSession(t, admin, sessionID)
	runRow := detail["exercises"].([]any)[0].(map[string]any)
	swimRow := detail["exercises"].([]any)[1].(map[string]any)
	if runRow["kind"] != "duration" {
		t.Fatalf("exercise kind not exposed: %v", runRow["kind"])
	}

	// Run 30 minutes, then swim 15 — the exact scenario from the brief.
	if s, p := admin.write(http.MethodPost,
		"/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(int64(runRow["id"].(float64)))+"/sets",
		map[string]any{"durationSeconds": 1800, "distanceMeters": 5000}); s != http.StatusCreated {
		t.Fatalf("add run set: %d (%v)", s, p)
	}
	if s, p := admin.write(http.MethodPost,
		"/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(int64(swimRow["id"].(float64)))+"/sets",
		map[string]any{"durationSeconds": 900}); s != http.StatusCreated {
		t.Fatalf("add swim set: %d (%v)", s, p)
	}

	// Finishing must work: a timed effort is real work.
	status, finished := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/finish", nil)
	if status != http.StatusOK {
		t.Fatalf("finish cardio workout: %d (%v)", status, finished)
	}
	if finished["durationSeconds"].(float64) != 2700 {
		t.Fatalf("recorded cardio time = %v, want 2700", finished["durationSeconds"])
	}
	if finished["volumeKg"].(float64) != 0 {
		t.Fatalf("cardio must not produce weight volume, got %v", finished["volumeKg"])
	}
	if finished["distanceMeters"].(float64) != 5000 {
		t.Fatalf("distance = %v, want 5000", finished["distanceMeters"])
	}

	// Per-exercise statistics report time, not volume.
	status, stats := admin.get("/api/stats/exercises/" + itoa(runID))
	if status != http.StatusOK {
		t.Fatalf("exercise stats: %d", status)
	}
	if stats["totalSeconds"].(float64) != 1800 {
		t.Fatalf("stats seconds = %v, want 1800", stats["totalSeconds"])
	}
	if stats["totalVolumeKg"].(float64) != 0 {
		t.Fatalf("stats volume should be 0 for cardio, got %v", stats["totalVolumeKg"])
	}
	if stats["maxWeightKg"] != nil {
		t.Fatalf("cardio has no max weight, got %v", stats["maxWeightKg"])
	}

	// Summary separates cardio time from lifting volume.
	status, summary := admin.get("/api/stats/summary")
	if status != http.StatusOK {
		t.Fatalf("summary: %d", status)
	}
	if summary["totalSeconds"].(float64) != 2700 {
		t.Fatalf("summary totalSeconds = %v, want 2700", summary["totalSeconds"])
	}
}

func TestWeightedExerciseRejectsDurationAndTimedRejectsReps(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	benchID := env.exerciseID(admin, "Жим штанги лёжа", "weighted")
	runID := env.exerciseID(admin, "Бег", "duration")

	_, session := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Тест"})
	sessionID := int64(session["id"].(float64))
	var rows []int64
	for _, exID := range []int64{benchID, runID} {
		_, added := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises",
			map[string]any{"exerciseId": exID})
		rows = append(rows, int64(added["exercise"].(map[string]any)["id"].(float64)))
	}

	// A strength movement must not record time.
	if s, p := admin.write(http.MethodPost,
		"/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(rows[0])+"/sets",
		map[string]any{"durationSeconds": 600}); s != http.StatusBadRequest {
		t.Fatalf("strength exercise accepted a duration: %d (%v)", s, p)
	}
	// A timed movement must not record repetitions.
	if s, p := admin.write(http.MethodPost,
		"/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(rows[1])+"/sets",
		map[string]any{"repetitions": 10}); s != http.StatusBadRequest {
		t.Fatalf("timed exercise accepted repetitions: %d (%v)", s, p)
	}
}

func TestBodyweightExerciseStoresNoLoad(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	id := env.exerciseID(admin, "Подтягивания", "bodyweight")
	_, session := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Спина"})
	sessionID := int64(session["id"].(float64))
	_, added := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises",
		map[string]any{"exerciseId": id})
	rowID := int64(added["exercise"].(map[string]any)["id"].(float64))

	// A client that sends a weight anyway must not have it stored: bodyweight means
	// bodyweight, and a stray load would corrupt volume.
	status, set := admin.write(http.MethodPost,
		"/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(rowID)+"/sets",
		map[string]any{"repetitions": 10, "weightKg": 20})
	if status != http.StatusCreated {
		t.Fatalf("add bodyweight set: %d (%v)", status, set)
	}
	if set["weightKg"] != nil {
		t.Fatalf("bodyweight set stored a weight: %v", set["weightKg"])
	}
	if set["isCompleted"] != true {
		t.Fatal("set should start completed")
	}
}

// --- 2. empty workouts are not recorded ------------------------------------

func TestFinishDiscardsWorkoutWithNothingInIt(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	// Case A: a workout with no exercises at all.
	_, session := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Пустая"})
	sessionID := int64(session["id"].(float64))

	status, payload := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/finish", nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("empty workout finish: status %d, want 422 (%v)", status, payload)
	}
	errObj := payload["error"].(map[string]any)
	if errObj["code"] != "rule_violation" {
		t.Fatalf("error code = %v", errObj["code"])
	}

	// The session must be gone, not merely left in progress.
	if s, _ := admin.get("/api/sessions/" + itoa(sessionID)); s != http.StatusNotFound {
		t.Fatalf("discarded session still readable: %d", s)
	}
	if s, list := admin.get("/api/sessions"); s != http.StatusOK {
		t.Fatal("list failed")
	} else if items := list["items"].([]any); len(items) != 0 {
		t.Fatalf("discarded workout appears in history: %d items", len(items))
	}
	// And no workout is left "in progress" to resume.
	if s, active := admin.get("/api/sessions/active"); s != http.StatusOK {
		t.Fatal("active lookup failed")
	} else if active["session"] != nil {
		t.Fatalf("discarded workout is still resumable: %v", active["session"])
	}

	// Case B: an exercise was added but nothing completed.
	exID := env.exerciseID(admin, "Приседания", "weighted")
	_, second := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Без подходов"})
	secondID := int64(second["id"].(float64))
	admin.write(http.MethodPost, "/api/sessions/"+itoa(secondID)+"/exercises", map[string]any{"exerciseId": exID})

	if s, _ := admin.write(http.MethodPost, "/api/sessions/"+itoa(secondID)+"/finish", nil); s != http.StatusUnprocessableEntity {
		t.Fatalf("workout without completed sets should be discarded, got %d", s)
	}
}

func TestFinishKeepsWorkoutWithCompletedSet(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	exID := env.exerciseID(admin, "Становая тяга", "weighted")
	_, session := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Спина"})
	sessionID := int64(session["id"].(float64))
	_, added := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises", map[string]any{"exerciseId": exID})
	rowID := int64(added["exercise"].(map[string]any)["id"].(float64))
	admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(rowID)+"/sets",
		map[string]any{"weightKg": 100, "repetitions": 5})

	if s, p := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/finish", nil); s != http.StatusOK {
		t.Fatalf("finish with a completed set failed: %d (%v)", s, p)
	}
}

// --- 3. editing start time and duration ------------------------------------

func TestEditStartTimeAndDuration(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	exID := env.exerciseID(admin, "Жим лёжа", "weighted")
	_, session := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Грудь"})
	sessionID := int64(session["id"].(float64))
	_, added := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises", map[string]any{"exerciseId": exID})
	rowID := int64(added["exercise"].(map[string]any)["id"].(float64))
	admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(rowID)+"/sets",
		map[string]any{"weightKg": 80, "repetitions": 10})
	admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/finish", nil)

	// Move the workout to yesterday evening, 19:30, lasting 75 minutes.
	yesterday := time.Now().AddDate(0, 0, -1)
	started := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 19, 30, 0, 0, time.Local)
	status, updated := admin.write(http.MethodPatch, "/api/sessions/"+itoa(sessionID), map[string]any{
		"startedAt":       started.Format(time.RFC3339),
		"durationMinutes": 75,
	})
	if status != http.StatusOK {
		t.Fatalf("edit time: %d (%v)", status, updated)
	}

	gotStart, err := time.Parse(time.RFC3339, updated["startedAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !gotStart.Equal(started) {
		t.Fatalf("startedAt = %v, want %v", gotStart, started)
	}
	if got := updated["durationMinutes"].(float64); got != 75 {
		t.Fatalf("duration = %v, want 75", got)
	}

	// Persisted, not just echoed.
	_, reread := admin.get("/api/sessions/" + itoa(sessionID))
	if got := reread["durationMinutes"].(float64); got != 75 {
		t.Fatalf("duration after reload = %v", got)
	}
}

func TestRejectsNonsenseTimes(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	exID := env.exerciseID(admin, "Тяга", "weighted")
	_, session := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Спина"})
	sessionID := int64(session["id"].(float64))
	_, added := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises", map[string]any{"exerciseId": exID})
	rowID := int64(added["exercise"].(map[string]any)["id"].(float64))
	admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(rowID)+"/sets",
		map[string]any{"weightKg": 60, "repetitions": 10})
	admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/finish", nil)

	future := time.Now().AddDate(0, 0, 3)
	cases := []struct {
		name string
		body map[string]any
	}{
		{"start in the future", map[string]any{"startedAt": future.Format(time.RFC3339)}},
		{"zero duration", map[string]any{"durationMinutes": 0}},
		{"negative duration", map[string]any{"durationMinutes": -30}},
		{"absurd duration", map[string]any{"durationMinutes": 60 * 30}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if s, _ := admin.write(http.MethodPatch, "/api/sessions/"+itoa(sessionID), tc.body); s != http.StatusBadRequest {
				t.Fatalf("%s accepted (status %d)", tc.name, s)
			}
		})
	}
}

// --- 4. deleting workouts -------------------------------------------------

func TestDeleteWorkoutRemovesItFromHistoryAndStats(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)

	exID := env.exerciseID(admin, "Приседания", "weighted")
	_, session := admin.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Ноги"})
	sessionID := int64(session["id"].(float64))
	_, added := admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises", map[string]any{"exerciseId": exID})
	rowID := int64(added["exercise"].(map[string]any)["id"].(float64))
	admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/exercises/"+itoa(rowID)+"/sets",
		map[string]any{"weightKg": 100, "repetitions": 5})
	admin.write(http.MethodPost, "/api/sessions/"+itoa(sessionID)+"/finish", nil)

	// Sanity: it counted.
	if _, summary := admin.get("/api/stats/summary"); summary["volumeKg"].(float64) != 500 {
		t.Fatalf("volume before delete = %v", summary["volumeKg"])
	}

	if s, _ := admin.write(http.MethodDelete, "/api/sessions/"+itoa(sessionID), nil); s != http.StatusNoContent {
		t.Fatalf("delete: status %d", s)
	}

	if s, _ := admin.get("/api/sessions/" + itoa(sessionID)); s != http.StatusNotFound {
		t.Fatalf("deleted session still readable: %d", s)
	}
	if _, list := admin.get("/api/sessions"); len(list["items"].([]any)) != 0 {
		t.Fatal("deleted session still in history")
	}
	if _, summary := admin.get("/api/stats/summary"); summary["volumeKg"].(float64) != 0 {
		t.Fatalf("deleted session still counted in statistics: %v", summary["volumeKg"])
	}
	// The catalogue entry survives: deleting history must never delete an exercise.
	if s, _ := admin.get("/api/exercises/" + itoa(exID)); s != http.StatusOK {
		t.Fatalf("deleting a workout removed the exercise: %d", s)
	}
}

func TestCannotDeleteAnotherUsersWorkout(t *testing.T) {
	env := newTestEnv(t)
	admin := env.newClient()
	admin.login(env.adminUsername, env.adminPassword)
	essa := env.newUser(admin, "essa9", "Squat-Harbor-2026", "user")
	brat := env.newUser(admin, "brat9", "Bench-Harbor-2026", "user")

	_, session := essa.write(http.MethodPost, "/api/sessions", map[string]string{"name": "Моя"})
	sessionID := int64(session["id"].(float64))

	if s, _ := brat.write(http.MethodDelete, "/api/sessions/"+itoa(sessionID), nil); s < 400 {
		t.Fatalf("another user deleted the workout (status %d)", s)
	}
	if s, _ := essa.get("/api/sessions/" + itoa(sessionID)); s != http.StatusOK {
		t.Fatalf("owner lost the workout: %d", s)
	}
}

// --- helpers --------------------------------------------------------------

func getSession(t *testing.T, c *client, id int64) map[string]any {
	t.Helper()
	status, payload := c.get("/api/sessions/" + itoa(id))
	if status != http.StatusOK {
		t.Fatalf("get session: %d", status)
	}
	return payload
}

var _ = context.Background
