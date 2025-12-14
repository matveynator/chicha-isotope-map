//go:build !duckdb

// This file keeps the wizard honest when DuckDB support is not compiled in. By
// setting the flag to false at build time, the wizard hides DuckDB from the
// database list so operators only see engines their binary can run.
package setupwizard

const duckDBBuilt = false
