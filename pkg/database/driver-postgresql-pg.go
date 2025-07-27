// driver-postgresql-pq386.go
//go:build windows && 386
// +build windows,386

package database

import (
	"database/sql"
	"github.com/lib/pq"
)

// На Win-x86 регистрируем lib/pq под тем же именем "pgx",
// поэтому остальной код и CLI-флаг -db-type=pgx менять не нужно.
func init() {
	sql.Register("pgx", &pq.Driver{})
}
