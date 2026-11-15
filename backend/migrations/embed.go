// Package migrations is the single source of truth for the database schema.
//
// The SQL files are embedded so the API binary carries its schema and a running
// container needs no mounted directory. Keeping the .sql files next to this
// file means sqlc, the migration runner and a human reading the repo all look at
// exactly the same DDL.
package migrations

import "embed"

// FS holds the numbered migrations, applied in filename order.
//
//go:embed *.sql
var FS embed.FS
