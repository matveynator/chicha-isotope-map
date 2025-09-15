package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// Database represents the interface for interacting with the database.
type Database struct {
	DB          *sql.DB    // The underlying SQL database connection
	idGenerator chan int64 // Channel for generating unique IDs
}

// startIDGenerator launches a goroutine for generating unique IDs.
func startIDGenerator(initialID int64) chan int64 {
	idChannel := make(chan int64)
	go func(start int64) {
		currentID := start
		for {
			idChannel <- currentID
			currentID++
		}
	}(initialID)
	return idChannel
}

// Config holds the configuration details for initializing the database.
type Config struct {
	DBType    string // The type of the database driver (e.g., "sqlite", "genji", or "pgx" (PostgreSQL))
	DBPath    string // The file path to the database file (for file-based databases)
	DBHost    string // The host for PostgreSQL
	DBPort    int    // The port for PostgreSQL
	DBUser    string // The user for PostgreSQL
	DBPass    string // The password for PostgreSQL
	DBName    string // The name of the PostgreSQL database
	PGSSLMode string // The SSL mode for PostgreSQL
	Port      int    // The port number (used in database file naming if needed)
}

// NewDatabase opens DB and configures connection pooling.
// For SQLite/Genji we force single-connection mode (no concurrent DB access).
func NewDatabase(config Config) (*Database, error) {
	var dsn string

	switch strings.ToLower(config.DBType) {
	case "sqlite", "genji":
		dsn = config.DBPath
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.%s", config.Port, strings.ToLower(config.DBType))
		}
	case "duckdb":
		// файл создастся при первом открытии
		dsn = config.DBPath
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.duckdb", config.Port)
		}
	case "pgx":
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			config.DBUser, config.DBPass, config.DBHost, config.DBPort, config.DBName, config.PGSSLMode)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.DBType)
	}

	db, err := sql.Open(strings.ToLower(config.DBType), dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening the database: %v", err)
	}

	// === CRITICAL: serialize SQLite/Genji access over a single underlying connection ===
	switch strings.ToLower(config.DBType) {
	case "sqlite", "genji":
		// One physical connection; no concurrent statements at DB layer.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		// Never recycle the single connection (keeps it stable for the whole process).
		db.SetConnMaxLifetime(0)
	}

	// Cheap liveness probe with timeout so we don't hang at startup
	{
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("error connecting to the database: %v", err)
		}
	}

	log.Printf("Using database driver: %s with DSN: %s", strings.ToLower(config.DBType), dsn)

	// Bootstrap ID generator from current MAX(id); ignore error if table not created yet
	var maxID sql.NullInt64
	_ = db.QueryRow(`SELECT MAX(id) FROM markers`).Scan(&maxID)
	initialID := int64(1)
	if maxID.Valid {
		initialID = maxID.Int64 + 1
	}
	idChannel := startIDGenerator(initialID)

	return &Database{
		DB:          db,
		idGenerator: idChannel,
	}, nil
}

// EnsureIndexesAsync builds non-critical indexes in background, politely.
// - No pinned connections (important for sqlite/genji with MaxOpenConns(1)).
// - No pre-checks: just CREATE INDEX IF NOT EXISTS.
// - Retries with exponential backoff on "database is locked"/"SQLITE_BUSY".
func (db *Database) EnsureIndexesAsync(ctx context.Context, cfg Config, logf func(string, ...any)) {
	type idx struct{ name, sql string }

	indexes := desiredIndexesPortable(cfg.DBType)
	if len(indexes) == 0 {
		return
	}

	// single worker: avoids DDL self-contention and keeps app responsive
	worker := func() {
		logf("⏳ background index build scheduled (engine=%s). Listeners are up; pages may be slower until indexes are ready.", cfg.DBType)

		for _, it := range indexes {
			start := time.Now()
			logf("▶️  start index %s", it.name)

			// polite retry loop for SQLite/Genji "busy"/locks; portable for others too
			backoff := 50 * time.Millisecond
			for {
				// respect outer context: if cancelled — stop gracefully
				select {
				case <-ctx.Done():
					logf("⏹️  stop index builder due to context cancel: %v", ctx.Err())
					return
				default:
				}

				_, err := db.DB.ExecContext(ctx, it.sql)
				if err == nil {
					logf("✅ index %s ready in %s", it.name, time.Since(start).Truncate(time.Millisecond))
					break
				}

				msg := strings.ToLower(err.Error())
				// treat "already exists" style as success (race, or double run)
				if strings.Contains(msg, "already exists") ||
					strings.Contains(msg, "duplicate key value") ||
					strings.Contains(msg, "sqlstate 23505") {
					logf("⏭️  index %s appears to exist. continue.", it.name)
					break
				}

				// busy/locked → backoff and retry
				if strings.Contains(msg, "database is locked") ||
					strings.Contains(msg, "sqlite_busy") ||
					strings.Contains(msg, "resource busy") ||
					strings.Contains(msg, "locked") {
					// cap backoff to 1s, keep it gentle to not starve uploads
					time.Sleep(backoff)
					if backoff < time.Second {
						backoff *= 2
						if backoff > time.Second {
							backoff = time.Second
						}
					}
					continue
				}

				// other errors: log and continue with next index
				logf("❌ index %s failed after %s: %v", it.name, time.Since(start).Truncate(time.Millisecond), err)
				break
			}
		}
	}

	// run in background
	go worker()
}

