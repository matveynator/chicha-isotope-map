package database

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
)

// =====================
// Track metadata helpers
// =====================

// StreamTrackSummaries streams metadata about tracks ordered by their ID.
// We keep the DB scan in its own goroutine so callers can consume the
// channel incrementally without blocking the main HTTP handlers.
func (db *Database) StreamTrackSummaries(
	ctx context.Context,
	startAfter string,
	limit int,
	dbType string,
) (<-chan TrackSummary, <-chan error) {
	results := make(chan TrackSummary)
	errs := make(chan error, 1)

	go func() {
		defer close(results)
		defer close(errs)

		if limit <= 0 {
			// Default to a conservative page so naive clients do not
			// accidentally request millions of rows.
			limit = 100
		}

		query := `SELECT trackID, MIN(id) AS first_id, MAX(id) AS last_id, COUNT(*) AS marker_count
FROM markers
WHERE trackID > %s
GROUP BY trackID
ORDER BY trackID
LIMIT %s;`

		placeholderGreater := "?"
		placeholderLimit := "?"

		switch strings.ToLower(dbType) {
		case "pgx":
			placeholderGreater = "$1"
			placeholderLimit = "$2"
		default:
			// SQLite/Chai/DuckDB share the question-mark syntax.
		}

		query = fmt.Sprintf(query, placeholderGreater, placeholderLimit)

		rows, err := db.DB.QueryContext(ctx, query, startAfter, limit)
		if err != nil {
			errs <- fmt.Errorf("list tracks: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var summary TrackSummary
			if err := rows.Scan(&summary.TrackID, &summary.FirstID, &summary.LastID, &summary.MarkerCount); err != nil {
				errs <- fmt.Errorf("scan track summary: %w", err)
				return
			}

			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case results <- summary:
			}
		}

		if err := rows.Err(); err != nil {
			errs <- fmt.Errorf("iterate track summaries: %w", err)
			return
		}

		errs <- nil
	}()

	return results, errs
}

// CountTracks returns the total number of distinct track IDs.
// The API layer uses this to hint clients about the upper bound of the
// pagination sequence so they can plan how many requests to issue.
func (db *Database) CountTracks(ctx context.Context) (int64, error) {
	row := db.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT trackID) FROM markers`)
	var count sql.NullInt64
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count tracks: %w", err)
	}
	if !count.Valid {
		return 0, nil
	}
	return count.Int64, nil
}

// GetTrackSummary returns metadata for a single track.
// Keeping this function tiny lets the HTTP handler reuse the information
// for range validation without duplicating SQL statements.
func (db *Database) GetTrackSummary(ctx context.Context, trackID, dbType string) (TrackSummary, error) {
	summary := TrackSummary{TrackID: trackID}
	query := `SELECT MIN(id) AS first_id, MAX(id) AS last_id, COUNT(*) AS marker_count
FROM markers
WHERE trackID = %s;`

	placeholder := "?"
	if strings.ToLower(dbType) == "pgx" {
		placeholder = "$1"
	}

	query = fmt.Sprintf(query, placeholder)

	row := db.DB.QueryRowContext(ctx, query, trackID)
	if err := row.Scan(&summary.FirstID, &summary.LastID, &summary.MarkerCount); err != nil {
		if err == sql.ErrNoRows {
			return summary, nil
		}
		return summary, fmt.Errorf("track summary: %w", err)
	}
	return summary, nil
}

// =========================
// Marker range streaming API
// =========================

// StreamMarkersByTrackRange streams markers by track ID and ID range.
// We limit the dataset inside SQL to keep the result bounded and forward
// rows through a channel so callers can encode them progressively.
func (db *Database) StreamMarkersByTrackRange(
	ctx context.Context,
	trackID string,
	fromID int64,
	toID int64,
	limit int,
	dbType string,
) (<-chan Marker, <-chan error) {
	out := make(chan Marker)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)

		if limit <= 0 {
			limit = 1000
		}
		if toID <= 0 || toID < fromID {
			toID = math.MaxInt64
		}

		query := `SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
FROM markers
WHERE trackID = %s AND id >= %s AND id <= %s
ORDER BY id
LIMIT %s;`

		placeholders := []string{"?", "?", "?", "?"}
		if strings.ToLower(dbType) == "pgx" {
			placeholders = []string{"$1", "$2", "$3", "$4"}
		}

		query = fmt.Sprintf(query, placeholders[0], placeholders[1], placeholders[2], placeholders[3])

		rows, err := db.DB.QueryContext(ctx, query, trackID, fromID, toID, limit)
		if err != nil {
			errs <- fmt.Errorf("stream markers: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var m Marker
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID); err != nil {
				errs <- fmt.Errorf("scan marker: %w", err)
				return
			}
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case out <- m:
			}
		}

		if err := rows.Err(); err != nil {
			errs <- fmt.Errorf("iterate markers: %w", err)
			return
		}

		errs <- nil
	}()

	return out, errs
}
