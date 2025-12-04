//go:build cgo && duckdb && linux && (amd64 || arm64)

// DuckDB driver is only enabled for Linux builds so cross compilation stays predictable
// and we avoid chasing platform-specific binary packages. This respects Go proverbs by
// keeping the surface small and explicit.
// Requires build tag: -tags duckdb and CGO enabled on Linux.
// Build examples:
//
//	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags duckdb
//	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -tags duckdb
//	go build -tags duckdb -o chicha-isotope-map
//
// Binaries that need DuckDB can import this package with the duckdb tag.
// This file lives outside the main build to keep CGO isolated and optional.
package drivers

import (
	_ "github.com/marcboeker/go-duckdb/v2"
)