// desiredIndexesPortable declares the set of indexes we want to have for each engine.
// Keep SQL portable: only CREATE {UNIQUE} INDEX IF NOT EXISTS on plain columns.
// We intentionally avoid engine-specific syntax and rely on background creation.
func desiredIndexesPortable(dbType string) []struct{ name, sql string } {
	low := strings.ToLower(dbType)
	switch low {

	case "pgx":
		// PostgreSQL: primary composite indexes that accelerate map/bounds queries.
		return []struct{ name, sql string }{
			// 1) Composite first — biggest wins for rendering/bounds
			{"idx_markers_zoom_bounds",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon)`},
			{"idx_markers_trackid_zoom_bounds",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_zoom_bounds ON markers (trackID, zoom, lat, lon)`},
			// Include speed for one-pass range plans
			{"idx_markers_zoom_bounds_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds_speed ON markers (zoom, lat, lon, speed)`},
			// Probe for duplicates detection/identity by lat/lon/date/doseRate
			{"idx_markers_identity_probe",
				`CREATE INDEX IF NOT EXISTS idx_markers_identity_probe ON markers (lat, lon, date, doseRate)`},
			// 2) Selective singles
			{"idx_markers_trackid",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID)`},
			{"idx_markers_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_date ON markers (date)`},
			{"idx_markers_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_speed ON markers (speed)`},
		}

	case "duckdb":
		// DuckDB: same useful composite/single indexes as for PostgreSQL.
		// Do NOT create an extra UNIQUE index — we already have table-level UNIQUE constraint in schema.
		return []struct{ name, sql string }{
			{"idx_markers_zoom_bounds",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon)`},
			{"idx_markers_trackid_zoom_bounds",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_zoom_bounds ON markers (trackID, zoom, lat, lon)`},
			{"idx_markers_zoom_bounds_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds_speed ON markers (zoom, lat, lon, speed)`},
			{"idx_markers_identity_probe",
				`CREATE INDEX IF NOT EXISTS idx_markers_identity_probe ON markers (lat, lon, date, doseRate)`},
			{"idx_markers_trackid",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID)`},
			{"idx_markers_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_date ON markers (date)`},
			{"idx_markers_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_speed ON markers (speed)`},
		}

	case "sqlite", "genji":
		// SQLite/Genji: keep a UNIQUE index (no table-level UNIQUE constraint there).
		return []struct{ name, sql string }{
			{"idx_markers_unique",
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_markers_unique ON markers (doseRate, date, lon, lat, countRate, zoom, speed, trackID)`},
			{"idx_markers_zoom_bounds",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon)`},
			{"idx_markers_trackid_zoom_bounds",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_zoom_bounds ON markers (trackID, zoom, lat, lon)`},
			{"idx_markers_zoom_bounds_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds_speed ON markers (zoom, lat, lon, speed)`},
			{"idx_markers_identity_probe",
				`CREATE INDEX IF NOT EXISTS idx_markers_identity_probe ON markers (lat, lon, date, doseRate)`},
			{"idx_markers_trackid",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID)`},
			{"idx_markers_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_date ON markers (date)`},
			{"idx_markers_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_speed ON markers (speed)`},
		}

	default:
		// Fallback: behave like SQLite/Genji (portable everywhere that supports IF NOT EXISTS).
		return []struct{ name, sql string }{
			{"idx_markers_unique",
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_markers_unique ON markers (doseRate, date, lon, lat, countRate, zoom, speed, trackID)`},
			{"idx_markers_zoom_bounds",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon)`},
			{"idx_markers_trackid_zoom_bounds",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_zoom_bounds ON markers (trackID, zoom, lat, lon)`},
			{"idx_markers_zoom_bounds_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds_speed ON markers (zoom, lat, lon, speed)`},
			{"idx_markers_identity_probe",
				`CREATE INDEX IF NOT EXISTS idx_markers_identity_probe ON markers (lat, lon, date, doseRate)`},
			{"idx_markers_trackid",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID)`},
			{"idx_markers_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_date ON markers (date)`},
			{"idx_markers_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_speed ON markers (speed)`},
		}
	}
}

