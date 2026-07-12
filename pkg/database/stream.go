package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// appendSpeedRangeClause adds a speed filter while keeping SQL portable.
// We merge continuous ranges to give query planners a single BETWEEN when possible.
func appendSpeedRangeClause(sb *strings.Builder, args *[]interface{}, speedRanges []SpeedRange, dbType string) {
	if len(speedRanges) == 0 {
		return
	}
	if ok, lo, hi := mergeContinuousRanges(speedRanges); ok {
		sb.WriteString(" AND speed BETWEEN " + placeholder(dbType, len(*args)+1) + " AND " + placeholder(dbType, len(*args)+2))
		*args = append(*args, lo, hi)
		return
	}
	sb.WriteString(" AND (")
	for i, r := range speedRanges {
		if i > 0 {
			sb.WriteString(" OR ")
		}
		sb.WriteString("speed BETWEEN " + placeholder(dbType, len(*args)+1) + " AND " + placeholder(dbType, len(*args)+2))
		*args = append(*args, r.Min, r.Max)
	}
	sb.WriteString(")")
}

// StreamMarkersByZoomAndBounds streams markers row by row through a channel.
// It avoids loading large result sets into memory and stops when the context is done.
func (db *Database) StreamMarkersByZoomAndBounds(ctx context.Context, zoom int, minLat, minLon, maxLat, maxLon float64, dbType string) (<-chan Marker, <-chan error) {
	out := make(chan Marker)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		var query string
		// Order by ascending timestamp so map streams paint from oldest to newest.
		switch dbType {
		case "pgx":
			query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(device_name, '') AS device_name
                FROM markers
                WHERE zoom = $1 AND lat BETWEEN $2 AND $3 AND lon BETWEEN $4 AND $5
                ORDER BY date ASC;
            `
		default:
			query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(device_name, '') AS device_name
                FROM markers
                WHERE zoom = ? AND lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?
                ORDER BY date ASC;
            `
		}

		rows, err := db.DB.QueryContext(ctx, query, zoom, minLat, maxLat, minLon, maxLon)
		if err != nil {
			errCh <- fmt.Errorf("query markers: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				m         Marker
				deviceRaw sql.NullString
			)
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID, &deviceRaw); err != nil {
				errCh <- fmt.Errorf("scan marker: %w", err)
				return
			}
			if deviceRaw.Valid {
				m.DeviceName = deviceRaw.String
			}
			select {
			case out <- m:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

		if err := rows.Err(); err != nil {
			errCh <- fmt.Errorf("iterate markers: %w", err)
		}
	}()

	return out, errCh
}

// StreamMarkersByZoomBoundsSpeed streams markers for the current viewport and speed filters.
// It mirrors GetMarkersByZoomBoundsSpeed but keeps memory usage low for large tiles.
func (db *Database) StreamMarkersByZoomBoundsSpeed(ctx context.Context, zoom int, minLat, minLon, maxLat, maxLon float64, speedRanges []SpeedRange, dbType string) (<-chan Marker, <-chan error) {
	out := make(chan Marker)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		var (
			sb   strings.Builder
			args []interface{}
		)

		sb.WriteString("zoom = " + placeholder(dbType, len(args)+1))
		args = append(args, zoom)
		sb.WriteString(" AND lat BETWEEN " + placeholder(dbType, len(args)+1) + " AND " + placeholder(dbType, len(args)+2))
		args = append(args, minLat, maxLat)
		sb.WriteString(" AND lon BETWEEN " + placeholder(dbType, len(args)+1) + " AND " + placeholder(dbType, len(args)+2))
		args = append(args, minLon, maxLon)
		appendSpeedRangeClause(&sb, &args, speedRanges, dbType)

		query := fmt.Sprintf(`
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(device_name, '') AS device_name
                FROM markers
                WHERE %s
                ORDER BY date ASC;
            `, sb.String())

		rows, err := db.DB.QueryContext(ctx, query, args...)
		if err != nil {
			errCh <- fmt.Errorf("query markers: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				m         Marker
				deviceRaw sql.NullString
			)
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID, &deviceRaw); err != nil {
				errCh <- fmt.Errorf("scan marker: %w", err)
				return
			}
			if deviceRaw.Valid {
				m.DeviceName = deviceRaw.String
			}
			select {
			case out <- m:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

		if err := rows.Err(); err != nil {
			errCh <- fmt.Errorf("iterate markers: %w", err)
		}
	}()

	return out, errCh
}

