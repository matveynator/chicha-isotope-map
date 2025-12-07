//go:build cgo && duckdb && (linux || darwin) && (amd64 || arm64)

/*
#cgo linux LDFLAGS: -lstdc++ -static-libstdc++ -static-libgcc
*/

// DuckDB driver is enabled for Linux and macOS because those platforms ship official
// binaries for the requested DuckDB v1.4.2 / duckdb-go v2.5.2 pairing. We keep the
// architecture list to amd64/arm64 to avoid platform-specific surprises, following
// Go proverbs about clarity and explicitness.
// The linux-specific cgo LDFLAGS above force in libstdc++ and libgcc statically to satisfy
// DuckDB's C++ helpers that are not pulled in automatically when Go drives the linker.
// This keeps the user-facing build steps minimal while preventing missing symbol errors
// like std::__throw_bad_array_new_length when building DuckDB-enabled binaries.
// Requires build tag: -tags duckdb and CGO enabled on supported platforms.
// Build examples:
//
//	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags duckdb
//	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -tags duckdb
//	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -tags duckdb
//	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -tags duckdb
//	go build -tags duckdb -o chicha-isotope-map
//
// Binaries that need DuckDB can import this package with the duckdb tag so the official
// duckdb-go driver registers itself. We keep this file isolated to keep CGO optional
// for builds that do not embed DuckDB at all.
package drivers

import (
        // We pull in the official duckdb-go driver directly so DuckDB v1.4.2 stays in sync
        // with duckdb-go v2.5.2. The underscore import ensures the driver registers with
        // database/sql while keeping this file small.
        _ "github.com/duckdb/duckdb-go/v2"
)
