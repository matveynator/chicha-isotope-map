//go:build duckdb

// Package duckdb provides a stub driver so DuckDB-tagged builds stay reproducible
// even when the network proxy prevents fetching the official module. The code
// registers a minimal database/sql driver that clearly reports the missing engine
// rather than failing at link time. This follows Go proverbs by making failures
// explicit and keeping the build simple under adverse network conditions.
package duckdb

import (
	"database/sql"
	"database/sql/driver"
	"errors"
)

// stubDriver implements driver.Driver without touching CGO. We keep the
// implementation minimal because the goal is to preserve build determinism in
// environments where the real DuckDB engine cannot be fetched.
type stubDriver struct{}

// Open returns a clear error to call sites so they know the embedded engine is
// unavailable in this offline build.
func (stubDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("duckdb driver stub: offline build has no embedded engine")
}

func init() {
	// The registration mirrors the real driver name so callers do not need
	// to change imports when toggling between the stub and the upstream
	// implementation.
	sql.Register("duckdb", stubDriver{})
}
