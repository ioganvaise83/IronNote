package service

import (
	"context"
	"sort"
	"time"

	"github.com/ioganvaise83/ironnote/internal/apperror"
)

// StatsPeriod is a half-open interval [From, To) in absolute time.
type StatsPeriod struct {
	From time.Time
	To   time.Time
}

// MonthlyPoint is one bar of the "sessions per month" chart.
type MonthlyPoint struct {
	Month        string `json:"month"`
	SessionCount int    `json:"sessionCount"`
}

// StatsSummary is the headline block of the statistics screen.
type StatsSummary struct {
	SessionCount           int            `json:"sessionCount"`
	CompletedSetCount      int            `json:"completedSetCount"`
	VolumeKg               float64        `json:"volumeKg"`
	ExerciseCount          int            `json:"exerciseCount"`
	AverageDurationMinutes float64        `json:"averageDurationMinutes"`
	Monthly                []MonthlyPoint `json:"monthly"`
}

// ExerciseBest is one row of the per-exercise leaderboard.
type ExerciseBest struct {
	ExerciseID        int64      `json:"exerciseId"`
	ExerciseName      string     `json:"exerciseName"`
	CompletedSetCount int        `json:"completedSetCount"`
	VolumeKg          float64    `json:"volumeKg"`
	MaxWeightKg       *float64   `json:"maxWeightKg"`
	MaxSingleKg       *float64   `json:"maxSingleKg"`
	Estimated1RMKg    *float64   `json:"estimated1RMKg"`
	BestSetVolumeKg   *float64   `json:"bestSetVolumeKg"`
	LastPerformedAt   *time.Time `json:"lastPerformedAt"`
}

// ExerciseSeriesPoint is one dot on an exercise progress chart.
type ExerciseSeriesPoint struct {
	SessionID      int64     `json:"sessionId"`
	Date           time.Time `json:"date"`
	VolumeKg       float64   `json:"volumeKg"`
	MaxWeightKg    *float64  `json:"maxWeightKg"`
	Estimated1RMKg *float64  `json:"estimated1RMKg"`
}

// ExerciseStats is the payload for the exercise statistics screen.
type ExerciseStats struct {
	ExerciseID     int64                 `json:"exerciseId"`
	ExerciseName   string                `json:"exerciseName"`
	Series         []ExerciseSeriesPoint `json:"series"`
	MaxWeightKg    *float64              `json:"maxWeightKg"`
	MaxSingleKg    *float64              `json:"maxSingleKg"`
	Estimated1RMKg *float64              `json:"estimated1RMKg"`
	TotalVolumeKg  float64               `json:"totalVolumeKg"`
	TotalSets      int                   `json:"totalSets"`
	RecentSets     []SetHistoryEntry     `json:"recentSets"`
}

// SetHistoryEntry is a performed set shown in the exercise detail list.
type SetHistoryEntry struct {
	SessionID   int64     `json:"sessionId"`
	Date        time.Time `json:"date"`
	SetNumber   int32     `json:"setNumber"`
	WeightKg    Weight    `json:"-"`
	WeightValue *float64  `json:"weightKg"`
	Repetitions int16     `json:"repetitions"`
	IsWarmup    bool      `json:"isWarmup"`
}

// ListSessionsForStats loads completed sessions in the period.
func (s *Service) periodSessions(ctx context.Context, p StatsPeriod) ([]Session, error) {
	status := StatusCompleted
	rows, err := s.q.ListSessionDetails(ctx, listDetailsParamsForPeriod(status, p))
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return groupSessionDetails(rows), nil
}

// SummaryStats aggregates the period. All maths comes from Summarize and Set,
// the same functions the history screen uses.
func (s *Service) SummaryStats(ctx context.Context, p StatsPeriod) (StatsSummary, error) {
	sessions, err := s.periodSessions(ctx, p)
	if err != nil {
		return StatsSummary{}, err
	}
	out := StatsSummary{Monthly: []MonthlyPoint{}}
	exercises := map[int64]bool{}
	var durationSum float64
	var durationCount int

	monthly := map[string]int{}
	for _, session := range sessions {
		summary := Summarize(session)
		out.SessionCount++
		out.CompletedSetCount += summary.CompletedSetCount
		out.VolumeKg += summary.VolumeKg
		for _, ex := range session.Exercises {
			if len(ex.Sets) > 0 {
				exercises[ex.ExerciseID] = true
			}
		}
		if summary.DurationMinutes != nil {
			durationSum += *summary.DurationMinutes
			durationCount++
		}
		monthly[monthKey(session.StartedAt, s.loc)]++
	}
	out.ExerciseCount = len(exercises)
	if durationCount > 0 {
		out.AverageDurationMinutes = roundTo(durationSum/float64(durationCount), 1)
	}

	months := make([]string, 0, len(monthly))
	for m := range monthly {
		months = append(months, m)
	}
	sort.Strings(months)
	for _, m := range months {
		out.Monthly = append(out.Monthly, MonthlyPoint{Month: m, SessionCount: monthly[m]})
	}
	return out, nil
}

