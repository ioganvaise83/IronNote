package store

import (
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

// The schema stores weights as NUMERIC so sums stay exact. Converting at the
// store boundary keeps pgtypes out of the service and API layers.

// NumericToPtr converts a nullable numeric to a float pointer, preserving NULL.
func NumericToPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, _ := n.Float64Value()
	if !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

// numericToFloat converts a numeric that the query guarantees to be non-NULL.
func NumericToFloat(n pgtype.Numeric) float64 {
	if p := NumericToPtr(n); p != nil {
		return *p
	}
	return 0
}

// PtrToNumeric converts an optional float into a NUMERIC parameter.
func PtrToNumeric(v *float64) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	// Format with three decimals to match NUMERIC(7,3) exactly.
	s := big.NewFloat(*v).Text('f', 3)
	n := pgtype.Numeric{}
	_ = n.Scan(s)
	return n
}
