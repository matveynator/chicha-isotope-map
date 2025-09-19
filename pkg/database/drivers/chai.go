//go:build dragonfly || ios || freebsd || darwin || (linux && ppc64) || (linux && ppc64le) || (linux && s390x) || (linux && amd64) || (linux && mips64) || (linux && mips64le) || (linux && arm64) || android || (windows && amd64) || (windows && arm64)

package drivers

import (
	"database/sql"
	"database/sql/driver"

	sqlite "modernc.org/sqlite"
)

// init wires up the "chai" driver name so callers can request it via database/sql.
// We reuse the modernc SQLite backend because Chai stores data in SQLite-compatible files,
// and sharing the implementation keeps the build simple and CGO-free.
func init() {
	sql.Register("chai", newChaiDriver())
}

// newChaiDriver returns a driver.Driver backed by modernc SQLite.
// Using a helper keeps the registration logic explicit and keeps the
// initialization testable in isolation.
func newChaiDriver() driver.Driver {
	return &sqlite.Driver{}
}
