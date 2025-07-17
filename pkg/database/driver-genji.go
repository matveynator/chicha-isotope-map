//go:build dragonfly !386 && !arm && !armbe && !armle || ios || freebsd || darwin || (linux && ppc64) || (linux && ppc64le) || (linux && s390x) || (linux && amd64) || (linux && mips64) || (linux && mips64le) || (linux && arm64) || (linux && 386) || android || windows

package database

import (
	_ "github.com/genjidb/genji/driver"
)
