package api_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

type muscleGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type exercise struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	IsArchived   bool   `json:"isArchived"`
	MuscleGroups []struct {
		MuscleGroupID   int64  `json:"muscleGroupId"`
		MuscleGroupName string `json:"muscleGroupName"`
		IsPrimary       bool   `json:"isPrimary"`
	} `json:"muscleGroups"`
}

type setPayload struct {
	ID          int64    `json:"id"`
	SetNumber   int32    `json:"setNumber"`
	WeightKg    *float64 `json:"weightKg"`
	Repetitions int16    `json:"repetitions"`
	IsWarmup    bool     `json:"isWarmup"`
	IsCompleted bool     `json:"isCompleted"`
}

type sessionPayload struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	StartedAt     string  `json:"startedAt"`
	FinishedAt    *string `json:"finishedAt"`
	VolumeKg      float64 `json:"volumeKg"`
	ExerciseCount int     `json:"exerciseCount"`
	CompletedSets int     `json:"completedSetCount"`
	Exercises     []struct {
		ID           int64        `json:"id"`
		ExerciseID   int64        `json:"exerciseId"`
		ExerciseName string       `json:"exerciseName"`
		Sets         []setPayload `json:"sets"`
	} `json:"exercises"`
}

// TestFullWorkoutJourney walks the scenario from the brief end to end:
// create a muscle group, an exercise, start a workout, log sets, finish it,
// then read it back from history and statistics.
func TestFullWorkoutJourney(t *testing.T) {
	env := newTestEnv(t)

	// 1. The seeded reference list is available.
	var groups struct {
		Items []muscleGroup `json:"items"`
	}
	resp, raw := env.do(http.MethodGet, "/api/muscle-groups", nil, &groups)
	env.expect(resp, http.StatusOK, raw)
	if len(groups.Items) < 10 {
		t.Fatalf("expected seeded muscle groups, got %d", len(groups.Items))
	}

	// 2. Create "Грудь" explicitly (the brief's first step) — but it is seeded,
	//    so this must be a clean conflict rather than a duplicate.
	resp, raw = env.do(http.MethodPost, "/api/muscle-groups", map[string]string{"name": "Грудь"}, nil)
	env.expect(resp, http.StatusConflict, raw)

	var createdGroup muscleGroup
	resp, raw = env.do(http.MethodPost, "/api/muscle-groups",
		map[string]string{"name": "Тестовая группа", "description": "для теста"}, &createdGroup)
	env.expect(resp, http.StatusCreated, raw)
	if createdGroup.ID == 0 || createdGroup.Name != "Тестовая группа" {
		t.Fatalf("created group = %+v", createdGroup)
	}

	// 3. Create the exercise with a primary muscle group.
	chestID := findGroup(t, groups.Items, "Грудь")
	var bench exercise
	resp, raw = env.do(http.MethodPost, "/api/exercises", map[string]any{
		"name":        "Жим штанги лёжа",
		"description": "Классический жим",
		"equipment":   "Штанга",
		"muscleGroups": []map[string]any{
			{"muscleGroupId": chestID, "isPrimary": true},
		},
	}, &bench)
	env.expect(resp, http.StatusCreated, raw)
	if len(bench.MuscleGroups) != 1 || !bench.MuscleGroups[0].IsPrimary {
		t.Fatalf("expected one primary muscle group, got %+v", bench.MuscleGroups)
	}

	// 4. Start a workout from scratch.
	var session sessionPayload
	resp, raw = env.do(http.MethodPost, "/api/sessions", map[string]string{"name": "Грудь"}, &session)
	env.expect(resp, http.StatusCreated, raw)
	if session.Status != "in_progress" {
		t.Fatalf("status = %s", session.Status)
	}

	// 5. A second concurrent workout is refused, pointing at the existing one.
	resp, raw = env.do(http.MethodPost, "/api/sessions", map[string]string{"name": "Вторая"}, nil)
	env.expect(resp, http.StatusConflict, raw)

	// 6. Add the exercise and log the sets from the brief.
	var added struct {
		Exercise struct {
			ID int64 `json:"id"`
		} `json:"exercise"`
	}
	resp, raw = env.do(http.MethodPost, "/api/sessions/"+itoa(session.ID)+"/exercises",
		map[string]any{"exerciseId": bench.ID}, &added)
	env.expect(resp, http.StatusCreated, raw)

	base := "/api/sessions/" + itoa(session.ID) + "/exercises/" + itoa(added.Exercise.ID) + "/sets"
	plan := []struct {
		weight float64
		reps   int
		warmup bool
	}{
		{60, 12, false},
		{80, 10, false},
		{90, 8, false},
		{90, 8, false},
		{40, 15, true},
	}
	for _, p := range plan {
		var set setPayload
		resp, raw = env.do(http.MethodPost, base, map[string]any{
			"weightKg": p.weight, "repetitions": p.reps, "isWarmup": p.warmup,
		}, &set)
		env.expect(resp, http.StatusCreated, raw)
	}

	// 7. A patch changes only the field sent (the mobile inline editor).
	var patched setPayload
	resp, raw = env.do(http.MethodPatch, base+"/1", map[string]any{"repetitions": 13}, &patched)
	env.expect(resp, http.StatusOK, raw)
	if patched.Repetitions != 13 || patched.WeightKg == nil || *patched.WeightKg != 60 {
		t.Fatalf("partial patch broke other fields: %+v", patched)
	}

	// 8. Copy the previous set's values in one tap.
	var copied setPayload
	resp, raw = env.do(http.MethodPost, base, map[string]any{"repetitions": 1, "copyPrevious": true}, &copied)
	env.expect(resp, http.StatusCreated, raw)
	if copied.WeightKg == nil || *copied.WeightKg != 40 || copied.Repetitions != 15 {
		t.Fatalf("copyPrevious did not copy the last set: %+v", copied)
	}

	// 9. Delete that copied set; numbering must stay contiguous.
	resp, raw = env.do(http.MethodDelete, base+"/"+itoa(copied.ID), nil, nil)
	env.expect(resp, http.StatusNoContent, raw)

	// 10. Finish: volume excludes the warmup and counts 60×13 + 80×10 + 90×8 + 90×8.
	resp, raw = env.do(http.MethodPost, "/api/sessions/"+itoa(session.ID)+"/finish", nil, &session)
	env.expect(resp, http.StatusOK, raw)
	if session.FinishedAt == nil {
		t.Fatal("finished workout has no finishedAt")
	}
	wantVolume := 60*13 + 80*10 + 90*8 + 90*8
	if session.VolumeKg != float64(wantVolume) {
		t.Fatalf("volume = %v, want %v", session.VolumeKg, wantVolume)
	}
	if session.CompletedSets != 4 {
		t.Fatalf("completed sets = %d, want 4", session.CompletedSets)
	}

	// 11. History shows it, newest first, with the same numbers.
	var history struct {
		Items []struct {
			ID       int64   `json:"id"`
			VolumeKg float64 `json:"volumeKg"`
			Status   string  `json:"status"`
		} `json:"items"`
	}
	resp, raw = env.do(http.MethodGet, "/api/sessions?status=completed", nil, &history)
	env.expect(resp, http.StatusOK, raw)
	if len(history.Items) != 1 || history.Items[0].ID != session.ID {
		t.Fatalf("history = %+v", history.Items)
	}
	if history.Items[0].VolumeKg != float64(wantVolume) {
		t.Fatalf("history volume = %v, want %v", history.Items[0].VolumeKg, wantVolume)
	}

	// 12. Statistics agree with history — the point of computing them once.
	var summary struct {
		SessionCount      int     `json:"sessionCount"`
		CompletedSetCount int     `json:"completedSetCount"`
		VolumeKg          float64 `json:"volumeKg"`
		Monthly           []struct {
			Month        string `json:"month"`
			SessionCount int    `json:"sessionCount"`
		} `json:"monthly"`
	}
	resp, raw = env.do(http.MethodGet, "/api/stats/summary", nil, &summary)
	env.expect(resp, http.StatusOK, raw)
	if summary.SessionCount != 1 || summary.VolumeKg != float64(wantVolume) || summary.CompletedSetCount != 4 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(summary.Monthly) != 1 || summary.Monthly[0].SessionCount != 1 {
		t.Fatalf("monthly = %+v", summary.Monthly)
	}

	// 13. Per-exercise records: max weight, no actual single, Epley estimate.
	var stats struct {
		MaxWeightKg    *float64 `json:"maxWeightKg"`
		MaxSingleKg    *float64 `json:"maxSingleKg"`
		Estimated1RMKg *float64 `json:"estimated1RMKg"`
		Series         []struct {
			VolumeKg float64 `json:"volumeKg"`
		} `json:"series"`
		RecentSets []struct {
			WeightKg *float64 `json:"weightKg"`
			IsWarmup bool     `json:"isWarmup"`
		} `json:"recentSets"`
	}
	resp, raw = env.do(http.MethodGet, "/api/stats/exercises/"+itoa(bench.ID), nil, &stats)
	env.expect(resp, http.StatusOK, raw)
	if stats.MaxWeightKg == nil || *stats.MaxWeightKg != 90 {
		t.Fatalf("max weight = %v", stats.MaxWeightKg)
	}
	if stats.MaxSingleKg != nil {
		t.Fatalf("no single was performed, got %v", *stats.MaxSingleKg)
	}
	// 90 × (1 + 8/30) = 114
	if stats.Estimated1RMKg == nil || *stats.Estimated1RMKg != 114 {
		t.Fatalf("estimated 1RM = %v, want 114", stats.Estimated1RMKg)
	}
	if len(stats.Series) != 1 || stats.Series[0].VolumeKg != float64(wantVolume) {
		t.Fatalf("series = %+v", stats.Series)
	}
	// The history lists every completed set, the warmup included and flagged;
	// the total counts working sets only.
	if len(stats.RecentSets) != 5 {
		t.Fatalf("recent sets = %d, want 5", len(stats.RecentSets))
	}
	warmups := 0
	for _, s := range stats.RecentSets {
		if s.IsWarmup {
			warmups++
		}
	}
	if warmups != 1 {
		t.Fatalf("warmups in history = %d, want 1", warmups)
	}

	// 14. Deleting a referenced exercise is refused; archive is the way.
	resp, raw = env.do(http.MethodDelete, "/api/exercises/"+itoa(bench.ID), nil, nil)
	env.expect(resp, http.StatusConflict, raw)
	var archived exercise
	resp, raw = env.do(http.MethodPost, "/api/exercises/"+itoa(bench.ID)+"/archive", nil, &archived)
	env.expect(resp, http.StatusOK, raw)
	if !archived.IsArchived {
		t.Fatal("archive flag not set")
	}

	// 15. History still resolves the archived exercise's name.
	var detail sessionPayload
	resp, raw = env.do(http.MethodGet, "/api/sessions/"+itoa(session.ID), nil, &detail)
	env.expect(resp, http.StatusOK, raw)
	if len(detail.Exercises) != 1 || detail.Exercises[0].ExerciseName != "Жим штанги лёжа" {
		t.Fatalf("history lost the exercise: %+v", detail.Exercises)
	}
}

