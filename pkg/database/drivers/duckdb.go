//go:build cgo && duckdb && (linux || darwin || windows) && (amd64 || arm64)

/*
#cgo linux LDFLAGS: -lstdc++ -static-libstdc++ -static-libgcc
*/

// DuckDB driver is enabled for Linux, macOS, and Windows so developers on the main
// desktop platforms can use the embedded engine while still keeping CGO explicit and
// predictable. We keep the architecture list to amd64/arm64 to avoid platform-specific
// surprises, following Go proverbs about clarity and explicitness.
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
//	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -tags duckdb
//	CGO_ENABLED=1 GOOS=windows GOARCH=arm64 go build -tags duckdb
//	go build -tags duckdb -o chicha-isotope-map
//
// Binaries that need DuckDB can import this package with the duckdb tag.
// This file lives outside the main build to keep CGO isolated and optional.
package drivers

import (
	_ "github.com/marcboeker/go-duckdb/v2"
)
