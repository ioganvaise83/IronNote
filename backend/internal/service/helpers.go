package service

import (
	"math"

	"github.com/ioganvaise83/ironnote/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// roundTo rounds to the given number of decimals, for display-ready numbers.
func roundTo(v float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(v*factor) / factor
}

// listDetailsParamsForPeriod builds the shared filter for statistics.
func listDetailsParamsForPeriod(userID int64, status string, p StatsPeriod) store.ListSessionDetailsParams {
	from := pgtype.Timestamptz{Time: p.From, Valid: !p.From.IsZero()}
	to := pgtype.Timestamptz{Time: p.To, Valid: !p.To.IsZero()}
	return store.ListSessionDetailsParams{
		UserID:      userID,
		Status:      &status,
		StartedFrom: from,
		StartedTo:   to,
	}
}