func TestTemplateCopiesIntoSessionAndDoesNotChangeHistory(t *testing.T) {
	env := newTestEnv(t)

	var groups struct {
		Items []muscleGroup `json:"items"`
	}
	_, raw := env.do(http.MethodGet, "/api/muscle-groups", nil, &groups)
	_ = raw
	chestID := findGroup(t, groups.Items, "Грудь")

	var squat exercise
	resp, raw := env.do(http.MethodPost, "/api/exercises", map[string]any{
		"name": "Приседания со штангой", "description": "",
		"muscleGroups": []map[string]any{{"muscleGroupId": chestID, "isPrimary": true}},
	}, &squat)
	env.expect(resp, http.StatusCreated, raw)

	sets := int32(3)
	reps := int32(5)
	weight := 100.0
	var tpl struct {
		ID        int64 `json:"id"`
		Exercises []struct {
			ExerciseID      int64    `json:"exerciseId"`
			PlannedSets     *int32   `json:"plannedSets"`
			PlannedWeightKg *float64 `json:"plannedWeightKg"`
		} `json:"exercises"`
	}
	resp, raw = env.do(http.MethodPost, "/api/templates", map[string]any{
		"name": "Ноги",
		"exercises": []map[string]any{{
			"exerciseId": squat.ID, "plannedSets": sets, "plannedReps": reps, "plannedWeightKg": weight,
		}},
	}, &tpl)
	env.expect(resp, http.StatusCreated, raw)

	var session sessionPayload
	resp, raw = env.do(http.MethodPost, "/api/templates/"+itoa(tpl.ID)+"/start", nil, &session)
	env.expect(resp, http.StatusCreated, raw)
	if session.Name != "Ноги" {
		t.Fatalf("session name = %s, want the template name", session.Name)
	}
	if len(session.Exercises) != 1 || len(session.Exercises[0].Sets) != 3 {
		t.Fatalf("planned sets not copied: %+v", session.Exercises)
	}
	planned := session.Exercises[0].Sets[0]
	if planned.IsCompleted {
		t.Fatal("planned sets must start uncompleted: they are a target, not a fact")
	}
	if planned.WeightKg == nil || *planned.WeightKg != 100 || planned.Repetitions != 5 {
		t.Fatalf("planned values not copied: %+v", planned)
	}

	// Complete the sets and finish.
	for i, s := range session.Exercises[0].Sets {
		path := "/api/sessions/" + itoa(session.ID) + "/exercises/" + itoa(session.Exercises[0].ID) + "/sets/" + itoa(s.ID)
		resp, raw = env.do(http.MethodPatch, path, map[string]any{"isCompleted": true}, nil)
		env.expect(resp, http.StatusOK, raw)
		_ = i
	}
	resp, raw = env.do(http.MethodPost, "/api/sessions/"+itoa(session.ID)+"/finish", nil, &session)
	env.expect(resp, http.StatusOK, raw)
	if session.VolumeKg != 1500 {
		t.Fatalf("volume = %v, want 1500", session.VolumeKg)
	}

	// Editing the template afterwards must not rewrite the finished workout.
	resp, raw = env.do(http.MethodPut, "/api/templates/"+itoa(tpl.ID), map[string]any{
		"name": "Ноги (обновлено)",
		"exercises": []map[string]any{{
			"exerciseId": squat.ID, "plannedSets": 5, "plannedReps": 10, "plannedWeightKg": 120,
		}},
	}, nil)
	env.expect(resp, http.StatusOK, raw)

	var after sessionPayload
	resp, raw = env.do(http.MethodGet, "/api/sessions/"+itoa(session.ID), nil, &after)
	env.expect(resp, http.StatusOK, raw)
	if after.VolumeKg != 1500 || len(after.Exercises[0].Sets) != 3 {
		t.Fatalf("template edit leaked into history: %+v", after)
	}

	// Deleting the template leaves the session intact (template_id becomes NULL).
	resp, raw = env.do(http.MethodDelete, "/api/templates/"+itoa(tpl.ID), nil, nil)
	env.expect(resp, http.StatusNoContent, raw)
	resp, raw = env.do(http.MethodGet, "/api/sessions/"+itoa(session.ID), nil, &after)
	env.expect(resp, http.StatusOK, raw)
	if len(after.Exercises) != 1 {
		t.Fatal("deleting a template removed history")
	}
}

