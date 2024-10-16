//go:build ( ( netbsd && amd64 ) || ios || freebsd || darwin || ( linux && riscv64 ) || ( linux && ppc64le )  || ( linux && s390x ) || ( linux && amd64 ) || ( linux && arm64 ) || ( linux && 386 ) || android || ( openbsd && amd64 )|| ( openbsd && arm64 ) || windows )

package database

import ( _ "modernc.org/sqlite")
