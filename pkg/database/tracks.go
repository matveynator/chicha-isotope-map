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

		limitClause := ""
		if limit > 0 {
			limitClause = fmt.Sprintf(" LIMIT %s", nextPlaceholder())
			args = append(args, limit)
		}

		query := fmt.Sprintf(`SELECT trackID, MIN(id) AS first_id, MAX(id) AS last_id, COUNT(*) AS marker_count
FROM markers
WHERE %s
GROUP BY trackID
ORDER BY trackID%s;`, strings.Join(conditions, " AND "), limitClause)

		rows, err := db.DB.QueryContext(ctx, query, args...)
		if err != nil {
			errs <- fmt.Errorf("list tracks: %w", err)
			return
		}
		defer rows.Close()

		// We read the entire page before emitting results so we can compute the
		// starting index once. This avoids hammering PostgreSQL with COUNT(DISTINCT)
		// calls for every single track, which previously spiked CPU during archive
		// creation. The buffered slice stays small because callers already cap
		// page sizes.
		capHint := limit
		if capHint <= 0 {
			capHint = 1024
		}
		summaries := make([]TrackSummary, 0, capHint)
		for rows.Next() {
			var summary TrackSummary
			if err := rows.Scan(&summary.TrackID, &summary.FirstID, &summary.LastID, &summary.MarkerCount); err != nil {
				errs <- fmt.Errorf("scan track summary: %w", err)
				return
			}
			summaries = append(summaries, summary)
		}

		if err := rows.Err(); err != nil {
			errs <- fmt.Errorf("iterate track summaries: %w", err)
			return
		}

		if len(summaries) == 0 {
			errs <- nil
			return
		}

		trimmed := strings.TrimSpace(startAfter)
		baseIndex := int64(0)
		haveBase := false

		if trimmed != "" {
			// When resuming from a known track we only need its index once
			// per page. Subsequent tracks increment locally without extra SQL.
			var count int64
			if count, err = db.CountTrackIDsUpTo(ctx, trimmed, dbType); err != nil {
				errs <- fmt.Errorf("count track ids base: %w", err)
				return
			}
			baseIndex = count
			haveBase = true
		}

		if !haveBase {
			// For the very first page we derive the base from the first row.
			// Using a second query here is still cheaper than doing it per track
			// and the buffer keeps the connection free before the next query.
			var firstCount int64
			if firstCount, err = db.CountTrackIDsUpTo(ctx, summaries[0].TrackID, dbType); err != nil {
				errs <- fmt.Errorf("count track ids first page: %w", err)
				return
			}
			baseIndex = firstCount - 1
			if baseIndex < 0 {
				baseIndex = 0
			}
		}

		for i := range summaries {
			summaries[i].Index = baseIndex + int64(i) + 1

			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case results <- summaries[i]:
			}
		}

		errs <- nil
	}()

	return results, errs
}

