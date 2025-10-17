package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Database represents the interface for interacting with the database.
type Database struct {
	DB          *sql.DB    // The underlying SQL database connection
	idGenerator chan int64 // Channel for generating unique IDs
	Driver      string     // Normalized driver name so SQL builders can stay declarative
}

// realtimeConverter is configured by the safecastrealtime package to translate
// raw Safecast units into µSv/h.  Keeping the dependency injected avoids an
// import cycle and mirrors the Go Proverb "The bigger the interface, the
// weaker the abstraction" by exposing only the function we need.
var realtimeConverter func(float64, string) (float64, bool)

// SetRealtimeConverter stores the helper used to convert Safecast realtime
// units.  We call it from main when the realtime feature is enabled so other
// database code can stay agnostic of specific detector logic.
func SetRealtimeConverter(fn func(float64, string) (float64, bool)) {
	realtimeConverter = fn
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
	DBType      string // The type of the database driver (e.g., "sqlite", "chai", or "pgx" (PostgreSQL))
	DBPath      string // The file path to the database file (for file-based databases)
	DBConn      string // Raw DSN for network drivers (pgx or clickhouse)
	DBHost      string // The host for PostgreSQL
	DBPort      int    // The port for PostgreSQL
	DBUser      string // The user for PostgreSQL
	DBPass      string // The password for PostgreSQL
	DBName      string // The name of the PostgreSQL database
	PGSSLMode   string // The SSL mode for PostgreSQL
	ClickSecure bool   // Enable TLS when connecting to ClickHouse over HTTP transport
	Port        int    // The port number (used in database file naming if needed)
}

// ClickHouseDSNFromConfig assembles a DSN understood by the lightweight HTTP driver.
// We parse host/port carefully so IPv6 literals keep their brackets intact.
func ClickHouseDSNFromConfig(cfg Config) string {
	if trimmed := strings.TrimSpace(cfg.DBConn); trimmed != "" {
		return trimmed
	}

	host := strings.TrimSpace(cfg.DBHost)
	if host == "" {
		host = "127.0.0.1"
	}

	if _, _, err := net.SplitHostPort(host); err != nil {
		port := cfg.DBPort
		if port <= 0 {
			port = 9000
		}
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}

	user := strings.TrimSpace(cfg.DBUser)
	pass := cfg.DBPass
	name := strings.Trim(strings.TrimSpace(cfg.DBName), "/")

	dsn := url.URL{Scheme: "clickhouse", Host: host}
	if user != "" {
		if strings.TrimSpace(pass) != "" {
			dsn.User = url.UserPassword(user, pass)
		} else {
			dsn.User = url.User(user)
		}
	}
	if name != "" {
		dsn.Path = "/" + name
	}

	params := url.Values{}
	if cfg.ClickSecure {
		params.Set("secure", "true")
	}
	dsn.RawQuery = params.Encode()
	return dsn.String()
}

// NewDatabase opens DB and configures connection pooling.
// For SQLite/Chai we force single-connection mode (no concurrent DB access).
func NewDatabase(config Config) (*Database, error) {
	driverName := strings.ToLower(strings.TrimSpace(config.DBType))
	var (
		dsn                string
		applySQLitePragmas bool
	)

	switch driverName {
	case "sqlite":
		applySQLitePragmas = true
		dsn = config.DBPath
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.%s", config.Port, driverName)
		}
	case "chai":
		// Chai is a separate driver that happens to reuse sqlite-style DSNs.
		// We still keep the single-connection behaviour but intentionally
		// skip SQLite-specific PRAGMA tuning so the driver can manage its
		// own transaction and caching strategy.
		dsn = config.DBPath
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.%s", config.Port, driverName)
		}
	case "duckdb":
		// файл создастся при первом открытии
		dsn = config.DBPath
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.duckdb", config.Port)
		}
	case "pgx":
		if strings.TrimSpace(config.DBConn) != "" {
			dsn = config.DBConn
		} else {
			dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
				config.DBUser, config.DBPass, config.DBHost, config.DBPort, config.DBName, config.PGSSLMode)
		}
	case "clickhouse":
		dsn = ClickHouseDSNFromConfig(config)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.DBType)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening the database: %v", err)
	}

	// === CRITICAL: serialize SQLite/Chai access over a single underlying connection ===
	switch driverName {
	case "sqlite", "chai":
		// One physical connection; no concurrent statements at DB layer.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		// Never recycle the single connection (keeps it stable for the whole process).
		db.SetConnMaxLifetime(0)
		// Tuning WAL/synchronous/busy_timeout keeps inserts fast enough for realtime uploads.
		if applySQLitePragmas {
			tuneCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := tuneSQLiteLikeConnection(tuneCtx, db, log.Printf); err != nil {
				log.Printf("sqlite tuning skipped: %v", err)
			}
			cancel()
		} else {
			log.Printf("sqlite tuning skipped: driver %s manages pragmas itself", driverName)
		}
	case "duckdb":
		// DuckDB performs all writes through a single transaction log and does not
		// currently benefit from multiple concurrent writers.  We cap it to one
		// connection so realtime refreshes avoid unique-key races while staying in
		// line with "Clear is better than clever".
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(0)
	case "clickhouse":
		// ClickHouse benefits from a few parallel connections while remaining lightweight.
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(8)
		db.SetConnMaxLifetime(5 * time.Minute)
		db.SetConnMaxIdleTime(2 * time.Minute)
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

	log.Printf("Using database driver: %s with DSN: %s", driverName, dsn)

	// Bootstrap ID generator from the highest ID across tables so each row
	// receives a unique primary key. We query both markers and realtime data
	// because the generator is shared. Errors are ignored to keep startup
	// robust even when tables are missing.
	var (
		maxMarkers  sql.NullInt64
		maxRealtime sql.NullInt64
	)
	_ = db.QueryRow(`SELECT MAX(id) FROM markers`).Scan(&maxMarkers)
	_ = db.QueryRow(`SELECT MAX(id) FROM realtime_measurements`).Scan(&maxRealtime)
	initialID := int64(1)
	if maxMarkers.Valid && maxMarkers.Int64 >= initialID {
		initialID = maxMarkers.Int64 + 1
	}
	if maxRealtime.Valid && maxRealtime.Int64 >= initialID {
		initialID = maxRealtime.Int64 + 1
	}
	idChannel := startIDGenerator(initialID)

	return &Database{
		DB:          db,
		idGenerator: idChannel,
		Driver:      driverName,
	}, nil
}