// ExerciseBests returns one row per exercise performed in the period.
func (s *Service) ExerciseBests(ctx context.Context, p StatsPeriod) ([]ExerciseBest, error) {
	sessions, err := s.periodSessions(ctx, p)
	if err != nil {
		return nil, err
	}

	type acc struct {
		best ExerciseBest
	}
	accs := map[int64]*acc{}
	order := []int64{}

	for _, session := range sessions {
		for _, ex := range session.Exercises {
			a, ok := accs[ex.ExerciseID]
			if !ok {
				a = &acc{best: ExerciseBest{ExerciseID: ex.ExerciseID, ExerciseName: ex.ExerciseName}}
				accs[ex.ExerciseID] = a
				order = append(order, ex.ExerciseID)
			}
			started := session.StartedAt
			if a.best.LastPerformedAt == nil || started.After(*a.best.LastPerformedAt) {
				at := started
				a.best.LastPerformedAt = &at
			}
			for _, set := range ex.Sets {
				if !set.IsCompleted {
					continue
				}
				a.best.CompletedSetCount++
				if volume, counted := set.Volume(); counted {
					a.best.VolumeKg += volume
					if a.best.BestSetVolumeKg == nil || volume > *a.best.BestSetVolumeKg {
						v := volume
						a.best.BestSetVolumeKg = &v
					}
					a.best.MaxWeightKg = maxPtr(a.best.MaxWeightKg, set.WeightKg.Float())
					if rm, ok := set.Epley1RM(); ok {
						a.best.Estimated1RMKg = maxPtr(a.best.Estimated1RMKg, rm)
					}
				}
				if set.IsActualSingle() {
					a.best.MaxSingleKg = maxPtr(a.best.MaxSingleKg, set.WeightKg.Float())
				}
			}
		}
	}

	out := make([]ExerciseBest, 0, len(order))
	for _, id := range order {
		b := accs[id].best
		b.VolumeKg = roundKg(b.VolumeKg)
		b.MaxWeightKg = roundPtr(b.MaxWeightKg)
		b.MaxSingleKg = roundPtr(b.MaxSingleKg)
		b.Estimated1RMKg = roundPtr(b.Estimated1RMKg)
		b.BestSetVolumeKg = roundPtr(b.BestSetVolumeKg)
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExerciseName < out[j].ExerciseName })
	return out, nil
}

// ExerciseStatsFor returns the series and records for one exercise. The series
// is chronological, so the chart can plot it directly.
func (s *Service) ExerciseStatsFor(ctx context.Context, exerciseID int64, p StatsPeriod, historyLimit int32) (ExerciseStats, error) {
	ex, err := s.GetExercise(ctx, exerciseID)
	if err != nil {
		return ExerciseStats{}, err
	}
	sessions, err := s.periodSessions(ctx, p)
	if err != nil {
		return ExerciseStats{}, err
	}
	if historyLimit <= 0 || historyLimit > 500 {
		historyLimit = 100
	}

	out := ExerciseStats{ExerciseID: ex.ID, ExerciseName: ex.Name, Series: []ExerciseSeriesPoint{}, RecentSets: []SetHistoryEntry{}}
	for _, session := range sessions {
		for _, sessionExercise := range session.Exercises {
			if sessionExercise.ExerciseID != exerciseID {
				continue
			}
			point := ExerciseSeriesPoint{SessionID: session.ID, Date: session.StartedAt}
			for _, set := range sessionExercise.Sets {
				if !set.IsCompleted {
					continue
				}
				// Warmups stay visible in the set history (flagged as such) but are
				// excluded from the headline totals, as required by default.
				if !set.IsWarmup {
					out.TotalSets++
				}
				out.RecentSets = append(out.RecentSets, SetHistoryEntry{
					SessionID: session.ID, Date: session.StartedAt, SetNumber: set.SetNumber,
					WeightKg: set.WeightKg, WeightValue: set.WeightKg.Ptr(),
					Repetitions: set.Repetitions, IsWarmup: set.IsWarmup,
				})
				if volume, counted := set.Volume(); counted {
					point.VolumeKg += volume
					out.TotalVolumeKg += volume
					point.MaxWeightKg = maxPtr(point.MaxWeightKg, set.WeightKg.Float())
					out.MaxWeightKg = maxPtr(out.MaxWeightKg, set.WeightKg.Float())
					if rm, ok := set.Epley1RM(); ok {
						point.Estimated1RMKg = maxPtr(point.Estimated1RMKg, rm)
						out.Estimated1RMKg = maxPtr(out.Estimated1RMKg, rm)
					}
				}
				if set.IsActualSingle() {
					out.MaxSingleKg = maxPtr(out.MaxSingleKg, set.WeightKg.Float())
				}
			}
			out.Series = append(out.Series, point)
			break
		}
	}

	sort.Slice(out.RecentSets, func(i, j int) bool {
		if out.RecentSets[i].Date.Equal(out.RecentSets[j].Date) {
			return out.RecentSets[i].SetNumber > out.RecentSets[j].SetNumber
		}
		return out.RecentSets[i].Date.After(out.RecentSets[j].Date)
	})
	if len(out.RecentSets) > int(historyLimit) {
		out.RecentSets = out.RecentSets[:historyLimit]
	}

	out.TotalVolumeKg = roundKg(out.TotalVolumeKg)
	out.MaxWeightKg = roundPtr(out.MaxWeightKg)
	out.MaxSingleKg = roundPtr(out.MaxSingleKg)
	out.Estimated1RMKg = roundPtr(out.Estimated1RMKg)
	return out, nil
}

func monthKey(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01")
}

func maxPtr(current *float64, candidate float64) *float64 {
	if current != nil && *current >= candidate {
		return current
	}
	v := candidate
	return &v
}

func roundKg(v float64) float64 { return roundTo(v, 2) }

func roundPtr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := roundKg(*p)
	return &v
}
