//go:build cgo

package duckdb

/*
#cgo LDFLAGS: -lduckdb
#include <duckdb.h>
*/
import "C"

// The driver links against a system-provided DuckDB shared library so we can
// adopt newer engine releases (such as v1.4.x) without shipping static archives.
// Using the shared object keeps rebuilds lean and lets operators upgrade the
// underlying engine independently. This follows the Go proverb that a little
// copying is better than a little dependency on bundled artifacts.
