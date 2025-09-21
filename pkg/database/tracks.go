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
// We delegate to the shared streamTrackSummaries helper so future filters
// (year/month) reuse the same channel-based plumbing.
func (db *Database) StreamTrackSummaries(
	ctx context.Context,
	startAfter string,
	limit int,
	dbType string,
) (<-chan TrackSummary, <-chan error) {
	return db.streamTrackSummaries(ctx, startAfter, limit, dbType, false, 0, 0)
}

// StreamTrackSummariesByDateRange restricts tracks to a time window.
// We expose it for the year/month API variants so they can reuse the
// streaming pattern without duplicating SQL logic.
func (db *Database) StreamTrackSummariesByDateRange(
	ctx context.Context,
	startAfter string,
	limit int,
	from int64,
	to int64,
	dbType string,
) (<-chan TrackSummary, <-chan error) {
	return db.streamTrackSummaries(ctx, startAfter, limit, dbType, true, from, to)
}

// streamTrackSummaries performs the actual query and pushes rows over
// channels so handlers can encode responses progressively.
func (db *Database) streamTrackSummaries(
	ctx context.Context,
	startAfter string,
	limit int,
	dbType string,
	restrictDates bool,
	from int64,
	to int64,
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

		nextPlaceholder := newPlaceholderGenerator(dbType)
		conditions := []string{fmt.Sprintf("trackID > %s", nextPlaceholder())}
		args := []any{startAfter}

		if restrictDates {
			// The API provides inclusive start and exclusive end boundaries
			// so date math stays consistent with Go's time package.
			conditions = append(conditions, fmt.Sprintf("date >= %s", nextPlaceholder()))
			args = append(args, from)
			conditions = append(conditions, fmt.Sprintf("date < %s", nextPlaceholder()))
			args = append(args, to)
		}

		limitPlaceholder := nextPlaceholder()
		args = append(args, limit)

		query := fmt.Sprintf(`SELECT trackID, MIN(id) AS first_id, MAX(id) AS last_id, COUNT(*) AS marker_count
FROM markers
WHERE %s
GROUP BY trackID
ORDER BY trackID
LIMIT %s;`, strings.Join(conditions, " AND "), limitPlaceholder)

		rows, err := db.DB.QueryContext(ctx, query, args...)
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

		query := `SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
       COALESCE(altitude, 0) AS altitude,
       COALESCE(detector, '') AS detector,
       COALESCE(radiation, '') AS radiation,
       COALESCE(temperature, 0) AS temperature,
       COALESCE(humidity, 0) AS humidity
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
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID,
				&m.Altitude, &m.Detector, &m.Radiation, &m.Temperature, &m.Humidity); err != nil {
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

// CountTrackIDsUpTo returns how many distinct track IDs are lexicographically
// less than or equal to the provided ID. We use it to translate string track
// IDs into stable numeric indices for the API.
func (db *Database) CountTrackIDsUpTo(ctx context.Context, trackID, dbType string) (int64, error) {
	if strings.TrimSpace(trackID) == "" {
		return 0, nil
	}

	nextPlaceholder := newPlaceholderGenerator(dbType)
	where := fmt.Sprintf("trackID <= %s", nextPlaceholder())
	query := fmt.Sprintf(`SELECT COUNT(*) FROM (SELECT DISTINCT trackID FROM markers WHERE %s) AS sub;`, where)

	row := db.DB.QueryRowContext(ctx, query, trackID)
	var count sql.NullInt64
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count track ids up to: %w", err)
	}
	if !count.Valid {
		return 0, nil
	}
	return count.Int64, nil
}

// GetTrackIDByIndex resolves a 1-based numeric index to the actual track ID.
// Returning an empty string keeps HTTP handlers free to decide how to map it
// to status codes.
func (db *Database) GetTrackIDByIndex(ctx context.Context, index int64, dbType string) (string, error) {
	if index <= 0 {
		return "", fmt.Errorf("index must be positive")
	}

	nextPlaceholder := newPlaceholderGenerator(dbType)
	offsetPlaceholder := nextPlaceholder()
	query := fmt.Sprintf(`SELECT trackID FROM (SELECT DISTINCT trackID FROM markers ORDER BY trackID LIMIT 1 OFFSET %s) AS sub;`, offsetPlaceholder)

	row := db.DB.QueryRowContext(ctx, query, index-1)
	var trackID string
	if err := row.Scan(&trackID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("track id by index: %w", err)
	}
	return trackID, nil
}

// CountTracksInRange reports how many distinct tracks contain markers inside
// the provided date window. Handlers expose the number so API users know how
// many pages exist for the requested period.
func (db *Database) CountTracksInRange(ctx context.Context, from, to int64, dbType string) (int64, error) {
	nextPlaceholder := newPlaceholderGenerator(dbType)
	condFrom := fmt.Sprintf("date >= %s", nextPlaceholder())
	condTo := fmt.Sprintf("date < %s", nextPlaceholder())
	query := fmt.Sprintf(`SELECT COUNT(*) FROM (SELECT DISTINCT trackID FROM markers WHERE %s AND %s) AS sub;`, condFrom, condTo)

	row := db.DB.QueryRowContext(ctx, query, from, to)
	var count sql.NullInt64
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count tracks in range: %w", err)
	}
	if !count.Valid {
		return 0, nil
	}
	return count.Int64, nil
}

// newPlaceholderGenerator returns a closure that produces the correct
// placeholder syntax for the configured driver. Using a generator keeps the
// SQL assembly readable even as the number of filters grows.
func newPlaceholderGenerator(dbType string) func() string {
	if strings.ToLower(dbType) == "pgx" {
		counter := 0
		return func() string {
			counter++
			return fmt.Sprintf("$%d", counter)
		}
	}
	return func() string { return "?" }
}