// tuneSQLiteLikeConnection applies WAL/synchronous/busy pragmas for SQLite-like engines.
// We keep the steps portable and run them through a small channel pipeline so the
// work happens outside the caller goroutine, following "Don't communicate by sharing
// memory; share memory by communicating".
func tuneSQLiteLikeConnection(ctx context.Context, db *sql.DB, logf func(string, ...any)) error {
	type pragma struct {
		label     string
		query     string
		expectRow bool
	}

	steps := []pragma{
		{label: "journal_mode", query: "PRAGMA journal_mode=WAL;", expectRow: true},
		{label: "synchronous", query: "PRAGMA synchronous=NORMAL;"},
		{label: "temp_store", query: "PRAGMA temp_store=MEMORY;"},
		{label: "cache_size", query: "PRAGMA cache_size=-20000;"},
		{label: "busy_timeout", query: "PRAGMA busy_timeout=5000;"},
	}

	jobs := make(chan pragma)
	errs := make(chan error, 1)

	go func() {
		defer close(errs)
		for step := range jobs {
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			default:
			}

			if step.expectRow {
				var mode string
				if err := db.QueryRowContext(ctx, step.query).Scan(&mode); err != nil {
					errs <- fmt.Errorf("apply %s: %w", step.label, err)
					return
				}
				logf("SQLite tuning %s -> %s", step.label, mode)
				continue
			}

			if _, err := db.ExecContext(ctx, step.query); err != nil {
				errs <- fmt.Errorf("apply %s: %w", step.label, err)
				return
			}
			logf("SQLite tuning %s applied", step.label)
		}
		errs <- nil
	}()

	go func() {
		defer close(jobs)
		for _, step := range steps {
			jobs <- step
		}
	}()

	if err := <-errs; err != nil {
		return err
	}
	return nil
}

