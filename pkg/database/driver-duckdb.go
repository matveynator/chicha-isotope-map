//go:build cgo && duckdb && ((linux && amd64) || (darwin && (amd64 || arm64)))

// DuckDB driver is enabled ONLY on:
//   - Linux/amd64
//   - macOS/amd64, macOS/arm64
// Requires build tag: -tags duckdb
// Windows and Linux/arm64 are intentionally excluded.
// Build example:
//   CGO_ENABLED=1 go build -tags duckdb

package database

import (
	_ "github.com/marcboeker/go-duckdb"
)
