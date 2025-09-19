// Package drivers groups database/sql driver registrations so heavy
// dependencies stay out of lightweight go test/go vet runs unless a
// binary explicitly imports this package.
package drivers

// Ready is a no-op helper used by main packages to make the import
// explicit.  Calling Ready from init maintains clarity about why the
// package is pulled in while still avoiding mutexes or other side effects.
func Ready() {}
