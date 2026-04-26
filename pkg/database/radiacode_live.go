package database

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// UpsertRadiacodeLivePoint keeps a single marker per coordinate and track while
// preserving the maximum dose and CPS values observed for that location.
//
// We intentionally keep the SQL portable (database/sql placeholders only), so
// the same logic works across SQLite, DuckDB, PostgreSQL, and ClickHouse-backed
// paths without driver-specific UPSERT syntax.
func (db *Database) UpsertRadiacodeLivePoint(ctx context.Context, marker Marker, epsilon float64) (Marker, error) {
	if db == nil || db.DB == nil {
		return Marker{}, fmt.Errorf("database unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if epsilon <= 0 {
		epsilon = 1e-6
	}
	if marker.TrackID == "" {
		marker.TrackID = fmt.Sprintf("radiacode-live-%d", time.Now().UTC().Unix())
	}
	if marker.Date <= 0 {
		marker.Date = time.Now().UTC().Unix()
	}
	marker.Zoom = 0

	var result Marker
	err := db.withSerializedConnectionFor(ctx, WorkloadRealtime, func(runCtx context.Context, conn *sql.DB) error {
		queryExisting := `
SELECT id, doseRate, countRate, radiation
FROM markers
WHERE trackID = ? AND zoom = 0
  AND lat BETWEEN ? AND ?
  AND lon BETWEEN ? AND ?
ORDER BY ABS(lat - ?) + ABS(lon - ?)
LIMIT 1`
		updateQuery := `
UPDATE markers
SET doseRate = ?, countRate = ?, date = ?, radiation = ?
WHERE id = ?`
		if db.Driver == "pgx" {
			queryExisting = `
SELECT id, doseRate, countRate, radiation
FROM markers
WHERE trackID = $1 AND zoom = 0
  AND lat BETWEEN $2 AND $3
  AND lon BETWEEN $4 AND $5
ORDER BY ABS(lat - $6) + ABS(lon - $7)
LIMIT 1`
			updateQuery = `
UPDATE markers
SET doseRate = $1, countRate = $2, date = $3, radiation = $4
WHERE id = $5`
		}
		row := conn.QueryRowContext(runCtx, queryExisting,
			marker.TrackID,
			marker.Lat-epsilon,
			marker.Lat+epsilon,
			marker.Lon-epsilon,
			marker.Lon+epsilon,
			marker.Lat,
			marker.Lon,
		)

		var existingID int64
		var existingDose float64
		var existingCount float64
		var existingRadiation sql.NullString
		err := row.Scan(&existingID, &existingDose, &existingCount, &existingRadiation)
		if err != nil {
			if err != sql.ErrNoRows {
				return fmt.Errorf("query existing radiacode point: %w", err)
			}
			if marker.ID == 0 {
				marker.ID = <-db.idGenerator
			}
			if saveErr := db.SaveMarkerAtomic(runCtx, conn, marker, db.Driver); saveErr != nil {
				return fmt.Errorf("insert radiacode point: %w", saveErr)
			}
			result = marker
			return nil
		}

		updatedDose := math.Max(existingDose, marker.DoseRate)
		updatedCount := math.Max(existingCount, marker.CountRate)
		updatedRadiation := strings.TrimSpace(marker.Radiation)
		if updatedRadiation == "" && existingRadiation.Valid {
			updatedRadiation = existingRadiation.String
		}

		_, err = conn.ExecContext(runCtx, updateQuery, updatedDose, updatedCount, marker.Date, updatedRadiation, existingID)
		if err != nil {
			return fmt.Errorf("update radiacode point: %w", err)
		}

		result = marker
		result.ID = existingID
		result.DoseRate = updatedDose
		result.CountRate = updatedCount
		result.Radiation = updatedRadiation
		return nil
	})
	if err != nil {
		return Marker{}, err
	}
	return result, nil
}
