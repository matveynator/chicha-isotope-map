//go:build !test

// This file wires in heavyweight SQL drivers only for production builds.
// go test/go vet exclude it via the build tag so command-line tooling stays
// responsive while keeping runtime behaviour unchanged for binaries.
package main

import "chicha-isotope-map/pkg/database/drivers"

func init() {
	// Touch the drivers package so its init functions register SQL
	// backends before the application opens database connections.
	drivers.Ready()
}
