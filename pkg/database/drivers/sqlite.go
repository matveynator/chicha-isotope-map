//go:build (netbsd && amd64) || ios || freebsd || darwin || (linux && riscv64) || (linux && ppc64le) || (linux && s390x) || (linux && amd64) || (linux && arm64) || (linux && 386) || android || (openbsd && amd64) || (openbsd && arm64)

package drivers

import (
	// Export the modernc SQLite driver so production binaries can opt in by
	// importing this lightweight package instead of pulling the dependency
	// into every test build.
	_ "modernc.org/sqlite"
)
