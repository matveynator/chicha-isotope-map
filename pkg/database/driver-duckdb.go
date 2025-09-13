//go:build cgo && duckdb && ((linux && amd64) || (darwin && (amd64 || arm64)) || (windows && amd64))

// DuckDB driver is enabled on:
//   - Linux/amd64
//   - macOS/amd64, macOS/arm64
//   - Windows/amd64
// Requires build tag: -tags duckdb and CGO enabled.
// Linux/arm64 remains excluded.
// Build examples:
//   # Linux/macOS
//   CGO_ENABLED=1 go build -tags duckdb
//   # Windows (PowerShell)
//   $env:CGO_ENABLED="1"; go build -tags duckdb -o chicha-isotope-map.exe

package database

import (
	_ "github.com/marcboeker/go-duckdb"
)