// indexExistsPortable checks index presence using portable catalog queries.
// We keep it engine-specific but simple, and do this at app level (= not
// relying on engine-specific CREATE options).
func (db *Database) indexExistsPortable(ctx context.Context, dbType, indexName string) (bool, error) {
	switch strings.ToLower(dbType) {
	case "pgx":
		// Look for an index with this name in any schema on search_path.
		// pg_class.relkind = 'i' means "index".
		const q = `
SELECT 1
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'i'
  AND c.relname = $1
  AND n.nspname = ANY (current_schemas(true))
LIMIT 1`
		var one int
		err := db.DB.QueryRowContext(ctx, q, indexName).Scan(&one)
		if err == sql.ErrNoRows {
			return false, nil
		}
		return err == nil, err

	case "sqlite", "genji":
		// Standard SQLite catalog
		const q = `SELECT name FROM sqlite_master WHERE type='index' AND name=? LIMIT 1`
		var name string
		err := db.DB.QueryRowContext(ctx, q, indexName).Scan(&name)
		if err == sql.ErrNoRows {
			return false, nil
		}
		return err == nil, err

	default:
		// Unknown engine: assume not exists to try creation;
		// caller will catch "already exists" text if any.
		return false, nil
	}
}

// InitSchema creates minimal required schema synchronously so that
// the app can accept traffic immediately. Heavy indexes are built later
// by EnsureIndexesAsync in background.
func (db *Database) InitSchema(cfg Config) error {
	var schema string

	switch strings.ToLower(cfg.DBType) {
	case "pgx":
		// PostgreSQL — standard types, named UNIQUE to target by ON CONFLICT
		schema = `
CREATE TABLE IF NOT EXISTS markers (
  id         BIGSERIAL PRIMARY KEY,
  doseRate   DOUBLE PRECISION,
  date       BIGINT,
  lon        DOUBLE PRECISION,
  lat        DOUBLE PRECISION,
  countRate  DOUBLE PRECISION,
  zoom       INTEGER,
  speed      DOUBLE PRECISION,
  trackID    TEXT,
  CONSTRAINT markers_unique UNIQUE (doseRate,date,lon,lat,countRate,zoom,speed,trackID)
);

CREATE TABLE IF NOT EXISTS realtime_measurements (
  id          BIGSERIAL PRIMARY KEY,
  device_id   TEXT,
  transport   TEXT,
  value       DOUBLE PRECISION,
  unit        TEXT,
  lat         DOUBLE PRECISION,
  lon         DOUBLE PRECISION,
  measured_at BIGINT,
  fetched_at  BIGINT,
  CONSTRAINT realtime_unique UNIQUE (device_id,measured_at)
);`

	case "sqlite", "genji":
		// Portable SQLite/Genji side — explicit INTEGER PK
		schema = `
CREATE TABLE IF NOT EXISTS markers (
  id         INTEGER PRIMARY KEY,
  doseRate   REAL,
  date       BIGINT,
  lon        REAL,
  lat        REAL,
  countRate  REAL,
  zoom       INTEGER,
  speed      REAL,
  trackID    TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_markers_unique
  ON markers (doseRate,date,lon,lat,countRate,zoom,speed,trackID);

CREATE TABLE IF NOT EXISTS realtime_measurements (
  id          INTEGER PRIMARY KEY,
  device_id   TEXT,
  transport   TEXT,
  value       REAL,
  unit        TEXT,
  lat         REAL,
  lon         REAL,
  measured_at BIGINT,
  fetched_at  BIGINT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_realtime_unique
  ON realtime_measurements (device_id,measured_at);`

	case "duckdb":
		// DuckDB — no SERIAL/AUTOINCREMENT; use a sequence + DEFAULT nextval(...).
		// Both CREATE SEQUENCE IF NOT EXISTS and ON CONFLICT are supported.
		// We also keep a named UNIQUE to match our upsert policy.
		// Ref: DuckDB docs for sequences & ON CONFLICT.
		schema = `
CREATE SEQUENCE IF NOT EXISTS markers_id_seq START 1;
CREATE TABLE IF NOT EXISTS markers (
  id         BIGINT PRIMARY KEY DEFAULT nextval('markers_id_seq'),
  doseRate   DOUBLE,
  date       BIGINT,
  lon        DOUBLE,
  lat        DOUBLE,
  countRate  DOUBLE,
  zoom       INTEGER,
  speed      DOUBLE,
  trackID    TEXT,
  CONSTRAINT markers_unique UNIQUE (doseRate,date,lon,lat,countRate,zoom,speed,trackID)
);

CREATE SEQUENCE IF NOT EXISTS realtime_measurements_id_seq START 1;
CREATE TABLE IF NOT EXISTS realtime_measurements (
  id          BIGINT PRIMARY KEY DEFAULT nextval('realtime_measurements_id_seq'),
  device_id   TEXT,
  transport   TEXT,
  value       DOUBLE,
  unit        TEXT,
  lat         DOUBLE,
  lon         DOUBLE,
  measured_at BIGINT,
  fetched_at  BIGINT,
  CONSTRAINT realtime_unique UNIQUE (device_id,measured_at)
);`

	default:
		return fmt.Errorf("unsupported database type: %s", cfg.DBType)
	}

	if _, err := db.DB.Exec(schema); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	// Ensure optional columns exist even for older databases.
	if err := db.ensureRealtimeTransportColumn(cfg.DBType); err != nil {
		return fmt.Errorf("add transport column: %w", err)
	}

	return nil
}

