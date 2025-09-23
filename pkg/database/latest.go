package database

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

// =============================
// Nearest marker lookup helpers
// =============================

// StreamLatestMarkersNear streams the newest markers around a coordinate.
// We favour a streaming design so callers can start encoding results without
// waiting for the full slice, mirroring the Go proverb "Don't communicate by
// sharing memory; share memory by communicating".
func (db *Database) StreamLatestMarkersNear(
	ctx context.Context,
	lat float64,
	lon float64,
	radiusMeters float64,
	limit int,
	dbType string,
) (<-chan Marker, <-chan error) {
	out := make(chan Marker)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)

		if db == nil || db.DB == nil {
			errs <- fmt.Errorf("database unavailable")
			return
		}

		if radiusMeters <= 0 {
			errs <- fmt.Errorf("radius must be positive")
			return
		}

		if limit <= 0 {
			limit = 1
		}
		if limit > 500 {
			limit = 500
		}

		lat = clampLatitude(lat)
		lon = clampLongitude(lon)

		fetchLimit := limit * 3
		if fetchLimit < limit {
			fetchLimit = limit
		}
		if fetchLimit > 750 {
			fetchLimit = 750
		}

		halfDeltaLat := radiusMeters / 111320.0
		if halfDeltaLat <= 0 {
			halfDeltaLat = 0.0001
		}

		latRad := lat * math.Pi / 180.0
		cosLat := math.Cos(latRad)
		if math.Abs(cosLat) < 1e-6 {
			cosLat = 1e-6
		}
		halfDeltaLon := radiusMeters / (111320.0 * math.Abs(cosLat))

		minLat := clampLatitude(lat - halfDeltaLat)
		maxLat := clampLatitude(lat + halfDeltaLat)
		rawMinLon := lon - halfDeltaLon
		rawMaxLon := lon + halfDeltaLon
		crossDateline := rawMinLon < -180 || rawMaxLon > 180
		minLon := clampLongitude(rawMinLon)
		maxLon := clampLongitude(rawMaxLon)
		if crossDateline {
			minLon = -180
			maxLon = 180
		}

		nextPlaceholder := newPlaceholderGenerator(dbType)
		minLatPlaceholder := nextPlaceholder()
		maxLatPlaceholder := nextPlaceholder()
		minLonPlaceholder := nextPlaceholder()
		maxLonPlaceholder := nextPlaceholder()
		limitPlaceholder := nextPlaceholder()

		query := fmt.Sprintf(`SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
       altitude,
       COALESCE(detector, '') AS detector,
       COALESCE(radiation, '') AS radiation,
       temperature,
       humidity
FROM markers
WHERE lat >= %s AND lat <= %s AND lon >= %s AND lon <= %s
ORDER BY date DESC
LIMIT %s;`,
			minLatPlaceholder,
			maxLatPlaceholder,
			minLonPlaceholder,
			maxLonPlaceholder,
			limitPlaceholder,
		)

		rows, err := db.DB.QueryContext(ctx, query, minLat, maxLat, minLon, maxLon, fetchLimit)
		if err != nil {
			errs <- fmt.Errorf("latest markers query: %w", err)
			return
		}
		defer rows.Close()

		sent := 0
		for rows.Next() {
			var marker Marker
			var altitude sql.NullFloat64
			var temperature sql.NullFloat64
			var humidity sql.NullFloat64

			if err := rows.Scan(
				&marker.ID,
				&marker.DoseRate,
				&marker.Date,
				&marker.Lon,
				&marker.Lat,
				&marker.CountRate,
				&marker.Zoom,
				&marker.Speed,
				&marker.TrackID,
				&altitude,
				&marker.Detector,
				&marker.Radiation,
				&temperature,
				&humidity,
			); err != nil {
				errs <- fmt.Errorf("scan latest marker: %w", err)
				return
			}

			if altitude.Valid {
				marker.Altitude = altitude.Float64
				marker.AltitudeValid = true
			}
			if temperature.Valid {
				marker.Temperature = temperature.Float64
				marker.TemperatureValid = true
			}
			if humidity.Valid {
				marker.Humidity = humidity.Float64
				marker.HumidityValid = true
			}

			if distanceMeters(marker.Lat, marker.Lon, lat, lon) > radiusMeters {
				continue
			}

			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case out <- marker:
				sent++
			}

			if sent >= limit {
				errs <- nil
				return
			}
		}

		if err := rows.Err(); err != nil {
			errs <- fmt.Errorf("iterate latest markers: %w", err)
			return
		}

		errs <- nil
	}()

	return out, errs
}

func clampLatitude(v float64) float64 {
	if v > 90 {
		return 90
	}
	if v < -90 {
		return -90
	}
	return v
}

func clampLongitude(v float64) float64 {
	if v > 180 {
		return 180
	}
	if v < -180 {
		return -180
	}
	return v
}

func distanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	phi1 := lat1 * math.Pi / 180.0
	phi2 := lat2 * math.Pi / 180.0
	dPhi := (lat2 - lat1) * math.Pi / 180.0
	dLambda := (lon2 - lon1) * math.Pi / 180.0

	sinDPhi := math.Sin(dPhi / 2)
	sinDLambda := math.Sin(dLambda / 2)
	a := sinDPhi*sinDPhi + math.Cos(phi1)*math.Cos(phi2)*sinDLambda*sinDLambda
	if a < 0 {
		a = 0
	}
	if a > 1 {
		a = 1
	}
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}
