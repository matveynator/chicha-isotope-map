//go:build duckdb

// The duckdb build tag is required for the DuckDB driver. This file marks the
// wizard as aware of DuckDB so it can expose the option only when compiled with
// the tag. Keeping the constant in a separate file mirrors Go's preference for
// clarity over cleverness.
package setupwizard

const duckDBBuilt = true
