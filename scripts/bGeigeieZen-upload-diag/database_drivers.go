//go:build !test

// Wiring SQL drivers via a dedicated file keeps go test/go vet nimble while
// command binaries still register the backends they rely on.
package main

import "chicha-isotope-map/pkg/database/drivers"

func init() {
	// Ensure driver registrations run before the diagnostics tool touches
	// database/sql.  Using a function call keeps the import intentional.
	drivers.Ready()
}
