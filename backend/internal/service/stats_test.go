package service

import (
	"math"
	"testing"
	"time"
)

func approx(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSetVolumeCountsOnlyCompletedWeightedWorkingSets(t *testing.T) {
	cases := []struct {
		name    string
		set     Set
		want    float64
		counted bool
	}{
		{"working set", Set{WeightKg: NewWeight(80), Repetitions: 10, IsCompleted: true}, 800, true},
		{"warmup excluded", Set{WeightKg: NewWeight(60), Repetitions: 10, IsWarmup: true, IsCompleted: true}, 0, false},
		{"incomplete excluded", Set{WeightKg: NewWeight(80), Repetitions: 10, IsCompleted: false}, 0, false},
		{"bodyweight excluded from volume", Set{Repetitions: 10, IsCompleted: true}, 0, false},
		{"fractional weight exact enough", Set{WeightKg: NewWeight(61.25), Repetitions: 8, IsCompleted: true}, 490, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, counted := tc.set.Volume()
			if counted != tc.counted {
				t.Fatalf("counted = %v, want %v", counted, tc.counted)
			}
			approx(t, got, tc.want)
		})
	}
}

func TestEpley1RM(t *testing.T) {
	// 100 kg × 5 → 100 × (1 + 5/30) = 116.666…
	got, ok := Set{WeightKg: NewWeight(100), Repetitions: 5, IsCompleted: true}.Epley1RM()
	if !ok {
		t.Fatal("expected an estimate")
	}
	if math.Abs(got-116.66666666666667) > 1e-9 {
		t.Fatalf("got %v", got)
	}

	// A true single is also an estimate of exactly its own weight.
	single, _ := Set{WeightKg: NewWeight(140), Repetitions: 1, IsCompleted: true}.Epley1RM()
	approx(t, single, 140*1.0333333333333332)

	// Warmups and unweighted sets never produce an estimate.
	if _, ok := (Set{WeightKg: NewWeight(100), Repetitions: 5, IsWarmup: true, IsCompleted: true}).Epley1RM(); ok {
		t.Fatal("warmup must not produce a 1RM estimate")
	}
	if _, ok := (Set{Repetitions: 5, IsCompleted: true}).Epley1RM(); ok {
		t.Fatal("bodyweight set must not produce a 1RM estimate")
	}
}

func TestActualSingleIsSeparatedFromEstimate(t *testing.T) {
	// 120×1 is an actual single; 100×8 has a bigger estimate (126.7) but is not
	// an actual max. The two must never be conflated.
	actual := Set{WeightKg: NewWeight(120), Repetitions: 1, IsCompleted: true}
	est := Set{WeightKg: NewWeight(100), Repetitions: 8, IsCompleted: true}

	if !actual.IsActualSingle() {
		t.Fatal("120×1 should be an actual single")
	}
	if est.IsActualSingle() {
		t.Fatal("100×8 is not an actual single")
	}
	estimated, _ := est.Epley1RM()
	if estimated <= 120 {
		t.Fatalf("expected estimate above the single, got %v", estimated)
	}
}

func TestSummarize(t *testing.T) {
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	finish := start.Add(75 * time.Minute)
	session := Session{
		ID: 1, Name: "Грудь", StartedAt: start, FinishedAt: &finish, Status: StatusCompleted,
		Exercises: []SessionExercise{
			{ExerciseID: 1, Sets: []Set{
				{WeightKg: NewWeight(60), Repetitions: 12, IsCompleted: true},                 // 720
				{WeightKg: NewWeight(80), Repetitions: 10, IsCompleted: true},                 // 800
				{WeightKg: NewWeight(90), Repetitions: 8, IsCompleted: true},                  // 720
				{WeightKg: NewWeight(40), Repetitions: 15, IsWarmup: true, IsCompleted: true}, // excluded
			}},
			{ExerciseID: 2, Sets: []Set{
				{WeightKg: NewWeight(12), Repetitions: 12, IsCompleted: true},  // 144
				{WeightKg: NewWeight(14), Repetitions: 10, IsCompleted: false}, // not done: excluded
			}},
			{ExerciseID: 3, Sets: nil}, // added but never trained: not counted as an exercise
		},
	}

	got := Summarize(session)
	if got.ExerciseCount != 2 {
		t.Fatalf("exercise count = %d, want 2", got.ExerciseCount)
	}
	approx(t, got.VolumeKg, 720+800+720+144)
	if got.CompletedSetCount != 4 {
		t.Fatalf("completed sets = %d, want 4", got.CompletedSetCount)
	}
	if got.DurationMinutes == nil {
		t.Fatal("expected a duration")
	}
	approx(t, *got.DurationMinutes, 75)
}

func TestSummarizeWithoutFinishHasNoDuration(t *testing.T) {
	got := Summarize(Session{StartedAt: time.Now(), Status: StatusInProgress})
	if got.DurationMinutes != nil {
		t.Fatal("in-progress workout must not report a duration")
	}
}

func TestMonthKeyUsesConfiguredTimezone(t *testing.T) {
	// 2026-03-01T01:00Z is still February in New York: the month bucket must
	// follow the user's timezone, not UTC.
	utc := time.Date(2026, 3, 1, 1, 0, 0, 0, time.UTC)
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("timezone database unavailable")
	}
	if got := monthKey(utc, time.UTC); got != "2026-03" {
		t.Fatalf("UTC month = %s", got)
	}
	if got := monthKey(utc, ny); got != "2026-02" {
		t.Fatalf("New York month = %s", got)
	}
}

func TestRoundKg(t *testing.T) {
	approx(t, roundKg(116.66666666666667), 116.67)
	approx(t, roundKg(490), 490)
}
