package database

import (
	"context"
	"fmt"
)

// StreamMarkersByZoomAndBounds streams markers row by row through a channel.
// It avoids loading large result sets into memory and stops when the context is done.
func (db *Database) StreamMarkersByZoomAndBounds(ctx context.Context, zoom int, minLat, minLon, maxLat, maxLon float64, dbType string) (<-chan Marker, <-chan error) {
	out := make(chan Marker)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		var query string
		switch dbType {
		case "pgx":
			query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
                FROM markers
                WHERE zoom = $1 AND lat BETWEEN $2 AND $3 AND lon BETWEEN $4 AND $5;
            `
		default:
			query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
                FROM markers
                WHERE zoom = ? AND lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?;
            `
		}

		rows, err := db.DB.QueryContext(ctx, query, zoom, minLat, maxLat, minLon, maxLon)
		if err != nil {
			errCh <- fmt.Errorf("query markers: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var m Marker
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID); err != nil {
				errCh <- fmt.Errorf("scan marker: %w", err)
				return
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
		switch dbType {
		case "pgx":
			query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
                FROM markers
                WHERE trackID = $1 AND zoom = $2 AND lat BETWEEN $3 AND $4 AND lon BETWEEN $5 AND $6;
            `
		default:
			query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
                FROM markers
                WHERE trackID = ? AND zoom = ? AND lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?;
            `
		}

		rows, err := db.DB.QueryContext(ctx, query, trackID, zoom, minLat, maxLat, minLon, maxLon)
		if err != nil {
			errCh <- fmt.Errorf("query markers: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var m Marker
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID); err != nil {
				errCh <- fmt.Errorf("scan marker: %w", err)
				return
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