func TestSetDeletionRenumbersContiguously(t *testing.T) {
	env := newTestEnv(t)
	var groups struct {
		Items []muscleGroup `json:"items"`
	}
	_, _ = env.do(http.MethodGet, "/api/muscle-groups", nil, &groups)
	chestID := findGroup(t, groups.Items, "Спина")

	var ex exercise
	resp, raw := env.do(http.MethodPost, "/api/exercises", map[string]any{
		"name":         "Подтягивания",
		"muscleGroups": []map[string]any{{"muscleGroupId": chestID, "isPrimary": true}},
	}, &ex)
	env.expect(resp, http.StatusCreated, raw)

	var session sessionPayload
	resp, raw = env.do(http.MethodPost, "/api/sessions", map[string]string{"name": "Спина"}, &session)
	env.expect(resp, http.StatusCreated, raw)
	var added struct {
		Exercise struct {
			ID int64 `json:"id"`
		} `json:"exercise"`
	}
	resp, raw = env.do(http.MethodPost, "/api/sessions/"+itoa(session.ID)+"/exercises", map[string]any{"exerciseId": ex.ID}, &added)
	env.expect(resp, http.StatusCreated, raw)

	base := "/api/sessions/" + itoa(session.ID) + "/exercises/" + itoa(added.Exercise.ID) + "/sets"
	for i := 0; i < 4; i++ {
		resp, raw = env.do(http.MethodPost, base, map[string]any{"repetitions": 10 - i}, nil)
		env.expect(resp, http.StatusCreated, raw)
	}
	// Remove the second set: the remaining ones must read 1,2,3 with no gap.
	resp, raw = env.do(http.MethodDelete, base+"/2", nil, nil)
	env.expect(resp, http.StatusNoContent, raw)

	var detail sessionPayload
	resp, raw = env.do(http.MethodGet, "/api/sessions/"+itoa(session.ID), nil, &detail)
	env.expect(resp, http.StatusOK, raw)
	got := detail.Exercises[0].Sets
	if len(got) != 3 {
		t.Fatalf("sets = %d, want 3", len(got))
	}
	for i, s := range got {
		if s.SetNumber != int32(i+1) {
			t.Fatalf("set %d has number %d", i, s.SetNumber)
		}
	}
}