// ensureRealtimeTransportColumn adds the transport column to realtime_measurements when missing.
// We keep SQL portable by using vendor-specific IF NOT EXISTS only where supported.
// Older SQLite/Genji databases are inspected via PRAGMA before issuing ALTER TABLE.
func (db *Database) ensureRealtimeTransportColumn(dbType string) error {
	switch strings.ToLower(dbType) {
	case "pgx", "duckdb":
		// These engines support IF NOT EXISTS and will ignore duplicates.
		_, err := db.DB.Exec(`ALTER TABLE realtime_measurements ADD COLUMN IF NOT EXISTS transport TEXT`)
		return err

	default:
		// SQLite and friends: introspect the table first to avoid an error.
		rows, err := db.DB.Query(`PRAGMA table_info(realtime_measurements);`)
		if err != nil {
			return fmt.Errorf("describe realtime_measurements: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				cid     int
				name    string
				ctype   string
				notnull int
				dflt    sql.NullString
				pk      int
			)
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				return fmt.Errorf("scan pragma: %w", err)
			}
			if name == "transport" {
				return nil // column already present
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate pragma: %w", err)
		}
		_, err = db.DB.Exec(`ALTER TABLE realtime_measurements ADD COLUMN transport TEXT`)
		return err
	}
}

// InsertMarkersBulk inserts markers in batches using multi-row VALUES.
// - Portable: only standard SQL and database/sql, no vendor extensions.
// - Fast: far fewer statements, WAL and B-Tree updates coalesce better.
// - Safe: still respects the unique key via ON CONFLICT DO NOTHING.
//
// Go-proverbs applied:
//   - "A little copying is better than a little dependency" — we build SQL by hand.
//   - "Don't communicate by sharing memory; share memory by communicating" — idGenerator via channel.
//   - "Make the zero value useful" — batch<=0 falls back to 500.
func (db *Database) InsertMarkersBulk(tx *sql.Tx, markers []Marker, dbType string, batch int) error {
	if len(markers) == 0 {
		return nil
	}
	if batch <= 0 {
		batch = 500
	}

	ph := func(n int) string {
		if strings.EqualFold(dbType, "pgx") {
			return fmt.Sprintf("$%d", n)
		}
		return "?"
	}

	i := 0
	for i < len(markers) {
		end := i + batch
		if end > len(markers) {
			end = len(markers)
		}
		chunk := markers[i:end]

		var sb strings.Builder
		args := make([]interface{}, 0, len(chunk)*9) // worst-case (SQLite needs id)

		switch strings.ToLower(dbType) {
		case "pgx":
			// PostgreSQL: BIGSERIAL fills id, no id column in VALUES.
			sb.WriteString("INSERT INTO markers (doseRate,date,lon,lat,countRate,zoom,speed,trackID) VALUES ")
			argn := 0
			for j, m := range chunk {
				if j > 0 {
					sb.WriteString(",")
				}
				sb.WriteString("(")
				for k := 0; k < 8; k++ {
					if k > 0 {
						sb.WriteString(",")
					}
					argn++
					sb.WriteString(ph(argn))
				}
				sb.WriteString(")")
				args = append(args, m.DoseRate, m.Date, m.Lon, m.Lat, m.CountRate, m.Zoom, m.Speed, m.TrackID)
			}
			sb.WriteString(" ON CONFLICT ON CONSTRAINT markers_unique DO NOTHING")

		default:
			// SQLite / Genji: need explicit 'id' if we want to avoid PRIMARY KEY conflicts
			// when multiple aggregated markers would default to 0.
			sb.WriteString("INSERT INTO markers (id,doseRate,date,lon,lat,countRate,zoom,speed,trackID) VALUES ")
			argn := 0
			for j, m := range chunk {
				if j > 0 {
					sb.WriteString(",")
				}
				// Ensure id is unique through our channel generator.
				if m.ID == 0 {
					m.ID = <-db.idGenerator
					chunk[j].ID = m.ID
				}
				sb.WriteString("(")
				for k := 0; k < 9; k++ {
					if k > 0 {
						sb.WriteString(",")
					}
					argn++
					sb.WriteString(ph(argn))
				}
				sb.WriteString(")")
				args = append(args, m.ID, m.DoseRate, m.Date, m.Lon, m.Lat, m.CountRate, m.Zoom, m.Speed, m.TrackID)
			}
			sb.WriteString(" ON CONFLICT DO NOTHING")
		}

		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return fmt.Errorf("bulk exec: %w", err)
		}
		i = end
	}
	return nil
}