// EnsureIndexesAsync builds non-critical indexes in background, politely.
// - No pinned connections (important for sqlite/chai with MaxOpenConns(1)).
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

			// polite retry loop for SQLite/Chai "busy"/locks; portable for others too
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
			// Date-first variant accelerates archive/year pagination that filters primarily by time.
			{"idx_markers_date_trackid",
				`CREATE INDEX IF NOT EXISTS idx_markers_date_trackid ON markers (date, trackID)`},
			{"idx_markers_date_trackid_id",
				`CREATE INDEX IF NOT EXISTS idx_markers_date_trackid_id ON markers (date, trackID, id)`},
			// Dedicated date helpers keep slider filtering responsive even with WAL on.
			{"idx_markers_trackid_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_date ON markers (trackID, date)`},
			{"idx_markers_trackid_id",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_id ON markers (trackID, id)`},
			{"idx_markers_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_date ON markers (date)`},
			{"idx_markers_zoom_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_date ON markers (zoom, date)`},
			{"idx_markers_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_speed ON markers (speed)`},
			// Realtime history: keep per-device scans and bounds responsive.
			{"idx_realtime_device_fetched",
				`CREATE INDEX IF NOT EXISTS idx_realtime_device_fetched ON realtime_measurements (device_id, fetched_at)`},
			{"idx_realtime_bounds",
				`CREATE INDEX IF NOT EXISTS idx_realtime_bounds ON realtime_measurements (lat, lon, fetched_at)`},
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
			// Date-first variant accelerates archive/year pagination that filters primarily by time.
			{"idx_markers_date_trackid",
				`CREATE INDEX IF NOT EXISTS idx_markers_date_trackid ON markers (date, trackID)`},
			{"idx_markers_date_trackid_id",
				`CREATE INDEX IF NOT EXISTS idx_markers_date_trackid_id ON markers (date, trackID, id)`},
			// Dedicated date helpers keep slider filtering responsive even with WAL on.
			{"idx_markers_trackid_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_date ON markers (trackID, date)`},
			{"idx_markers_trackid_id",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_id ON markers (trackID, id)`},
			{"idx_markers_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_date ON markers (date)`},
			{"idx_markers_zoom_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_date ON markers (zoom, date)`},
			{"idx_markers_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_speed ON markers (speed)`},
			// Realtime history: keep per-device scans and bounds responsive.
			{"idx_realtime_device_fetched",
				`CREATE INDEX IF NOT EXISTS idx_realtime_device_fetched ON realtime_measurements (device_id, fetched_at)`},
			{"idx_realtime_bounds",
				`CREATE INDEX IF NOT EXISTS idx_realtime_bounds ON realtime_measurements (lat, lon, fetched_at)`},
		}

	case "sqlite", "chai":
		// SQLite/Chai: keep a UNIQUE index (no table-level UNIQUE constraint there).
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
			// Date-first variant accelerates archive/year pagination that filters primarily by time.
			{"idx_markers_date_trackid",
				`CREATE INDEX IF NOT EXISTS idx_markers_date_trackid ON markers (date, trackID)`},
			{"idx_markers_date_trackid_id",
				`CREATE INDEX IF NOT EXISTS idx_markers_date_trackid_id ON markers (date, trackID, id)`},
			// Dedicated date helpers keep slider filtering responsive even with WAL on.
			{"idx_markers_trackid_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_date ON markers (trackID, date)`},
			{"idx_markers_trackid_id",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_id ON markers (trackID, id)`},
			{"idx_markers_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_date ON markers (date)`},
			{"idx_markers_zoom_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_date ON markers (zoom, date)`},
			{"idx_markers_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_speed ON markers (speed)`},
			// Realtime history: keep per-device scans and bounds responsive.
			{"idx_realtime_device_fetched",
				`CREATE INDEX IF NOT EXISTS idx_realtime_device_fetched ON realtime_measurements (device_id, fetched_at)`},
			{"idx_realtime_bounds",
				`CREATE INDEX IF NOT EXISTS idx_realtime_bounds ON realtime_measurements (lat, lon, fetched_at)`},
		}

	case "clickhouse":
		// MergeTree handles ordering internally; explicit secondary indexes are unnecessary here.
		return nil

	default:
		// Fallback: behave like SQLite/Chai (portable everywhere that supports IF NOT EXISTS).
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
			// Date-first variant accelerates archive/year pagination that filters primarily by time.
			{"idx_markers_date_trackid",
				`CREATE INDEX IF NOT EXISTS idx_markers_date_trackid ON markers (date, trackID)`},
			{"idx_markers_date_trackid_id",
				`CREATE INDEX IF NOT EXISTS idx_markers_date_trackid_id ON markers (date, trackID, id)`},
			// Dedicated date helpers keep slider filtering responsive even with WAL on.
			{"idx_markers_trackid_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_date ON markers (trackID, date)`},
			{"idx_markers_trackid_id",
				`CREATE INDEX IF NOT EXISTS idx_markers_trackid_id ON markers (trackID, id)`},
			{"idx_markers_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_date ON markers (date)`},
			{"idx_markers_zoom_date",
				`CREATE INDEX IF NOT EXISTS idx_markers_zoom_date ON markers (zoom, date)`},
			{"idx_markers_speed",
				`CREATE INDEX IF NOT EXISTS idx_markers_speed ON markers (speed)`},
			// Realtime history: keep per-device scans and bounds responsive.
			{"idx_realtime_device_fetched",
				`CREATE INDEX IF NOT EXISTS idx_realtime_device_fetched ON realtime_measurements (device_id, fetched_at)`},
			{"idx_realtime_bounds",
				`CREATE INDEX IF NOT EXISTS idx_realtime_bounds ON realtime_measurements (lat, lon, fetched_at)`},
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

	case "sqlite", "chai":
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
	var (
		schema     string
		statements []string
	)

	switch strings.ToLower(cfg.DBType) {
	case "pgx":
		// PostgreSQL — standard types, named UNIQUE to target by ON CONFLICT
		schema = `
CREATE TABLE IF NOT EXISTS markers (
  id          BIGSERIAL PRIMARY KEY,
  doseRate    DOUBLE PRECISION,
  date        BIGINT,
  lon         DOUBLE PRECISION,
  lat         DOUBLE PRECISION,
  countRate   DOUBLE PRECISION,
  zoom        INTEGER,
  speed       DOUBLE PRECISION,
  trackID     TEXT,
  altitude    DOUBLE PRECISION,
  detector    TEXT,
  radiation   TEXT,
  temperature DOUBLE PRECISION,
  humidity    DOUBLE PRECISION,
  CONSTRAINT markers_unique UNIQUE (doseRate,date,lon,lat,countRate,zoom,speed,trackID)
);

CREATE TABLE IF NOT EXISTS realtime_measurements (
  id          BIGSERIAL PRIMARY KEY,
  device_id   TEXT,
  transport   TEXT,
  device_name TEXT,
  tube        TEXT,
  country     TEXT,
  value       DOUBLE PRECISION,
  unit        TEXT,
  lat         DOUBLE PRECISION,
  lon         DOUBLE PRECISION,
  measured_at BIGINT,
  fetched_at  BIGINT,
  extra       TEXT,
  CONSTRAINT realtime_unique UNIQUE (device_id,measured_at)
);

CREATE TABLE IF NOT EXISTS short_links (
  id         BIGSERIAL PRIMARY KEY,
  code       TEXT UNIQUE NOT NULL,
  target     TEXT UNIQUE NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_short_links_target_lookup
  ON short_links (target);
CREATE INDEX IF NOT EXISTS idx_short_links_created
  ON short_links (created_at);
`

	case "sqlite", "chai":
		// Portable SQLite/Chai side — explicit INTEGER PK
		schema = `
CREATE TABLE IF NOT EXISTS markers (
  id          INTEGER PRIMARY KEY,
  doseRate    REAL,
  date        BIGINT,
  lon         REAL,
  lat         REAL,
  countRate   REAL,
  zoom        INTEGER,
  speed       REAL,
  trackID     TEXT,
  altitude    REAL,
  detector    TEXT,
  radiation   TEXT,
  temperature REAL,
  humidity    REAL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_markers_unique
  ON markers (doseRate,date,lon,lat,countRate,zoom,speed,trackID);

CREATE TABLE IF NOT EXISTS realtime_measurements (
  id          INTEGER PRIMARY KEY,
  device_id   TEXT,
  transport   TEXT,
  device_name TEXT,
  tube        TEXT,
  country     TEXT,
  value       REAL,
  unit        TEXT,
  lat         REAL,
  lon         REAL,
  measured_at BIGINT,
  fetched_at  BIGINT,
  extra       TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_realtime_unique
  ON realtime_measurements (device_id,measured_at);

CREATE TABLE IF NOT EXISTS short_links (
  id         INTEGER PRIMARY KEY,
  code       TEXT NOT NULL UNIQUE,
  target     TEXT NOT NULL UNIQUE,
  created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_short_links_target_lookup
  ON short_links (target);
CREATE INDEX IF NOT EXISTS idx_short_links_created
  ON short_links (created_at);
`

	case "duckdb":
		// DuckDB — no SERIAL/AUTOINCREMENT; use a sequence + DEFAULT nextval(...).
		// Both CREATE SEQUENCE IF NOT EXISTS and ON CONFLICT are supported.
		// We also keep a named UNIQUE to match our upsert policy.
		// Ref: DuckDB docs for sequences & ON CONFLICT.
		schema = `
CREATE SEQUENCE IF NOT EXISTS markers_id_seq START 1;
CREATE TABLE IF NOT EXISTS markers (
  id          BIGINT PRIMARY KEY DEFAULT nextval('markers_id_seq'),
  doseRate    DOUBLE,
  date        BIGINT,
  lon         DOUBLE,
  lat         DOUBLE,
  countRate   DOUBLE,
  zoom        INTEGER,
  speed       DOUBLE,
  trackID     TEXT,
  altitude    DOUBLE,
  detector    TEXT,
  radiation   TEXT,
  temperature DOUBLE,
  humidity    DOUBLE,
  CONSTRAINT markers_unique UNIQUE (doseRate,date,lon,lat,countRate,zoom,speed,trackID)
);

CREATE SEQUENCE IF NOT EXISTS realtime_measurements_id_seq START 1;
CREATE TABLE IF NOT EXISTS realtime_measurements (
  id          BIGINT PRIMARY KEY DEFAULT nextval('realtime_measurements_id_seq'),
  device_id   TEXT,
  transport   TEXT,
  device_name TEXT,
  tube        TEXT,
  country     TEXT,
  value       DOUBLE,
  unit        TEXT,
  lat         DOUBLE,
  lon         DOUBLE,
  measured_at BIGINT,
  fetched_at  BIGINT,
  extra       TEXT,
  CONSTRAINT realtime_unique UNIQUE (device_id,measured_at)
);

CREATE SEQUENCE IF NOT EXISTS short_links_id_seq START 1;
CREATE TABLE IF NOT EXISTS short_links (
  id         BIGINT PRIMARY KEY DEFAULT nextval('short_links_id_seq'),
  code       TEXT UNIQUE NOT NULL,
  target     TEXT UNIQUE NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_short_links_target_lookup
  ON short_links (target);
CREATE INDEX IF NOT EXISTS idx_short_links_created
  ON short_links (created_at);
`

	case "clickhouse":
		statements = []string{
			`CREATE TABLE IF NOT EXISTS markers (
  id          UInt64,
  doseRate    Float64,
  date        Int64,
  lon         Float64,
  lat         Float64,
  countRate   Float64,
  zoom        Int32,
  speed       Float64,
  trackID     String,
  altitude    Float64,
  detector    String,
  radiation   String,
  temperature Float64,
  humidity    Float64
) ENGINE = MergeTree()
ORDER BY (trackID, date, id);`,
			`CREATE TABLE IF NOT EXISTS realtime_measurements (
  id          UInt64,
  device_id   String,
  transport   String,
  device_name String,
  tube        String,
  country     String,
  value       Float64,
  unit        String,
  lat         Float64,
  lon         Float64,
  measured_at Int64,
  fetched_at  Int64,
  extra       String
) ENGINE = MergeTree()
ORDER BY (device_id, measured_at);`,
			`CREATE TABLE IF NOT EXISTS short_links (
  id         UInt64,
  code       String,
  target     String,
  created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (code);`,
		}

	default:
		return fmt.Errorf("unsupported database type: %s", cfg.DBType)
	}

	if len(statements) > 0 {
		if err := execStatements(db.DB, statements); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	} else {
		if _, err := db.DB.Exec(schema); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}

	// Ensure optional columns exist even for older databases.
	if err := db.ensureMarkerMetadataColumns(cfg.DBType); err != nil {
		return fmt.Errorf("add marker metadata column: %w", err)
	}
	if err := db.ensureRealtimeMetadataColumns(cfg.DBType); err != nil {
		return fmt.Errorf("add realtime metadata column: %w", err)
	}

	return nil
}

// execStatements executes a slice of DDL statements sequentially so engines that
// do not support multi-statement Exec calls (e.g. ClickHouse HTTP) still boot
// correctly. We trim whitespace to stay tolerant of blank entries.
func execStatements(db *sql.DB, stmts []string) error {
	for _, raw := range stmts {
		stmt := strings.TrimSpace(raw)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// ensureRealtimeMetadataColumns upgrades realtime_measurements with optional metadata columns.
// Each column is added lazily so existing installations keep their history without manual SQL.
// ensureMarkerMetadataColumns upgrades the markers table with optional telemetry columns.
// We add altitude, detector type, radiation channel, temperature, and humidity lazily so
// historical databases built before this format continue working without manual SQL.
func (db *Database) ensureMarkerMetadataColumns(dbType string) error {
	type column struct {
		name string
		def  string
	}
	required := []column{
		{name: "altitude", def: "altitude DOUBLE PRECISION"},
		{name: "detector", def: "detector TEXT"},
		{name: "radiation", def: "radiation TEXT"},
		{name: "temperature", def: "temperature DOUBLE PRECISION"},
		{name: "humidity", def: "humidity DOUBLE PRECISION"},
	}

	switch strings.ToLower(dbType) {
	case "pgx", "duckdb":
		for _, col := range required {
			stmt := fmt.Sprintf("ALTER TABLE markers ADD COLUMN IF NOT EXISTS %s", col.def)
			if _, err := db.DB.Exec(stmt); err != nil {
				return err
			}
		}
		return nil

	case "clickhouse":
		// ClickHouse schema already ships with the extended columns, so no ALTER needed.
		return nil

	default:
		rows, err := db.DB.Query(`PRAGMA table_info(markers);`)
		if err != nil {
			return fmt.Errorf("describe markers: %w", err)
		}
		defer rows.Close()

		present := make(map[string]bool)
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
				return fmt.Errorf("scan markers pragma: %w", err)
			}
			present[name] = true
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate markers pragma: %w", err)
		}

		for _, col := range required {
			if present[col.name] {
				continue
			}
			stmt := fmt.Sprintf("ALTER TABLE markers ADD COLUMN %s", col.def)
			if _, err := db.DB.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	}
}

func (db *Database) ensureRealtimeMetadataColumns(dbType string) error {
	type column struct {
		name string
		def  string
	}
	required := []column{
		{name: "transport", def: "transport TEXT"},
		{name: "device_name", def: "device_name TEXT"},
		{name: "tube", def: "tube TEXT"},
		{name: "country", def: "country TEXT"},
		{name: "extra", def: "extra TEXT"},
	}

	switch strings.ToLower(dbType) {
	case "pgx", "duckdb":
		// Engines with IF NOT EXISTS syntax can add columns individually without prior inspection.
		for _, col := range required {
			stmt := fmt.Sprintf("ALTER TABLE realtime_measurements ADD COLUMN IF NOT EXISTS %s", col.def)
			if _, err := db.DB.Exec(stmt); err != nil {
				return err
			}
		}
		return nil

	case "clickhouse":
		// Tables created for ClickHouse already contain the optional metadata columns.
		return nil

	default:
		// SQLite-style engines require manual detection before issuing ALTER TABLE statements.
		rows, err := db.DB.Query(`PRAGMA table_info(realtime_measurements);`)
		if err != nil {
			return fmt.Errorf("describe realtime_measurements: %w", err)
		}
		defer rows.Close()

		present := make(map[string]bool)
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
			present[name] = true
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate pragma: %w", err)
		}

		for _, col := range required {
			if present[col.name] {
				continue
			}
			stmt := fmt.Sprintf("ALTER TABLE realtime_measurements ADD COLUMN %s", col.def)
			if _, err := db.DB.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
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

	var exec sqlExecutor
	if tx != nil {
		exec = tx
	} else {
		exec = db.DB
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
		args := make([]interface{}, 0, len(chunk)*14) // 14 covers SQLite/Chai worst-case with id

		switch strings.ToLower(dbType) {
		case "pgx":
			// PostgreSQL: BIGSERIAL fills id, so we only ship the payload columns.
			sb.WriteString("INSERT INTO markers (doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity) VALUES ")
			argn := 0
			const cols = 13
			for j, m := range chunk {
				if j > 0 {
					sb.WriteString(",")
				}
				sb.WriteString("(")
				for k := 0; k < cols; k++ {
					if k > 0 {
						sb.WriteString(",")
					}
					argn++
					sb.WriteString(ph(argn))
				}
				sb.WriteString(")")
				args = append(args,
					m.DoseRate, m.Date, m.Lon, m.Lat,
					m.CountRate, m.Zoom, m.Speed, m.TrackID,
					nullableFloat64(m.AltitudeValid, m.Altitude),
					m.Detector, m.Radiation,
					nullableFloat64(m.TemperatureValid, m.Temperature),
					nullableFloat64(m.HumidityValid, m.Humidity),
				)
			}
			sb.WriteString(" ON CONFLICT ON CONSTRAINT markers_unique DO NOTHING")

		case "clickhouse":
			usable := make([]Marker, 0, len(chunk))
			for j := range chunk {
				idx := i + j
				probe := markers[idx]
				exists, err := db.markerExistsClickHouse(probe)
				if err != nil {
					return fmt.Errorf("clickhouse marker exists check: %w", err)
				}
				if exists {
					continue
				}
				if probe.ID == 0 {
					probe.ID = <-db.idGenerator
					markers[idx].ID = probe.ID
				}
				usable = append(usable, probe)
			}
			if len(usable) == 0 {
				i = end
				continue
			}

			sb.WriteString("INSERT INTO markers (id,doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity) VALUES ")
			argn := 0
			const cols = 14
			for j, m := range usable {
				if j > 0 {
					sb.WriteString(",")
				}
				sb.WriteString("(")
				for k := 0; k < cols; k++ {
					if k > 0 {
						sb.WriteString(",")
					}
					argn++
					sb.WriteString(ph(argn))
				}
				sb.WriteString(")")
				args = append(args,
					m.ID, m.DoseRate, m.Date, m.Lon, m.Lat,
					m.CountRate, m.Zoom, m.Speed, m.TrackID,
					nullableFloat64(m.AltitudeValid, m.Altitude),
					m.Detector, m.Radiation,
					nullableFloat64(m.TemperatureValid, m.Temperature),
					nullableFloat64(m.HumidityValid, m.Humidity),
				)
			}

		default:
			// SQLite / Chai: keep explicit ids to avoid PRIMARY KEY clashes when aggregating zooms.
			sb.WriteString("INSERT INTO markers (id,doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity) VALUES ")
			argn := 0
			const cols = 14
			for j, m := range chunk {
				if j > 0 {
					sb.WriteString(",")
				}
				if m.ID == 0 {
					m.ID = <-db.idGenerator
					chunk[j].ID = m.ID
				}
				sb.WriteString("(")
				for k := 0; k < cols; k++ {
					if k > 0 {
						sb.WriteString(",")
					}
					argn++
					sb.WriteString(ph(argn))
				}
				sb.WriteString(")")
				args = append(args,
					m.ID, m.DoseRate, m.Date, m.Lon, m.Lat,
					m.CountRate, m.Zoom, m.Speed, m.TrackID,
					nullableFloat64(m.AltitudeValid, m.Altitude),
					m.Detector, m.Radiation,
					nullableFloat64(m.TemperatureValid, m.Temperature),
					nullableFloat64(m.HumidityValid, m.Humidity),
				)
			}
			sb.WriteString(" ON CONFLICT DO NOTHING")
		}

		if sb.Len() == 0 {
			i = end
			continue
		}
		if _, err := exec.Exec(sb.String(), args...); err != nil {
			return fmt.Errorf("bulk exec: %w", err)
		}
		i = end
	}
	return nil
}

// SaveMarkerAtomic inserts a marker and silently ignores duplicates.
//
//   - PostgreSQL (pgx) – опираемся на BIGSERIAL, id не передаём;
//   - SQLite и Chai   – если id == 0, берём следующий из idGenerator.
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
      (doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT ON CONSTRAINT markers_unique DO NOTHING`,
			m.DoseRate, m.Date, m.Lon, m.Lat,
			m.CountRate, m.Zoom, m.Speed, m.TrackID,
			nullableFloat64(m.AltitudeValid, m.Altitude),
			m.Detector, m.Radiation,
			nullableFloat64(m.TemperatureValid, m.Temperature),
			nullableFloat64(m.HumidityValid, m.Humidity))
		return err

	case "clickhouse":
		exists, err := db.markerExistsClickHouse(m)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		if m.ID == 0 {
			m.ID = <-db.idGenerator
		}
		_, err = exec.Exec(`
INSERT INTO markers
      (id,doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			m.ID, m.DoseRate, m.Date, m.Lon, m.Lat,
			m.CountRate, m.Zoom, m.Speed, m.TrackID,
			nullableFloat64(m.AltitudeValid, m.Altitude),
			m.Detector, m.Radiation,
			nullableFloat64(m.TemperatureValid, m.Temperature),
			nullableFloat64(m.HumidityValid, m.Humidity))
		return err

	// ─────────────────────── SQLite / Chai / другие ─────────
	default:
		if m.ID == 0 {
			// берём следующий уникальный id из генератора
			m.ID = <-db.idGenerator
		}
		_, err := exec.Exec(`
INSERT INTO markers
      (id,doseRate,date,lon,lat,countRate,zoom,speed,trackID,altitude,detector,radiation,temperature,humidity)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT DO NOTHING`,
			m.ID, m.DoseRate, m.Date, m.Lon, m.Lat,
			m.CountRate, m.Zoom, m.Speed, m.TrackID,
			nullableFloat64(m.AltitudeValid, m.Altitude),
			m.Detector, m.Radiation,
			nullableFloat64(m.TemperatureValid, m.Temperature),
			nullableFloat64(m.HumidityValid, m.Humidity))
		return err
	}
}

// nullableFloat64 converts optional measurement fields into SQL-friendly values so we
// persist NULL when a sensor never reported altitude, temperature, or humidity.
func nullableFloat64(valid bool, value float64) any {
	if !valid {
		return nil
	}
	return value
}

// markerExistsClickHouse checks for duplicates because MergeTree tables do not enforce unique keys.
// We rely on the same composite uniqueness used for SQLite to keep behaviour consistent.
func (db *Database) markerExistsClickHouse(m Marker) (bool, error) {
	if db == nil || db.DB == nil {
		return false, fmt.Errorf("database unavailable")
	}
	const q = `SELECT 1 FROM markers WHERE doseRate = ? AND date = ? AND lon = ? AND lat = ? AND countRate = ? AND zoom = ? AND speed = ? AND trackID = ? LIMIT 1`
	var one int
	err := db.DB.QueryRow(q,
		m.DoseRate, m.Date, m.Lon, m.Lat,
		m.CountRate, m.Zoom, m.Speed, m.TrackID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// realtimeExistsClickHouse prevents duplicate realtime rows without relying on UNIQUE constraints.
func (db *Database) realtimeExistsClickHouse(deviceID string, measuredAt int64) (bool, error) {
	if db == nil || db.DB == nil {
		return false, fmt.Errorf("database unavailable")
	}
	const q = `SELECT 1 FROM realtime_measurements WHERE device_id = ? AND measured_at = ? LIMIT 1`
	var one int
	err := db.DB.QueryRow(q, deviceID, measuredAt).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// duckDBIsConflict detects duplicate-key errors reported by DuckDB so callers can retry safely.
// DuckDB tends to use user-facing phrases like "duplicate key" inside a Constraint Error message.
func duckDBIsConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "constraint error")
}

// InsertRealtimeMeasurement stores live device data and skips duplicates.
// A little copying is better than a little dependency, so we build SQL by hand.
func (db *Database) InsertRealtimeMeasurement(m RealtimeMeasurement, dbType string) error {
	switch strings.ToLower(dbType) {
	case "pgx":
		_, err := db.DB.Exec(`
INSERT INTO realtime_measurements
      (device_id,transport,device_name,tube,country,value,unit,lat,lon,measured_at,fetched_at,extra)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT ON CONSTRAINT realtime_unique DO NOTHING`,
			m.DeviceID, m.Transport, m.DeviceName, m.Tube, m.Country,
			m.Value, m.Unit, m.Lat, m.Lon, m.MeasuredAt, m.FetchedAt, m.Extra)
		return err

	case "duckdb":
		// DuckDB currently refuses to update indexed columns inside an ON CONFLICT branch.
		// To keep markers fresh we replace the row inside a transaction: delete the stale
		// entry and insert the latest reading.  Concurrent writers may collide, so we
		// retry politely when DuckDB reports a duplicate-key race, mirroring "Don't panic"
		// by keeping the logic straightforward and resilient.
		const maxDuckDBRetries = 3
		var lastConflict error

		for attempt := 0; attempt < maxDuckDBRetries; attempt++ {
			tx, err := db.DB.BeginTx(context.Background(), nil)
			if err != nil {
				return fmt.Errorf("begin duckdb realtime tx: %w", err)
			}

			if _, execErr := tx.Exec(`DELETE FROM realtime_measurements WHERE device_id = ? AND measured_at = ?`, m.DeviceID, m.MeasuredAt); execErr != nil {
				_ = tx.Rollback()
				return fmt.Errorf("duckdb delete realtime: %w", execErr)
			}

			if _, execErr := tx.Exec(`
INSERT INTO realtime_measurements
      (device_id,transport,device_name,tube,country,value,unit,lat,lon,measured_at,fetched_at,extra)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				m.DeviceID, m.Transport, m.DeviceName, m.Tube, m.Country,
				m.Value, m.Unit, m.Lat, m.Lon, m.MeasuredAt, m.FetchedAt, m.Extra); execErr != nil {
				_ = tx.Rollback()
				if duckDBIsConflict(execErr) {
					lastConflict = execErr
					continue
				}
				return fmt.Errorf("duckdb insert realtime: %w", execErr)
			}

			if commitErr := tx.Commit(); commitErr != nil {
				_ = tx.Rollback()
				if duckDBIsConflict(commitErr) {
					lastConflict = commitErr
					continue
				}
				return fmt.Errorf("duckdb commit realtime: %w", commitErr)
			}
			return nil
		}
		if lastConflict != nil {
			return fmt.Errorf("duckdb realtime conflict after retries: %w", lastConflict)
		}
		return fmt.Errorf("duckdb realtime conflict without error context")

	case "clickhouse":
		exists, err := db.realtimeExistsClickHouse(m.DeviceID, m.MeasuredAt)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		if m.ID == 0 {
			m.ID = <-db.idGenerator
		}
		_, err = db.DB.Exec(`
INSERT INTO realtime_measurements
      (id,device_id,transport,device_name,tube,country,value,unit,lat,lon,measured_at,fetched_at,extra)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			m.ID, m.DeviceID, m.Transport, m.DeviceName, m.Tube, m.Country,
			m.Value, m.Unit, m.Lat, m.Lon, m.MeasuredAt, m.FetchedAt, m.Extra)
		return err

	default:
		if m.ID == 0 {
			m.ID = <-db.idGenerator
		}
		_, err := db.DB.Exec(`
INSERT INTO realtime_measurements
      (id,device_id,transport,device_name,tube,country,value,unit,lat,lon,measured_at,fetched_at,extra)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(device_id,measured_at) DO NOTHING`,
			m.ID, m.DeviceID, m.Transport, m.DeviceName, m.Tube, m.Country,
			m.Value, m.Unit, m.Lat, m.Lon, m.MeasuredAt, m.FetchedAt, m.Extra)
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
SELECT device_id,transport,device_name,tube,country,value,unit,lat,lon,measured_at,extra
FROM realtime_measurements
WHERE lat BETWEEN $1 AND $2 AND lon BETWEEN $3 AND $4
ORDER BY device_id,fetched_at DESC;`
	default:
		query = `
SELECT device_id,transport,device_name,tube,country,value,unit,lat,lon,measured_at,extra
FROM realtime_measurements
WHERE lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?
ORDER BY device_id,fetched_at DESC;`
	}

	rows, err := db.DB.Query(query, minLat, maxLat, minLon, maxLon)
	if err != nil {
		return nil, fmt.Errorf("query realtime: %w", err)
	}
	defer rows.Close()

	now := time.Now().Unix()
	const daySeconds = int64((24 * time.Hour) / time.Second)

	seen := make(map[string]bool)
	var out []Marker
	for rows.Next() {
		var (
			id, transport, name, tube, country, extraRaw string
			val                                          float64
			unit                                         string
			lat, lon                                     float64
			measured                                     int64
		)
		if err := rows.Scan(&id, &transport, &name, &tube, &country, &val, &unit, &lat, &lon, &measured, &extraRaw); err != nil {
			return nil, fmt.Errorf("scan realtime: %w", err)
		}
		if lat == 0 && lon == 0 {
			continue // skip bogus locations at the equator
		}
		if val <= 0 {
			continue // ignore non-positive readings
		}
		if now-measured > daySeconds {
			continue // drop devices that have been silent for more than a day
		}
		if seen[id] {
			continue // keep the newest reading only once per device
		}

		// Convert raw CPM into µSv/h; unsupported units stay hidden to
		// avoid misreporting dose rates on the map.
		var (
			doseRate float64
			ok       bool
		)
		if realtimeConverter != nil {
			doseRate, ok = realtimeConverter(val, unit)
		}
		if !ok {
			continue
		}

		var extras map[string]float64
		trimmed := strings.TrimSpace(extraRaw)
		if trimmed != "" {
			// Parsing happens lazily to avoid overhead for historical rows without metadata.
			if err := json.Unmarshal([]byte(trimmed), &extras); err != nil {
				log.Printf("parse realtime extra for %s: %v", id, err)
			}
		}

		seen[id] = true
		marker := Marker{
			DoseRate:   doseRate,
			Date:       measured,
			Lon:        lon,
			Lat:        lat,
			CountRate:  0,
			Zoom:       0,
			Speed:      -1,           // negative speed marks realtime in UI
			TrackID:    "live:" + id, // prefix avoids clashing with stored tracks
			DeviceID:   id,
			DeviceName: name,
			Transport:  transport,
			Tube:       tube,
			Country:    country,
			LiveExtra:  extras,
		}
		out = append(out, marker)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate realtime: %w", err)
	}
	return out, nil
}

// GetRealtimeHistory returns all realtime measurements for a device since the requested timestamp.
// Callers can reuse the raw transport/name metadata to describe the sensor while charting the values.
func (db *Database) GetRealtimeHistory(deviceID string, since int64, dbType string) ([]RealtimeMeasurement, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("device id required")
	}

	var query string
	switch strings.ToLower(dbType) {
	case "pgx", "duckdb":
		query = `
SELECT device_id,transport,device_name,tube,country,value,unit,lat,lon,measured_at,fetched_at,extra
FROM realtime_measurements
WHERE device_id = $1 AND measured_at >= $2
ORDER BY measured_at ASC;`
	default:
		query = `
SELECT device_id,transport,device_name,tube,country,value,unit,lat,lon,measured_at,fetched_at,extra
FROM realtime_measurements
WHERE device_id = ? AND measured_at >= ?
ORDER BY measured_at ASC;`
	}

	rows, err := db.DB.Query(query, deviceID, since)
	if err != nil {
		return nil, fmt.Errorf("realtime history: %w", err)
	}
	defer rows.Close()

	var out []RealtimeMeasurement
	for rows.Next() {
		var m RealtimeMeasurement
		if err := rows.Scan(
			&m.DeviceID,
			&m.Transport,
			&m.DeviceName,
			&m.Tube,
			&m.Country,
			&m.Value,
			&m.Unit,
			&m.Lat,
			&m.Lon,
			&m.MeasuredAt,
			&m.FetchedAt,
			&m.Extra,
		); err != nil {
			return nil, fmt.Errorf("scan realtime history: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate realtime history: %w", err)
	}
	return out, nil
}

// PromoteStaleRealtime moves device histories from the realtime table into the
// regular markers table when a device has been offline for more than a day and
// changed its position.  This keeps long tracks for mobile devices while
// leaving stationary sensors in place.  The cutoff value is a Unix timestamp
// (seconds) – any device whose newest fetched_at is older is eligible.
func (db *Database) PromoteStaleRealtime(cutoff int64, dbType string) error {
	// gather candidate device IDs over a channel to avoid blocking callers
	ids := make(chan string)
	errc := make(chan error, 1)

	go func() {
		defer close(ids)
		var q string
		switch strings.ToLower(dbType) {
		case "pgx", "duckdb":
			q = `SELECT device_id FROM realtime_measurements GROUP BY device_id HAVING max(fetched_at) <= $1`
		default:
			q = `SELECT device_id FROM realtime_measurements GROUP BY device_id HAVING max(fetched_at) <= ?`
		}
		rows, err := db.DB.Query(q, cutoff)
		if err != nil {
			errc <- fmt.Errorf("stale list: %w", err)
			return
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				errc <- fmt.Errorf("scan stale: %w", err)
				rows.Close()
				return
			}
			ids <- id
		}
		errc <- rows.Err()
	}()

	// process each device sequentially; no mutex is needed
	for id := range ids {
		ms, err := db.fetchRealtimeByDevice(id, dbType)
		if err != nil {
			return err
		}
		if !moved(ms) {
			continue // stationary sensors stay in realtime table
		}

		useTx := !strings.EqualFold(dbType, "clickhouse")
		var (
			tx   *sql.Tx
			exec sqlExecutor
		)
		if useTx {
			tx, err = db.DB.Begin()
			if err != nil {
				return err
			}
			exec = tx
		} else {
			exec = db.DB
		}
		for _, m := range ms {
			// Normalise CPM before storing in the historical markers table.
			var (
				doseRate float64
				ok       bool
			)
			if realtimeConverter != nil {
				doseRate, ok = realtimeConverter(m.Value, m.Unit)
			}
			if !ok {
				continue
			}
			marker := Marker{
				DoseRate:  doseRate,
				Date:      m.MeasuredAt,
				Lon:       m.Lon,
				Lat:       m.Lat,
				CountRate: 0,
				Zoom:      0,
				Speed:     0,
				TrackID:   id,
			}
			if err := db.SaveMarkerAtomic(exec, marker, dbType); err != nil {
				if tx != nil {
					tx.Rollback()
				}
				return err
			}
		}
		if err := db.deleteRealtimeDevice(exec, id, dbType); err != nil {
			if tx != nil {
				tx.Rollback()
			}
			return err
		}
		if tx != nil {
			if err := tx.Commit(); err != nil {
				return err
			}
		}
	}
	return <-errc
}

// fetchRealtimeByDevice returns all realtime rows for a given device.
func (db *Database) fetchRealtimeByDevice(id, dbType string) ([]RealtimeMeasurement, error) {
	var q string
	switch strings.ToLower(dbType) {
	case "pgx", "duckdb":
		q = `SELECT device_id,transport,value,unit,lat,lon,measured_at,fetched_at FROM realtime_measurements WHERE device_id=$1`
	default:
		q = `SELECT device_id,transport,value,unit,lat,lon,measured_at,fetched_at FROM realtime_measurements WHERE device_id=?`
	}
	rows, err := db.DB.Query(q, id)
	if err != nil {
		return nil, fmt.Errorf("fetch realtime: %w", err)
	}
	defer rows.Close()
	var out []RealtimeMeasurement
	for rows.Next() {
		var m RealtimeMeasurement
		if err := rows.Scan(&m.DeviceID, &m.Transport, &m.Value, &m.Unit, &m.Lat, &m.Lon, &m.MeasuredAt, &m.FetchedAt); err != nil {
			return nil, fmt.Errorf("scan realtime row: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// deleteRealtimeDevice removes all realtime rows for the given device.
func (db *Database) deleteRealtimeDevice(exec sqlExecutor, id, dbType string) error {
	var q string
	switch strings.ToLower(dbType) {
	case "pgx", "duckdb":
		q = `DELETE FROM realtime_measurements WHERE device_id=$1`
	default:
		q = `DELETE FROM realtime_measurements WHERE device_id=?`
	}
	_, err := exec.Exec(q, id)
	return err
}

// moved reports whether a set of realtime measurements changed location.
// A tiny epsilon avoids floating‑point noise.
func moved(ms []RealtimeMeasurement) bool {
	if len(ms) == 0 {
		return false
	}
	const eps = 0.0001
	baseLat, baseLon := ms[0].Lat, ms[0].Lon
	for _, m := range ms[1:] {
		if math.Abs(m.Lat-baseLat) > eps || math.Abs(m.Lon-baseLon) > eps {
			return true
		}
	}
	return false
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
	default: // SQLite or Chai
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
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(altitude, 0),
                       COALESCE(detector, ''),
                       COALESCE(radiation, ''),
                       COALESCE(temperature, 0),
                       COALESCE(humidity, 0)
                FROM markers
                WHERE trackID = $1;
                `
	default:
		query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(altitude, 0),
                       COALESCE(detector, ''),
                       COALESCE(radiation, ''),
                       COALESCE(temperature, 0),
                       COALESCE(humidity, 0)
                FROM markers
                WHERE trackID = ?;
                `
	}

	rows, err := db.DB.Query(query, trackID)
	if err != nil {
		return nil, fmt.Errorf("error querying markers by trackID: %v", err)
	}
	defer rows.Close()

	var markers []Marker
	for rows.Next() {
		var marker Marker
		if err := rows.Scan(
			&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat,
			&marker.CountRate, &marker.Zoom, &marker.Speed, &marker.TrackID,
			&marker.Altitude, &marker.Detector, &marker.Radiation, &marker.Temperature, &marker.Humidity,
		); err != nil {
			return nil, fmt.Errorf("error scanning marker: %v", err)
		}
		markers = append(markers, marker)
	}

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
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(altitude, 0),
                       COALESCE(detector, ''),
                       COALESCE(radiation, ''),
                       COALESCE(temperature, 0),
                       COALESCE(humidity, 0)
                FROM markers
                WHERE trackID = $1 AND lat BETWEEN $2 AND $3 AND lon BETWEEN $4 AND $5;
                `
	default:
		query = `
                SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
                       COALESCE(altitude, 0),
                       COALESCE(detector, ''),
                       COALESCE(radiation, ''),
                       COALESCE(temperature, 0),
                       COALESCE(humidity, 0)
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
		err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat,
			&marker.CountRate, &marker.Zoom, &marker.Speed, &marker.TrackID,
			&marker.Altitude, &marker.Detector, &marker.Radiation, &marker.Temperature, &marker.Humidity)
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
        SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
               COALESCE(altitude, 0),
               COALESCE(detector, ''),
               COALESCE(radiation, ''),
               COALESCE(temperature, 0),
               COALESCE(humidity, 0)
        FROM   markers
        WHERE  trackID = $1
          AND  zoom     = $2
          AND  lat BETWEEN $3 AND $4
          AND  lon BETWEEN $5 AND $6;`
	default: // SQLite / Chai
		query = `
        SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID,
               COALESCE(altitude, 0),
               COALESCE(detector, ''),
               COALESCE(radiation, ''),
               COALESCE(temperature, 0),
               COALESCE(humidity, 0)
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
			&m.Lon, &m.Lat, &m.CountRate, &m.Zoom, &m.Speed, &m.TrackID,
			&m.Altitude, &m.Detector, &m.Radiation, &m.Temperature, &m.Humidity); err != nil {
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

	query := fmt.Sprintf(`SELECT id,doseRate,date,lon,lat,countRate,zoom,speed,trackID,
                                     COALESCE(altitude, 0) AS altitude,
                                     COALESCE(detector, '') AS detector,
                                     COALESCE(radiation, '') AS radiation,
                                     COALESCE(temperature, 0) AS temperature,
                                     COALESCE(humidity, 0) AS humidity
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
			&m.CountRate, &m.Zoom, &m.Speed, &m.TrackID,
			&m.Altitude, &m.Detector, &m.Radiation, &m.Temperature, &m.Humidity); err != nil {
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
//     anything else → use "?" (SQLite, Chai, MySQL…).
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

	query := fmt.Sprintf(`SELECT id,doseRate,date,lon,lat,countRate,zoom,speed,trackID,
                                     COALESCE(altitude, 0) AS altitude,
                                     COALESCE(detector, '') AS detector,
                                     COALESCE(radiation, '') AS radiation,
                                     COALESCE(temperature, 0) AS temperature,
                                     COALESCE(humidity, 0) AS humidity
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
			&m.CountRate, &m.Zoom, &m.Speed, &m.TrackID,
			&m.Altitude, &m.Detector, &m.Radiation, &m.Temperature, &m.Humidity); err != nil {
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
	dbType string, // "pgx" | "sqlite" | "chai" | …
) (string, error) {

	if len(markers) == 0 {
		return "", nil // nothing to compare
	}
	if threshold <= 0 {
		threshold = 10 // sane default
	}

	// We deduplicate probe points so the SQL planner works on the minimal set
	// of equality comparisons. This keeps the generated OR chain compact and
	// reduces needless aggregation work on engines that are already busy
	// serving inserts.
	type identityPoint struct {
		lat, lon float64
		date     int64
		dose     float64
	}
	seen := make(map[identityPoint]struct{})
	unique := make([]identityPoint, 0, len(markers))
	for _, m := range markers {
		p := identityPoint{lat: m.Lat, lon: m.Lon, date: m.Date, dose: m.DoseRate}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		unique = append(unique, p)
	}
	if len(unique) == 0 {
		return "", nil
	}

	// Collect chunked work over a channel so we stay faithful to the project
	// rule “share memory by communicating”. Each chunk translates into a
	// single SQL statement, dramatically lowering round-trips for the pure-Go
	// Chai driver while remaining portable for other engines.
	const maxChunk = 32
	chunkSize := maxChunk
	if len(unique) < chunkSize {
		chunkSize = len(unique)
	}
	chunks := make(chan []identityPoint)
	done := make(chan struct{})
	go func(points []identityPoint, size int, stop <-chan struct{}) {
		defer close(chunks)
		for start := 0; start < len(points); start += size {
			end := start + size
			if end > len(points) {
				end = len(points)
			}
			select {
			case <-stop:
				return
			case chunks <- points[start:end]:
			}
		}
	}(unique, chunkSize, done)
	defer close(done)

	totals := make(map[string]int) // TrackID → aggregated identical hits
	engine := strings.ToLower(dbType)

	for block := range chunks {
		if len(block) == 0 {
			continue
		}

		var sb strings.Builder
		// We write the SELECT+GROUP BY statement by hand so we can reuse it
		// for every engine without relying on vendor-specific helpers.
		sb.WriteString("SELECT trackID, COUNT(*) FROM markers WHERE ")

		args := make([]interface{}, 0, len(block)*4)
		argPos := 0
		ph := func() string {
			argPos++
			if engine == "pgx" {
				return fmt.Sprintf("$%d", argPos)
			}
			return "?"
		}

		for i, point := range block {
			if i > 0 {
				sb.WriteString(" OR ")
			}
			sb.WriteString("(lat = ")
			sb.WriteString(ph())
			sb.WriteString(" AND lon = ")
			sb.WriteString(ph())
			sb.WriteString(" AND date = ")
			sb.WriteString(ph())
			sb.WriteString(" AND doseRate = ")
			sb.WriteString(ph())
			sb.WriteString(")")

			args = append(args, point.lat, point.lon, point.date, point.dose)
		}

		sb.WriteString(" GROUP BY trackID")

		rows, err := db.DB.Query(sb.String(), args...)
		if err != nil {
			return "", fmt.Errorf("DetectExistingTrackID bulk: %w", err)
		}

		for rows.Next() {
			var (
				tid  string
				hits int
			)
			if err := rows.Scan(&tid, &hits); err != nil {
				rows.Close()
				return "", fmt.Errorf("DetectExistingTrackID scan: %w", err)
			}
			totals[tid] += hits
			if totals[tid] >= threshold {
				rows.Close()
				return tid, nil // FOUND!
			}
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return "", fmt.Errorf("DetectExistingTrackID rows: %w", err)
		}
		rows.Close()
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
