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
// The bundled driver now links against a system DuckDB shared library so hosts
// can upgrade to DuckDB v1.4.x without waiting for new static archives. Keeping
// the dynamic dependency mirrors Go's advice to trust the standard toolbox and
// makes cross-compiles leaner.

package drivers

import (
	// DuckDB stays behind an explicit build tag because it requires CGO.
	// Binaries that need DuckDB can import this package with the duckdb tag.
	_ "github.com/marcboeker/go-duckdb"
)
