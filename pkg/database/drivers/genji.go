//go:build dragonfly || ios || freebsd || darwin || (linux && ppc64) || (linux && ppc64le) || (linux && s390x) || (linux && amd64) || (linux && mips64) || (linux && mips64le) || (linux && arm64) || android || (windows && amd64) || (windows && arm64)

package drivers

import (
	// Register the Genji driver when a binary explicitly opts into the
	// drivers package.  Tests can skip importing this package to stay fast.
	_ "github.com/genjidb/genji/driver"
)