// SaveMarkerAtomic inserts a marker and silently ignores duplicates.
//
//   - PostgreSQL (pgx) – опираемся на BIGSERIAL, id не передаём;
//   - SQLite и Genji   – если id == 0, берём следующий из idGenerator.
//     Это устраняет ошибку, когда все агрегатные маркеры имели id-0
//     и вторая вставка ломалась на UNIQUE PRIMARY KEY.
func (db *Database) SaveMarkerAtomic(
	exec sqlExecutor, m Marker, dbType string,
) error {

	switch dbType {

	// ──────────────────────────── PostgreSQL (pgx) ───────────
	case "pgx":
		_, err := exec.Exec(`
INSERT INTO markers
      (doseRate,date,lon,lat,countRate,zoom,speed,trackID)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT ON CONSTRAINT markers_unique DO NOTHING`,
			m.DoseRate, m.Date, m.Lon, m.Lat,
			m.CountRate, m.Zoom, m.Speed, m.TrackID)
		return err

	// ─────────────────────── SQLite / Genji / другие ─────────
	default:
		if m.ID == 0 {
			// берём следующий уникальный id из генератора
			m.ID = <-db.idGenerator
		}
		_, err := exec.Exec(`
INSERT INTO markers
      (id,doseRate,date,lon,lat,countRate,zoom,speed,trackID)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT DO NOTHING`,
			m.ID, m.DoseRate, m.Date, m.Lon, m.Lat,
			m.CountRate, m.Zoom, m.Speed, m.TrackID)
		return err
	}
}

// InsertRealtimeMeasurement stores live device data and skips duplicates.
// A little copying is better than a little dependency, so we build SQL by hand.
func (db *Database) InsertRealtimeMeasurement(m RealtimeMeasurement, dbType string) error {
	switch strings.ToLower(dbType) {
	case "pgx", "duckdb":
		_, err := db.DB.Exec(`
INSERT INTO realtime_measurements
      (device_id,transport,value,unit,lat,lon,measured_at,fetched_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT ON CONSTRAINT realtime_unique DO NOTHING`,
			m.DeviceID, m.Transport, m.Value, m.Unit, m.Lat, m.Lon, m.MeasuredAt, m.FetchedAt)
		return err

	default:
		if m.ID == 0 {
			m.ID = <-db.idGenerator
		}
		_, err := db.DB.Exec(`
INSERT INTO realtime_measurements
      (id,device_id,transport,value,unit,lat,lon,measured_at,fetched_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT(device_id,measured_at) DO NOTHING`,
			m.ID, m.DeviceID, m.Transport, m.Value, m.Unit, m.Lat, m.Lon, m.MeasuredAt, m.FetchedAt)
		return err
	}
}

