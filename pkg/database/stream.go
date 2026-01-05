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
                SELECT m.id, m.doseRate, m.date, m.lon, m.lat, m.countRate, m.zoom, m.speed, m.trackID,
                       COALESCE(NULLIF(m.device_id, ''), t.device_id, '') AS device_id,
                       COALESCE(NULLIF(m.device_name, ''), d.model, '') AS device_name
                FROM markers m
                LEFT JOIN tracks t ON m.trackID = t.trackID
                LEFT JOIN devices d ON t.device_id = d.device_id
                WHERE m.zoom = $1 AND m.lat BETWEEN $2 AND $3 AND m.lon BETWEEN $4 AND $5;
            `
		default:
			query = `
                SELECT m.id, m.doseRate, m.date, m.lon, m.lat, m.countRate, m.zoom, m.speed, m.trackID,
                       COALESCE(NULLIF(m.device_id, ''), t.device_id, '') AS device_id,
                       COALESCE(NULLIF(m.device_name, ''), d.model, '') AS device_name
                FROM markers m
                LEFT JOIN tracks t ON m.trackID = t.trackID
                LEFT JOIN devices d ON t.device_id = d.device_id
                WHERE m.zoom = ? AND m.lat BETWEEN ? AND ? AND m.lon BETWEEN ? AND ?;
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
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID, &m.DeviceID, &m.DeviceName); err != nil {
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
                SELECT m.id, m.doseRate, m.date, m.lon, m.lat, m.countRate, m.zoom, m.speed, m.trackID,
                       COALESCE(NULLIF(m.device_id, ''), t.device_id, '') AS device_id,
                       COALESCE(NULLIF(m.device_name, ''), d.model, '') AS device_name
                FROM markers m
                LEFT JOIN tracks t ON m.trackID = t.trackID
                LEFT JOIN devices d ON t.device_id = d.device_id
                WHERE m.trackID = $1 AND m.zoom = $2 AND m.lat BETWEEN $3 AND $4 AND m.lon BETWEEN $5 AND $6;
            `
		default:
			query = `
                SELECT m.id, m.doseRate, m.date, m.lon, m.lat, m.countRate, m.zoom, m.speed, m.trackID,
                       COALESCE(NULLIF(m.device_id, ''), t.device_id, '') AS device_id,
                       COALESCE(NULLIF(m.device_name, ''), d.model, '') AS device_name
                FROM markers m
                LEFT JOIN tracks t ON m.trackID = t.trackID
                LEFT JOIN devices d ON t.device_id = d.device_id
                WHERE m.trackID = ? AND m.zoom = ? AND m.lat BETWEEN ? AND ? AND m.lon BETWEEN ? AND ?;
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
			if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID, &m.DeviceID, &m.DeviceName); err != nil {
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
