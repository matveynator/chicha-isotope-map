package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// insertMarkersPostgreSQLCopy streams a chunk of markers into PostgreSQL using COPY to
// keep imports fast. We lean on a temporary table so we can still enforce the
// ON CONFLICT policy from the main table without losing COPY's throughput.
// The helper stays connection-local to avoid mutexes and follows "Don't
// communicate by sharing memory; share memory by communicating" by letting the
// database enforce ordering.
func (db *Database) insertMarkersPostgreSQLCopy(ctx context.Context, chunk []Marker) error {
	if len(chunk) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil || db.DB == nil {
		return fmt.Errorf("database unavailable")
	}

	conn, err := db.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open postgres connection: %w", err)
	}
	defer conn.Close()

	// The timestamp-based suffix keeps names unique per call while staying
	// predictable for debugging. Temporary scope avoids cross-connection
	// contention and mirrors "A little copying is better than a little
	// dependency" by keeping the helper self-contained.
	tempTable := fmt.Sprintf("temp_markers_%d", time.Now().UnixNano())
	// Avoid ON COMMIT DROP so the temporary table survives PostgreSQL's
	// autocommit mode long enough for COPY and the final INSERT to finish.
	createTemp := fmt.Sprintf(`CREATE TEMP TABLE %s (
doseRate DOUBLE PRECISION,
date BIGINT,
lon DOUBLE PRECISION,
lat DOUBLE PRECISION,
countRate DOUBLE PRECISION,
zoom INT,
speed DOUBLE PRECISION,
trackID TEXT,
altitude DOUBLE PRECISION,
detector TEXT,
radiation TEXT,
temperature DOUBLE PRECISION,
humidity DOUBLE PRECISION
)`, tempTable)
	if _, err := conn.ExecContext(ctx, createTemp); err != nil {
		return fmt.Errorf("create temp table: %w", err)
	}

	// Ensure cleanup even if the COPY or final insert fails; use a detached
	// context to avoid blocking shutdown when the caller's context is already
	// cancelled.
	dropCtx, dropCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dropCancel()
	defer conn.ExecContext(dropCtx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tempTable))

	rows := make([][]interface{}, 0, len(chunk))
	for _, m := range chunk {
		rows = append(rows, []interface{}{
			m.DoseRate, m.Date, m.Lon, m.Lat,
			m.CountRate, m.Zoom, m.Speed, m.TrackID,
			nullableFloat64(m.AltitudeValid, m.Altitude),
			m.Detector, m.Radiation,
			nullableFloat64(m.TemperatureValid, m.Temperature),
			nullableFloat64(m.HumidityValid, m.Humidity),
		})
	}

	copyErr := conn.Raw(func(driverConn any) error {
		direct, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("unexpected postgres driver %T", driverConn)
		}
		_, err := direct.Conn().CopyFrom(
			ctx,
			pgx.Identifier{tempTable},
			[]string{"doseRate", "date", "lon", "lat", "countRate", "zoom", "speed", "trackID", "altitude", "detector", "radiation", "temperature", "humidity"},
			pgx.CopyFromRows(rows),
		)
		return err
	})
	if copyErr != nil {
		return fmt.Errorf("copy markers into temp table: %w", copyErr)
	}

	insertFromTemp := fmt.Sprintf(`INSERT INTO markers
(doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity)
SELECT doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity FROM %s
ON CONFLICT ON CONSTRAINT markers_unique DO NOTHING`, tempTable)
	if _, err := conn.ExecContext(ctx, insertFromTemp); err != nil {
		return fmt.Errorf("merge temp markers: %w", err)
	}

	return nil
}
