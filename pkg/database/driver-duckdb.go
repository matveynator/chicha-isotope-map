//go:build cgo && duckdb && (linux || darwin || windows) && (amd64 || arm64)

// We register the DuckDB driver for database/sql behind build tags.
// This keeps the default builds CGO-free. Enable with: -tags duckdb
// Notes:
//   - DuckDB’s Go driver uses CGO to talk to the C/C++ engine.
//   - Supported placeholders: "?" and "$1"… (we use "?").
//   - ON CONFLICT DO NOTHING is supported in DuckDB.
//
// Build example:
//   CGO_ENABLED=1 go build -tags duckdb

package database

import (
	_ "github.com/marcboeker/go-duckdb"
)
