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

		// Keep the temporary table alive during COPY by placing it inside a
		// single transaction. ON COMMIT DROP handles cleanup even when the
		// caller exits early, mirroring "Clear is better than clever" by
		// letting PostgreSQL manage scope.
		pgConn := direct.Conn()
		tx, err := pgConn.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin postgres transaction: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback(ctx)
			}
		}()

		// The timestamp-based suffix keeps names unique per call while staying
		// predictable for debugging. Temporary scope avoids cross-connection
		// contention and mirrors "A little copying is better than a little
		// dependency" by keeping the helper self-contained.
		tempTable := fmt.Sprintf("temp_markers_%d", time.Now().UnixNano())
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
 ) ON COMMIT DROP`, tempTable)
		if _, err := tx.Exec(ctx, createTemp); err != nil {
			return fmt.Errorf("create temp table: %w", err)
		}

		if _, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{tempTable},
			[]string{"doseRate", "date", "lon", "lat", "countRate", "zoom", "speed", "trackID", "altitude", "detector", "radiation", "temperature", "humidity"},
			pgx.CopyFromRows(rows),
		); err != nil {
			return fmt.Errorf("copy markers into temp table: %w", err)
		}

		insertFromTemp := fmt.Sprintf(`INSERT INTO markers
 (doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity)
 SELECT doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity FROM %s
 ON CONFLICT ON CONSTRAINT markers_unique DO NOTHING`, tempTable)
		if _, err := tx.Exec(ctx, insertFromTemp); err != nil {
			return fmt.Errorf("merge temp markers: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit temp markers: %w", err)
		}
		committed = true
		return nil
	})
	if copyErr != nil {
		return copyErr
	}

	return nil
}