// GetLatestRealtimeByBounds returns the newest reading per device within bounds.
// We keep SQL portable and filter duplicates in Go, following "Clear is better than clever".
func (db *Database) GetLatestRealtimeByBounds(minLat, minLon, maxLat, maxLon float64, dbType string) ([]Marker, error) {
	var query string
	switch strings.ToLower(dbType) {
	case "pgx", "duckdb":
		query = `
SELECT device_id,value,unit,lat,lon,measured_at
FROM realtime_measurements
WHERE lat BETWEEN $1 AND $2 AND lon BETWEEN $3 AND $4
ORDER BY device_id,fetched_at DESC;`
	default:
		query = `
SELECT device_id,value,unit,lat,lon,measured_at
FROM realtime_measurements
WHERE lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?
ORDER BY device_id,fetched_at DESC;`
	}

	rows, err := db.DB.Query(query, minLat, maxLat, minLon, maxLon)
	if err != nil {
		return nil, fmt.Errorf("query realtime: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var out []Marker
	for rows.Next() {
		var (
			id       string
			val      float64
			unit     string
			lat, lon float64
			measured int64
		)
		if err := rows.Scan(&id, &val, &unit, &lat, &lon, &measured); err != nil {
			return nil, fmt.Errorf("scan realtime: %w", err)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, Marker{
			DoseRate:  val,
			Date:      measured,
			Lon:       lon,
			Lat:       lat,
			CountRate: 0,
			Zoom:      0,
			Speed:     -1, // negative speed marks realtime in UI
			TrackID:   id,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate realtime: %w", err)
	}
	return out, nil
}

// sqlExecutor is satisfied by both *sql.Tx and *sql.DB.
type sqlExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// GetMarkersByZoomAndBounds retrieves markers filtered by zoom level and geographical bounds.
func (db *Database) GetMarkersByZoomAndBounds(zoom int, minLat, minLon, maxLat, maxLon float64, dbType string) ([]Marker, error) {
	var query string

	// Use appropriate placeholders depending on the database type
	switch dbType {
	case "pgx": // PostgreSQL
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE zoom = $1 AND lat BETWEEN $2 AND $3 AND lon BETWEEN $4 AND $5;
		`
	default: // SQLite or Genji
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE zoom = ? AND lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?;
		`
	}

	// Execute the query with the appropriate placeholders
	rows, err := db.DB.Query(query, zoom, minLat, maxLat, minLon, maxLon)
	if err != nil {
		return nil, fmt.Errorf("error querying markers: %v", err)
	}
	defer rows.Close()

	var markers []Marker

	// Iterate over the rows and scan each result into a Marker struct
	for rows.Next() {
		var marker Marker
		err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate, &marker.Zoom, &marker.Speed, &marker.TrackID)
		if err != nil {
			return nil, fmt.Errorf("error scanning marker: %v", err)
		}
		markers = append(markers, marker)
	}

	// Check for any errors encountered during iteration
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over result set: %v", err)
	}

	return markers, nil
}

// GetMarkersByTrackID retrieves markers filtered by trackID.
func (db *Database) GetMarkersByTrackID(trackID string, dbType string) ([]Marker, error) {
	var query string

	// Use appropriate placeholders depending on the database type
	switch dbType {
	case "pgx":
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE trackID = $1;
		`
	default:
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE trackID = ?;
		`
	}

	// Execute the query
	rows, err := db.DB.Query(query, trackID)
	if err != nil {
		return nil, fmt.Errorf("error querying markers by trackID: %v", err)
	}
	defer rows.Close()

	var markers []Marker

	// Iterate over the rows and scan each result into a Marker struct
	for rows.Next() {
		var marker Marker
		err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate, &marker.Zoom, &marker.Speed, &marker.TrackID)
		if err != nil {
			return nil, fmt.Errorf("error scanning marker: %v", err)
		}
		markers = append(markers, marker)
	}

	// Check for any errors encountered during iteration
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over result set: %v", err)
	}

	return markers, nil
}

// GetMarkersByTrackIDAndBounds retrieves markers filtered by trackID and geographical bounds.
func (db *Database) GetMarkersByTrackIDAndBounds(trackID string, minLat, minLon, maxLat, maxLon float64, dbType string) ([]Marker, error) {
	var query string

	// Use appropriate placeholders depending on the database type
	switch dbType {
	case "pgx":
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE trackID = $1 AND lat BETWEEN $2 AND $3 AND lon BETWEEN $4 AND $5;
		`
	default:
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE trackID = ? AND lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?;
		`
	}

	// Execute the query
	rows, err := db.DB.Query(query, trackID, minLat, maxLat, minLon, maxLon)
	if err != nil {
		return nil, fmt.Errorf("error querying markers by trackID and bounds: %v", err)
	}
	defer rows.Close()

	var markers []Marker

	// Iterate over the rows and scan each result into a Marker struct
	for rows.Next() {
		var marker Marker
		err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate, &marker.Zoom, &marker.Speed, &marker.TrackID)
		if err != nil {
			return nil, fmt.Errorf("error scanning marker: %v", err)
		}
		markers = append(markers, marker)
	}

	// Check for any errors encountered during iteration
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over result set: %v", err)
	}

	log.Printf("Found %d markers for TrackID=%s", len(markers), trackID)
	return markers, nil
}

// GetMarkersByTrackIDZoomAndBounds исправленный вариант
func (db *Database) GetMarkersByTrackIDZoomAndBounds(
	trackID string,
	zoom int,
	minLat, minLon, maxLat, maxLon float64,
	dbType string,
) ([]Marker, error) {

	var query string
	switch dbType {
	case "pgx": // PostgreSQL
		query = `
        SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
        FROM   markers
        WHERE  trackID = $1
          AND  zoom     = $2
          AND  lat BETWEEN $3 AND $4
          AND  lon BETWEEN $5 AND $6;`
	default: // SQLite / Genji
		query = `
        SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
        FROM   markers
        WHERE  trackID = ?
          AND  zoom     = ?
          AND  lat BETWEEN ? AND ?
          AND  lon BETWEEN ? AND ?;`
	}

	rows, err := db.DB.Query(query,
		trackID, zoom, minLat, maxLat, minLon, maxLon)
	if err != nil {
		return nil, fmt.Errorf("error querying markers: %w", err)
	}
	defer rows.Close()

	var markers []Marker
	for rows.Next() {
		var m Marker
		if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date,
			&m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID); err != nil {
			return nil, fmt.Errorf("error scanning marker: %w", err)
		}
		markers = append(markers, m)
	}
	return markers, rows.Err()
}

