package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/service"
)

// periodFromQuery resolves ?from/?to (dates or RFC 3339) into a half-open range.
// An omitted bound defaults to the widest sensible window rather than failing.
func (s *Server) periodFromQuery(r *http.Request) (service.StatsPeriod, error) {
	q := r.URL.Query()
	from, err := parseDate(q.Get("from"))
	if err != nil {
		return service.StatsPeriod{}, apperror.Validation("Некорректная дата в from", map[string]string{"from": q.Get("from")})
	}
	to, err := parseDate(q.Get("to"))
	if err != nil {
		return service.StatsPeriod{}, apperror.Validation("Некорректная дата в to", map[string]string{"to": q.Get("to")})
	}

	now := time.Now()
	period := service.StatsPeriod{}
	if from == nil {
		period.From = time.Date(2000, 1, 1, 0, 0, 0, 0, now.Location())
	} else {
		period.From = *from
	}
	if to == nil {
		period.To = now.AddDate(0, 0, 1)
	} else {
		// A bare date means "through the end of that day".
		if to.Hour() == 0 && to.Minute() == 0 && to.Second() == 0 {
			period.To = to.AddDate(0, 0, 1)
		} else {
			period.To = *to
		}
	}
	if !period.To.After(period.From) {
		return service.StatsPeriod{}, apperror.Validation("Конец периода должен быть позже начала", nil)
	}
	return period, nil
}

func (s *Server) handleStatsSummary(w http.ResponseWriter, r *http.Request) {
	period, err := s.periodFromQuery(r)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	summary, err := s.svc.SummaryStats(r.Context(), period)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":                   timeJSON(period.From),
		"to":                     timeJSON(period.To),
		"sessionCount":           summary.SessionCount,
		"completedSetCount":      summary.CompletedSetCount,
		"volumeKg":               summary.VolumeKg,
		"exerciseCount":          summary.ExerciseCount,
		"averageDurationMinutes": summary.AverageDurationMinutes,
		"monthly":                summary.Monthly,
	})
}

func (s *Server) handleExerciseBests(w http.ResponseWriter, r *http.Request) {
	period, err := s.periodFromQuery(r)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	items, err := s.svc.ExerciseBests(r.Context(), period)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":  timeJSON(period.From),
		"to":    timeJSON(period.To),
		"items": items,
	})
}

func (s *Server) handleExerciseStats(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	period, err := s.periodFromQuery(r)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	limit := int32(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeError(w, r, s.log, apperror.Validation("Некорректный limit", map[string]string{"limit": raw}))
			return
		}
		limit = int32(parsed)
	}
	stats, err := s.svc.ExerciseStatsFor(r.Context(), id, period, limit)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":           timeJSON(period.From),
		"to":             timeJSON(period.To),
		"exerciseId":     stats.ExerciseID,
		"exerciseName":   stats.ExerciseName,
		"series":         stats.Series,
		"maxWeightKg":    stats.MaxWeightKg,
		"maxSingleKg":    stats.MaxSingleKg,
		"estimated1RMKg": stats.Estimated1RMKg,
		"totalVolumeKg":  stats.TotalVolumeKg,
		"totalSets":      stats.TotalSets,
		"recentSets":     stats.RecentSets,
	})
}