func TestValidationAndErrorEnvelope(t *testing.T) {
	env := newTestEnv(t)

	var errBody struct {
		Error struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	resp, raw := env.do(http.MethodPost, "/api/muscle-groups", map[string]string{"name": "   "}, &errBody)
	env.expect(resp, http.StatusBadRequest, raw)
	if errBody.Error.Code != "validation_error" || errBody.Error.Details["name"] == "" {
		t.Fatalf("error envelope = %+v", errBody)
	}

	resp, raw = env.do(http.MethodGet, "/api/muscle-groups/999999", nil, &errBody)
	env.expect(resp, http.StatusNotFound, raw)
	if errBody.Error.Code != "not_found" {
		t.Fatalf("code = %s", errBody.Error.Code)
	}

	resp, raw = env.do(http.MethodPost, "/api/exercises", map[string]any{
		"name": "", "muscleGroups": []map[string]any{},
	}, &errBody)
	env.expect(resp, http.StatusBadRequest, raw)

	// An unknown JSON field is rejected instead of silently ignored.
	req, _ := http.NewRequest(http.MethodPost, env.server.URL+"/api/muscle-groups",
		bytesReader(`{"name":"x","typo":1}`))
	req.Header.Set("Content-Type", "application/json")
	rawResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := io.ReadAll(rawResp.Body)
	rawResp.Body.Close()
	env.expect(rawResp, http.StatusBadRequest, payload)
}

func TestMuscleGroupInUseCannotBeDeleted(t *testing.T) {
	env := newTestEnv(t)
	var groups struct {
		Items []muscleGroup `json:"items"`
	}
	_, _ = env.do(http.MethodGet, "/api/muscle-groups", nil, &groups)
	chestID := findGroup(t, groups.Items, "Грудь")

	var ex exercise
	resp, raw := env.do(http.MethodPost, "/api/exercises", map[string]any{
		"name":         "Разведение гантелей",
		"muscleGroups": []map[string]any{{"muscleGroupId": chestID, "isPrimary": true}},
	}, &ex)
	env.expect(resp, http.StatusCreated, raw)

	resp, raw = env.do(http.MethodDelete, "/api/muscle-groups/"+itoa(chestID), nil, nil)
	env.expect(resp, http.StatusConflict, raw)

	// Detach it, then deletion succeeds.
	resp, raw = env.do(http.MethodPatch, "/api/exercises/"+itoa(ex.ID), map[string]any{
		"name": "Разведение гантелей", "description": "", "equipment": "",
		"muscleGroups": []map[string]any{},
	}, nil)
	env.expect(resp, http.StatusOK, raw)

	resp, raw = env.do(http.MethodDelete, "/api/muscle-groups/"+itoa(chestID), nil, nil)
	env.expect(resp, http.StatusNoContent, raw)
}

// --- small helpers ---------------------------------------------------------

func findGroup(t *testing.T, items []muscleGroup, name string) int64 {
	t.Helper()
	for _, g := range items {
		if g.Name == name {
			return g.ID
		}
	}
	t.Fatalf("muscle group %q not found", name)
	return 0
}

func itoa(v int64) string { return fmt.Sprintf("%d", v) }
