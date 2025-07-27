//go:build (dragonfly && amd64) || (openbsd && amd64) || (openbsd && arm64) || (openbsd && mips64) || (netbsd && amd64) || (netbsd && arm64) || freebsd || darwin || (linux && ppc64) || (linux && ppc64le) || (linux && s390x) || (linux && amd64) || (linux && mips64) || (linux && mips64le) || (linux && arm64) || (linux && 386) || (linux && riscv64) || android || (windows && amd64) || (windows && arm64) || (aix && ppc64) || (illumos && amd64) || (solaris && amd64) || (plan9 && amd64)

package database

import (
	_ "github.com/jackc/pgx/v5/stdlib"
)