// CountTracks returns the total number of distinct track IDs.
// The API layer uses this to hint clients about the upper bound of the
// pagination sequence so they can plan how many requests to issue.
func (db *Database) CountTracks(ctx context.Context, dbType string) (int64, error) {
	count, err := db.countTracksFromCatalog(ctx)
	if err != nil {
		if isTrackCatalogError(err) {
			if seedErr := db.populateTrackCatalog(ctx, dbType); seedErr != nil && !isTrackCatalogError(seedErr) {
				return 0, seedErr
			}
			count, err = db.countTracksFromCatalog(ctx)
		}
		if err != nil && !isTrackCatalogError(err) {
			return 0, err
		}
	}

	if count > 0 {
		return count, nil
	}

	if seedErr := db.populateTrackCatalog(ctx, dbType); seedErr != nil && !isTrackCatalogError(seedErr) {
		return 0, seedErr
	}

	if refreshed, err := db.countTracksFromCatalog(ctx); err == nil && refreshed >= 0 {
		return refreshed, nil
	}

	legacy, legacyErr := db.countTracksLegacy(ctx)
	if legacyErr != nil {
		return 0, legacyErr
	}
	return legacy, nil
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
// An optional LIMIT keeps the dataset bounded when callers request a
// window; otherwise we stream the entire track.
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

		if toID <= 0 || toID < fromID {
			toID = math.MaxInt64
		}

		nextPlaceholder := newPlaceholderGenerator(dbType)
		trackPlaceholder := nextPlaceholder()
		fromPlaceholder := nextPlaceholder()
		toPlaceholder := nextPlaceholder()

		limitClause := ""
		args := []any{trackID, fromID, toID}
		if limit > 0 {
			limitClause = fmt.Sprintf(" LIMIT %s", nextPlaceholder())
			args = append(args, limit)
		}

		query := fmt.Sprintf(`SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
       altitude,
       COALESCE(detector, '') AS detector,
       COALESCE(radiation, '') AS radiation,
       temperature,
       humidity
FROM markers
WHERE trackID = %s AND id >= %s AND id <= %s
ORDER BY id%s;`, trackPlaceholder, fromPlaceholder, toPlaceholder, limitClause)

		rows, err := db.DB.QueryContext(ctx, query, args...)
		if err != nil {
			errs <- fmt.Errorf("stream markers: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var m Marker
			var altitude sql.NullFloat64
			var temperature sql.NullFloat64
			var humidity sql.NullFloat64
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID,
				&altitude, &m.Detector, &m.Radiation, &temperature, &humidity); err != nil {
				errs <- fmt.Errorf("scan marker: %w", err)
				return
			}
			if altitude.Valid {
				m.Altitude = altitude.Float64
				m.AltitudeValid = true
			}
			if temperature.Valid {
				m.Temperature = temperature.Float64
				m.TemperatureValid = true
			}
			if humidity.Valid {
				m.Humidity = humidity.Float64
				m.HumidityValid = true
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
	query := fmt.Sprintf(`SELECT COUNT(*) FROM track_catalog WHERE trackID <= %s;`, nextPlaceholder())

	row := db.DB.QueryRowContext(ctx, query, trackID)
	var count sql.NullInt64
	if err := row.Scan(&count); err != nil {
		if isTrackCatalogError(err) {
			if seedErr := db.populateTrackCatalog(ctx, dbType); seedErr != nil && !isTrackCatalogError(seedErr) {
				return 0, seedErr
			}
			row = db.DB.QueryRowContext(ctx, query, trackID)
			if scanErr := row.Scan(&count); scanErr != nil {
				if isTrackCatalogError(scanErr) {
					return db.countTrackIDsUpToLegacy(ctx, trackID, dbType)
				}
				return 0, fmt.Errorf("count track ids up to: %w", scanErr)
			}
		} else {
			return 0, fmt.Errorf("count track ids up to: %w", err)
		}
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
	query := fmt.Sprintf(`SELECT trackID FROM track_catalog ORDER BY trackID LIMIT 1 OFFSET %s;`, offsetPlaceholder)

	row := db.DB.QueryRowContext(ctx, query, index-1)
	var trackID string
	if err := row.Scan(&trackID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		if isTrackCatalogError(err) {
			if seedErr := db.populateTrackCatalog(ctx, dbType); seedErr != nil && !isTrackCatalogError(seedErr) {
				return "", seedErr
			}
			row = db.DB.QueryRowContext(ctx, query, index-1)
			if scanErr := row.Scan(&trackID); scanErr != nil {
				if scanErr == sql.ErrNoRows {
					return "", nil
				}
				if isTrackCatalogError(scanErr) {
					return db.getTrackIDByIndexLegacy(ctx, index, dbType)
				}
				return "", fmt.Errorf("track id by index: %w", scanErr)
			}
			return trackID, nil
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

// countTracksFromCatalog reads the cached track count without touching the markers table.
// Keeping the helper tiny mirrors "Clear is better than clever" so callers can reason about fallbacks.
func (db *Database) countTracksFromCatalog(ctx context.Context) (int64, error) {
	row := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM track_catalog;`)
	var count sql.NullInt64
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count track catalog: %w", err)
	}
	if !count.Valid {
		return 0, nil
	}
	return count.Int64, nil
}

// countTracksLegacy computes the distinct track count directly from markers as a fallback.
// We call it only when the catalogue is missing so PostgreSQL is not hammered on every request.
func (db *Database) countTracksLegacy(ctx context.Context) (int64, error) {
	row := db.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT trackID) FROM markers;`)
	var count sql.NullInt64
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count tracks legacy: %w", err)
	}
	if !count.Valid {
		return 0, nil
	}
	return count.Int64, nil
}

// countTrackIDsUpToLegacy mirrors the previous DISTINCT query when the catalogue is unavailable.
func (db *Database) countTrackIDsUpToLegacy(ctx context.Context, trackID, dbType string) (int64, error) {
	nextPlaceholder := newPlaceholderGenerator(dbType)
	where := fmt.Sprintf("trackID <= %s", nextPlaceholder())
	query := fmt.Sprintf(`SELECT COUNT(*) FROM (SELECT DISTINCT trackID FROM markers WHERE %s) AS sub;`, where)

	row := db.DB.QueryRowContext(ctx, query, trackID)
	var count sql.NullInt64
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count track ids legacy: %w", err)
	}
	if !count.Valid {
		return 0, nil
	}
	return count.Int64, nil
}

// getTrackIDByIndexLegacy falls back to the DISTINCT plan so API consumers still succeed during migrations.
func (db *Database) getTrackIDByIndexLegacy(ctx context.Context, index int64, dbType string) (string, error) {
	nextPlaceholder := newPlaceholderGenerator(dbType)
	offsetPlaceholder := nextPlaceholder()
	query := fmt.Sprintf(`SELECT trackID FROM (SELECT DISTINCT trackID FROM markers ORDER BY trackID LIMIT 1 OFFSET %s) AS sub;`, offsetPlaceholder)

	row := db.DB.QueryRowContext(ctx, query, index-1)
	var trackID string
	if err := row.Scan(&trackID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("track id legacy: %w", err)
	}
	return trackID, nil
}

// isTrackCatalogError checks whether an error suggests the track_catalog table is missing or unreadable.
// We treat it loosely — if the catalogue is unavailable we rebuild it, embracing "Simplicity is complicated".
func isTrackCatalogError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "track_catalog") || strings.Contains(msg, "track catalog") || strings.Contains(msg, "no such table")
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