// StreamMarkersByTrackIDZoomAndBounds streams markers of one track within bounds.
// This keeps memory usage low while focusing on a single track only.
func (db *Database) StreamMarkersByTrackIDZoomAndBounds(ctx context.Context, trackID string, zoom int, minLat, minLon, maxLat, maxLon float64, dbType string) (<-chan Marker, <-chan error) {
	out := make(chan Marker)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		var query string
		// Order by ascending timestamp so track-focused streaming paints from oldest to newest.
		switch dbType {
		case "pgx":
			query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(device_name, '') AS device_name
                FROM markers
                WHERE trackID = $1 AND zoom = $2 AND lat BETWEEN $3 AND $4 AND lon BETWEEN $5 AND $6
                ORDER BY date ASC;
            `
		default:
			query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(device_name, '') AS device_name
                FROM markers
                WHERE trackID = ? AND zoom = ? AND lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?
                ORDER BY date ASC;
            `
		}

		rows, err := db.DB.QueryContext(ctx, query, trackID, zoom, minLat, maxLat, minLon, maxLon)
		if err != nil {
			errCh <- fmt.Errorf("query markers: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				m         Marker
				deviceRaw sql.NullString
			)
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID, &deviceRaw); err != nil {
				errCh <- fmt.Errorf("scan marker: %w", err)
				return
			}
			if deviceRaw.Valid {
				m.DeviceName = deviceRaw.String
			}
			select {
			case out <- m:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

		if err := rows.Err(); err != nil {
			errCh <- fmt.Errorf("iterate markers: %w", err)
		}
	}()

	return out, errCh
}

// StreamMarkersByTrackIDZoomBoundsSpeed streams markers for a single track with speed filters.
// Keeping the speed filter in SQL avoids client-side work on large tracks.
func (db *Database) StreamMarkersByTrackIDZoomBoundsSpeed(ctx context.Context, trackID string, zoom int, minLat, minLon, maxLat, maxLon float64, speedRanges []SpeedRange, dbType string) (<-chan Marker, <-chan error) {
	out := make(chan Marker)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		var (
			sb   strings.Builder
			args []interface{}
		)

		sb.WriteString("trackID = " + placeholder(dbType, len(args)+1))
		args = append(args, trackID)
		sb.WriteString(" AND zoom = " + placeholder(dbType, len(args)+1))
		args = append(args, zoom)
		sb.WriteString(" AND lat BETWEEN " + placeholder(dbType, len(args)+1) + " AND " + placeholder(dbType, len(args)+2))
		args = append(args, minLat, maxLat)
		sb.WriteString(" AND lon BETWEEN " + placeholder(dbType, len(args)+1) + " AND " + placeholder(dbType, len(args)+2))
		args = append(args, minLon, maxLon)
		appendSpeedRangeClause(&sb, &args, speedRanges, dbType)

		query := fmt.Sprintf(`
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(device_name, '') AS device_name
                FROM markers
                WHERE %s
                ORDER BY date ASC;
            `, sb.String())

		rows, err := db.DB.QueryContext(ctx, query, args...)
		if err != nil {
			errCh <- fmt.Errorf("query markers: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				m         Marker
				deviceRaw sql.NullString
			)
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID, &deviceRaw); err != nil {
				errCh <- fmt.Errorf("scan marker: %w", err)
				return
			}
			if deviceRaw.Valid {
				m.DeviceName = deviceRaw.String
			}
			select {
			case out <- m:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}

		if err := rows.Err(); err != nil {
			errCh <- fmt.Errorf("iterate markers: %w", err)
		}
	}()

	return out, errCh
}