// SpeedRange задаёт замкнутый диапазон [Min, Max] скорости.
type SpeedRange struct{ Min, Max float64 }

// GetMarkersByZoomBoundsSpeed — z/bounds/date/speed фильтр.
// Мини-оптимизация: если несколько speedRanges образуют один непрерывный
// интервал, склеиваем их в ОДИН "speed BETWEEN lo AND hi", чтобы планировщик
// использовал составной индекс (zoom,lat,lon,speed).
func (db *Database) GetMarkersByZoomBoundsSpeed(
	zoom int,
	minLat, minLon, maxLat, maxLon float64,
	dateFrom, dateTo int64,
	speedRanges []SpeedRange,
	dbType string,
) ([]Marker, error) {

	ph := func(n int) string {
		if dbType == "pgx" {
			return fmt.Sprintf("$%d", n)
		}
		return "?"
	}

	var (
		sb   strings.Builder
		args []interface{}
	)

	// ---- обязательные условия -----------------------------------
	sb.WriteString("zoom = " + ph(len(args)+1))
	args = append(args, zoom)

	sb.WriteString(" AND lat BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
	args = append(args, minLat, maxLat)

	sb.WriteString(" AND lon BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
	args = append(args, minLon, maxLon)

	// ---- фильтр по времени (опционально) ------------------------
	if dateFrom > 0 {
		sb.WriteString(" AND date >= " + ph(len(args)+1))
		args = append(args, dateFrom)
	}
	if dateTo > 0 {
		sb.WriteString(" AND date <= " + ph(len(args)+1))
		args = append(args, dateTo)
	}

	// ---- скорость: склейка смежных диапазонов в один BETWEEN ----
	if len(speedRanges) > 0 {
		if ok, lo, hi := mergeContinuousRanges(speedRanges); ok {
			sb.WriteString(" AND speed BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
			args = append(args, lo, hi)
		} else {
			// Непрерывной склейки нет → оставляем OR-цепочку (старое поведение).
			sb.WriteString(" AND (")
			for i, r := range speedRanges {
				if i > 0 {
					sb.WriteString(" OR ")
				}
				sb.WriteString("speed BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
				args = append(args, r.Min, r.Max)
			}
			sb.WriteString(")")
		}
	}

	query := fmt.Sprintf(`SELECT id,doseRate,date,lon,lat,countRate,zoom,speed,trackID
	                      FROM markers WHERE %s;`, sb.String())

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var out []Marker
	for rows.Next() {
		var m Marker
		if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat,
			&m.CountRate, &m.Zoom, &m.Speed, &m.TrackID); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ------------------------------------------------------------------
// GetMarkersByTrackIDZoomBoundsSpeed
// ------------------------------------------------------------------
// Selects markers that belong to a single track, lie inside the given
// viewport, match the requested zoom level and fall into at least one
// of the supplied speed ranges.
//
//   - trackID         – UUID or any unique identifier of the track.
//   - zoom            – current Leaflet/XYZ-tile zoom level.
//   - minLat/minLon   – south-west corner of the requested bounding box.
//   - maxLat/maxLon   – north-east corner of the requested bounding box.
//   - speedRanges     – zero, one or many closed intervals [Min, Max] m/s.
//     Empty → no speed filter at all.
//   - dbType          – "pgx" → PostgreSQL dollar-placeholders ($1,$2,…),
//     anything else → use "?" (SQLite, Genji, MySQL…).
//
// The function never locks—concurrency is achieved by running each call
// in its own goroutine and passing the result through a channel if the
// caller needs to merge several queries.
//
// It relies solely on the standard         database/sql     package.
// Та же оптимизация: склейка непрерывных speedRanges в один BETWEEN.

func (db *Database) GetMarkersByTrackIDZoomBoundsSpeed(
	trackID string,
	zoom int,
	minLat, minLon, maxLat, maxLon float64,
	dateFrom, dateTo int64,
	speedRanges []SpeedRange,
	dbType string,
) ([]Marker, error) {

	ph := func(n int) string {
		if dbType == "pgx" {
			return fmt.Sprintf("$%d", n)
		}
		return "?"
	}

	var (
		sb   strings.Builder
		args []interface{}
	)

	sb.WriteString("trackID = " + ph(len(args)+1))
	args = append(args, trackID)

	sb.WriteString(" AND zoom = " + ph(len(args)+1))
	args = append(args, zoom)

	sb.WriteString(" AND lat BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
	args = append(args, minLat, maxLat)

	sb.WriteString(" AND lon BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
	args = append(args, minLon, maxLon)

	if dateFrom > 0 {
		sb.WriteString(" AND date >= " + ph(len(args)+1))
		args = append(args, dateFrom)
	}
	if dateTo > 0 {
		sb.WriteString(" AND date <= " + ph(len(args)+1))
		args = append(args, dateTo)
	}

	if len(speedRanges) > 0 {
		if ok, lo, hi := mergeContinuousRanges(speedRanges); ok {
			sb.WriteString(" AND speed BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
			args = append(args, lo, hi)
		} else {
			sb.WriteString(" AND (")
			for i, r := range speedRanges {
				if i > 0 {
					sb.WriteString(" OR ")
				}
				sb.WriteString("speed BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
				args = append(args, r.Min, r.Max)
			}
			sb.WriteString(")")
		}
	}

	query := fmt.Sprintf(`SELECT id,doseRate,date,lon,lat,countRate,zoom,speed,trackID
	                      FROM markers WHERE %s;`, sb.String())

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var out []Marker
	for rows.Next() {
		var m Marker
		if err := rows.Scan(&m.ID, &m.DoseRate, &m.Date, &m.Lon, &m.Lat,
			&m.CountRate, &m.Zoom, &m.Speed, &m.TrackID); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DetectExistingTrackID scans the first incoming markers and tries to
// recognise an already-stored track.
// If ≥ threshold identical markers (lat,lon,date,doseRate) point to the
// same TrackID, that TrackID is returned; otherwise an empty string is returned.
//
// – Works with any SQL driver: only simple equality, no vendor features.
// – No mutexes: each call owns its slice and DB handle is concurrency-safe.
// – Follows the “return early” advice: exits as soon as threshold is reached.
func (db *Database) DetectExistingTrackID(
	markers []Marker, // freshly-parsed markers (any zoom)
	threshold int, // how many identical points constitute identity
	dbType string, // "pgx" | "sqlite" | "genji" | …
) (string, error) {

	if len(markers) == 0 {
		return "", nil // nothing to compare
	}
	if threshold <= 0 {
		threshold = 10 // sane default
	}

	// Helper: build driver-neutral query text in one place.
	query := map[string]string{
		"pgx":   `SELECT trackID FROM markers WHERE lat=$1 AND lon=$2 AND date=$3 AND doseRate=$4 LIMIT 1`,
		"other": `SELECT trackID FROM markers WHERE lat=?  AND lon=?  AND date=? AND doseRate=?  LIMIT 1`,
	}
	q := query["other"]
	if dbType == "pgx" {
		q = query["pgx"]
	}

	hits := make(map[string]int) // TrackID → identical-points counter

	for _, m := range markers {
		var tid string
		err := db.DB.QueryRow(q, m.Lat, m.Lon, m.Date, m.DoseRate).Scan(&tid)
		switch {
		case err == sql.ErrNoRows:
			continue // no identical point yet → keep scanning
		case err != nil:
			return "", fmt.Errorf("DetectExistingTrackID: %w", err)
		default:
			hits[tid]++
			if hits[tid] >= threshold {
				return tid, nil // FOUND!
			}
		}
	}
	return "", nil // unique track — safe to create a new one
}

// mergeContinuousRanges tries to collapse several [Min,Max] speed ranges
// into a single continuous interval [lo,hi]. If there is any gap between
// input ranges, returns ok=false and the caller must fall back to "(... OR ...)".
//
// Why:
//   - A single "speed BETWEEN lo AND hi" helps the planner use a composite
//     (zoom,lat,lon,speed) index;
//   - The default UI state (car+ped) forms a continuous [0..70] interval,
//     so most queries become index-friendly immediately.
func mergeContinuousRanges(rr []SpeedRange) (ok bool, lo, hi float64) {
	if len(rr) == 0 {
		return false, 0, 0
	}
	// Work on a copy; keep caller's slice intact.
	r := append([]SpeedRange(nil), rr...)
	sort.Slice(r, func(i, j int) bool { return r[i].Min < r[j].Min })

	lo, hi = r[0].Min, r[0].Max
	for i := 1; i < len(r); i++ {
		// "Adjacent or overlapping" counts as continuous.
		// We allow tiny numeric jitter (eps) for floats.
		const eps = 1e-9
		if r[i].Min <= hi+eps {
			if r[i].Max > hi {
				hi = r[i].Max
			}
			continue
		}
		// Found a gap → cannot merge into a single interval.
		return false, 0, 0
	}
	return true, lo, hi
}
