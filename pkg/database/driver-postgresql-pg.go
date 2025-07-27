//go:build (windows && 386) || (windows && amd64) || (windows && arm64)

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
