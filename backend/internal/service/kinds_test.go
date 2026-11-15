package service

import "testing"

// Timed exercises (бег, плавание, эллипсоид) have no weight and no repetitions.
// Bodyweight exercises have repetitions but no load. These tests pin the rules that
// distinguish them, because a regression here silently corrupts statistics.

func TestDurationSetHasTimeNoRepsNoVolume(t *testing.T) {
	set := Set{
		Kind: KindDuration, DurationSeconds: secs(1800),
		DistanceMeters: NewWeight(5000), IsCompleted: true,
	}

	if _, ok := set.Volume(); ok {
		t.Fatal("a timed set must not contribute to weight volume")
	}
	if _, ok := set.Epley1RM(); ok {
		t.Fatal("a timed set has no 1RM")
	}
	if set.IsActualSingle() {
		t.Fatal("a timed set is never an actual single")
	}

	seconds, ok := set.WorkSeconds()
	if !ok || seconds != 1800 {
		t.Fatalf("work seconds = %d, %v; want 1800", seconds, ok)
	}
	meters, ok := set.WorkDistance()
	if !ok || meters != 5000 {
		t.Fatalf("work distance = %v, %v; want 5000", meters, ok)
	}
}

func TestBodyweightSetCountsRepsButNoVolume(t *testing.T) {
	set := Set{Kind: KindBodyweight, Repetitions: reps16(12), IsCompleted: true}

	if _, ok := set.Volume(); ok {
		t.Fatal("bodyweight has no external load, so no volume")
	}
	if v := set.Reps(); v != 12 {
		t.Fatalf("reps = %d, want 12", v)
	}
	if _, ok := set.WorkSeconds(); ok {
		t.Fatal("a reps-based set has no duration")
	}
}

func TestWarmupTimedSetIsExcludedFromWorkTotals(t *testing.T) {
	warmup := Set{Kind: KindDuration, DurationSeconds: secs(600), IsWarmup: true, IsCompleted: true}
	if _, ok := warmup.WorkSeconds(); ok {
		t.Fatal("a warmup timed set must not count as work")
	}
}

func TestSummaryAddsCardioTimeAndDistance(t *testing.T) {
	session := Session{
		Exercises: []SessionExercise{
			{Kind: KindDuration, Sets: []Set{
				{Kind: KindDuration, DurationSeconds: secs(1800), DistanceMeters: NewWeight(5000), IsCompleted: true},
			}},
			{Kind: KindDuration, Sets: []Set{
				{Kind: KindDuration, DurationSeconds: secs(900), IsCompleted: true},
				{Kind: KindDuration, DurationSeconds: secs(300), IsWarmup: true, IsCompleted: true},
			}},
			{Kind: KindWeighted, Sets: []Set{
				{Kind: KindWeighted, WeightKg: NewWeight(100), Repetitions: reps16(5), IsCompleted: true},
			}},
		},
	}

	got := Summarize(session)
	if got.DurationSeconds != 2700 {
		t.Fatalf("cardio seconds = %d, want 2700 (warmup excluded)", got.DurationSeconds)
	}
	if got.DistanceMeters != 5000 {
		t.Fatalf("distance = %v, want 5000", got.DistanceMeters)
	}
	if got.VolumeKg != 500 {
		t.Fatalf("volume = %v, want 500 — cardio must not leak into volume", got.VolumeKg)
	}
	if got.CompletedSetCount != 1 {
		t.Fatalf("completed weighted sets = %d, want 1", got.CompletedSetCount)
	}
}

func TestHasRecordableWork(t *testing.T) {
	cases := []struct {
		name    string
		session Session
		want    bool
	}{
		{"no exercises", Session{}, false},
		{"exercise with no sets", Session{Exercises: []SessionExercise{{Kind: KindWeighted}}}, false},
		{"only uncompleted sets", Session{Exercises: []SessionExercise{{
			Kind: KindWeighted,
			Sets: []Set{{Kind: KindWeighted, WeightKg: NewWeight(80), Repetitions: reps16(10), IsCompleted: false}},
		}}}, false},
		{"only warmups", Session{Exercises: []SessionExercise{{
			Kind: KindWeighted,
			Sets: []Set{{Kind: KindWeighted, WeightKg: NewWeight(40), Repetitions: reps16(10), IsWarmup: true, IsCompleted: true}},
		}}}, false},
		{"one working set", Session{Exercises: []SessionExercise{{
			Kind: KindWeighted,
			Sets: []Set{{Kind: KindWeighted, WeightKg: NewWeight(80), Repetitions: reps16(10), IsCompleted: true}},
		}}}, true},
		{"bodyweight reps only", Session{Exercises: []SessionExercise{{
			Kind: KindBodyweight,
			Sets: []Set{{Kind: KindBodyweight, Repetitions: reps16(8), IsCompleted: true}},
		}}}, true},
		{"timed effort only", Session{Exercises: []SessionExercise{{
			Kind: KindDuration,
			Sets: []Set{{Kind: KindDuration, DurationSeconds: secs(1800), IsCompleted: true}},
		}}}, true},
		{"completed timed set with zero seconds", Session{Exercises: []SessionExercise{{
			Kind: KindDuration,
			Sets: []Set{{Kind: KindDuration, DurationSeconds: secs(0), IsCompleted: true}},
		}}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasRecordableWork(tc.session); got != tc.want {
				t.Fatalf("hasRecordableWork = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestKindParsing(t *testing.T) {
	for _, k := range []ExerciseKind{KindWeighted, KindBodyweight, KindDuration} {
		if got := ParseKind(string(k)); got != k {
			t.Fatalf("ParseKind(%q) = %q", k, got)
		}
	}
	if got := ParseKind("nonsense"); got != KindWeighted {
		t.Fatalf("unknown kind should fall back to weighted, got %q", got)
	}
}

func TestKindCapabilities(t *testing.T) {
	if !KindWeighted.UsesWeight() || !KindWeighted.UsesRepetitions() || KindWeighted.UsesDuration() {
		t.Fatal("weighted: weight + reps, no duration")
	}
	if KindBodyweight.UsesWeight() || !KindBodyweight.UsesRepetitions() {
		t.Fatal("bodyweight: reps only")
	}
	if !KindDuration.UsesDuration() || KindDuration.UsesRepetitions() || KindDuration.UsesWeight() {
		t.Fatal("duration: time only")
	}
}
