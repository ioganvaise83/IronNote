// Package service holds IronNote's business rules.
//
// The rules that must never disagree between screens live here as pure
// functions over plain structs: volume, set counts, and the two kinds of 1RM.
// Handlers deal only with HTTP; the store deals only with SQL.
package service

import (
	"time"

	"github.com/ioganvaise83/ironnote/internal/store"
)

// Weight is a kilogram value with the precision the schema stores.
// It is a decimal string in JSON so that 1.25 kg never becomes 1.2499999.
type Weight struct {
	value float64
	set   bool
}

func NewWeight(v float64) Weight { return Weight{value: v, set: true} }
func WeightFromPtr(v *float64) Weight {
	if v == nil {
		return Weight{}
	}
	return Weight{value: *v, set: true}
}
func (w Weight) IsSet() bool    { return w.set }
func (w Weight) Float() float64 { return w.value }
func (w Weight) Ptr() *float64 {
	if !w.set {
		return nil
	}
	v := w.value
	return &v
}

// MuscleGroup is a reference entry ("Грудь", "Спина").
type MuscleGroup struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ExerciseGroup links an exercise to one muscle group it trains.
type ExerciseGroup struct {
	MuscleGroupID   int64
	MuscleGroupName string
	IsPrimary       bool
}

// Exercise is a catalogue entry. Archived exercises stay resolvable so old
// workouts keep rendering their names.
type Exercise struct {
	ID           int64
	Name         string
	Description  string
	Equipment    string
	IsArchived   bool
	MuscleGroups []ExerciseGroup
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TemplateExercise is one planned movement inside a template.
type TemplateExercise struct {
	ID               int64
	ExerciseID       int64
	ExerciseName     string
	ExerciseArchived bool
	SortOrder        int32
	PlannedSets      *int32
	PlannedReps      *int32
	PlannedWeightKg  Weight
}

// Template is a reusable plan. It is copied into a session when that session
// starts, so editing a template never rewrites history.
type Template struct {
	ID          int64
	Name        string
	Description string
	Exercises   []TemplateExercise
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TemplateSummary is the list projection: no exercises, with counters.
type TemplateSummary struct {
	ID            int64
	Name          string
	Description   string
	ExerciseCount int64
	PlannedSets   int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Set is one performed set.
type Set struct {
	ID          int64
	SetNumber   int32
	WeightKg    Weight
	Repetitions int16
	IsWarmup    bool
	IsCompleted bool
}

// Volume returns weight × reps for a working, completed, weighted set.
// Returns (0, false) when the set does not count toward training volume.
func (s Set) Volume() (float64, bool) {
	if !s.IsCompleted || s.IsWarmup || !s.WeightKg.IsSet() {
		return 0, false
	}
	return s.WeightKg.Float() * float64(s.Repetitions), true
}

// Epley1RM returns the estimated one-rep max for a set: weight × (1 + reps/30).
// Estimated only: it is never reported as an actual max, which requires reps == 1.
// Warmups are excluded — a light warmup must never look like a record attempt.
func (s Set) Epley1RM() (float64, bool) {
	if !s.IsCompleted || s.IsWarmup || !s.WeightKg.IsSet() || s.Repetitions <= 0 {
		return 0, false
	}
	return s.WeightKg.Float() * (1 + float64(s.Repetitions)/30.0), true
}

// IsActualSingle reports a genuinely performed single at that weight.
func (s Set) IsActualSingle() bool {
	return s.IsCompleted && !s.IsWarmup && s.WeightKg.IsSet() && s.Repetitions == 1
}

// SessionExercise is one exercise inside a workout, with its sets.
type SessionExercise struct {
	ID               int64
	ExerciseID       int64
	ExerciseName     string
	Equipment        string
	ExerciseArchived bool
	SortOrder        int32
	Notes            string
	Sets             []Set
	// PreviousSets is "last time" for the same movement, for prefill and context.
	PreviousSets []Set
	PreviousAt   *time.Time
}

// Session is a workout. StartedAt/FinishedAt are instants.
type Session struct {
	ID         int64
	TemplateID *int64
	Name       string
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     string
	Notes      string
	Exercises  []SessionExercise
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const (
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
)

// SessionSummary is the history list row. Counters are derived from the sets,
// never stored, so they cannot go stale.
type SessionSummary struct {
	ID                int64
	TemplateID        *int64
	Name              string
	StartedAt         time.Time
	FinishedAt        *time.Time
	Status            string
	Notes             string
	ExerciseCount     int
	CompletedSetCount int
	VolumeKg          float64
	DurationMinutes   *float64
}

// Summarize derives the list row from a full session.
func Summarize(s Session) SessionSummary {
	summary := SessionSummary{
		ID:         s.ID,
		TemplateID: s.TemplateID,
		Name:       s.Name,
		StartedAt:  s.StartedAt,
		FinishedAt: s.FinishedAt,
		Status:     s.Status,
		Notes:      s.Notes,
	}
	for _, ex := range s.Exercises {
		if len(ex.Sets) > 0 {
			summary.ExerciseCount++
		}
		for _, set := range ex.Sets {
			if v, ok := set.Volume(); ok {
				summary.VolumeKg += v
				summary.CompletedSetCount++
			}
		}
	}
	if s.FinishedAt != nil {
		// One decimal is enough for "1 ч 15 мин"; the raw value would otherwise
		// surface as 0.00574995 in JSON when a workout is ended immediately.
		mins := roundTo(s.FinishedAt.Sub(s.StartedAt).Minutes(), 1)
		if mins >= 0 {
			summary.DurationMinutes = &mins
		}
	}
	return summary
}

// TouchOf converts generated rows into domain values.

func MuscleGroupFromRow(r store.MuscleGroup) MuscleGroup {
	return MuscleGroup{
		ID:          r.ID,
		Name:        r.Name,
		Description: derefString(r.Description),
		CreatedAt:   r.CreatedAt.Time,
		UpdatedAt:   r.UpdatedAt.Time,
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
