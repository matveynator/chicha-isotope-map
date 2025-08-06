package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
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

// NewDatabase creates and initializes a new database connection.
func NewDatabase(config Config) (*Database, error) {
	var dsn string

	switch config.DBType {
	case "sqlite", "genji":
		dsn = config.DBPath
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.%s", config.Port, config.DBType)
		}
	case "pgx":
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			config.DBUser, config.DBPass, config.DBHost, config.DBPort, config.DBName, config.PGSSLMode)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.DBType)
	}

	db, err := sql.Open(config.DBType, dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening the database: %v", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error connecting to the database: %v", err)
	}

	// Log the database type and location being used
	log.Printf("Using database driver: %s with DSN: %s", config.DBType, dsn)

	// Get the maximum current ID from the database
	var maxID sql.NullInt64 // Variable to hold the max ID (nullable)

	// Query to find the maximum ID in the 'markers' table
	_ = db.QueryRow(`SELECT MAX(id) FROM markers`).Scan(&maxID)

	// If a valid max ID is found, set initialID to maxID + 1
	initialID := int64(1)
	if maxID.Valid {
		initialID = maxID.Int64 + 1
	}

	// Start the ID generator goroutine
	idChannel := startIDGenerator(initialID)

	// Return the Database struct with the idGenerator channel
	return &Database{
		DB:          db,
		idGenerator: idChannel,
	}, nil
}

// InitSchema initializes the database schema for storing data (markers).
func (db *Database) InitSchema(config Config) error {
	var schema string

	switch config.DBType {
	case "pgx": // PostgreSQL
		schema = `
        CREATE TABLE IF NOT EXISTS markers (
            id SERIAL PRIMARY KEY,
            doseRate REAL,
            date BIGINT,
            lon REAL,
            lat REAL,
            countRate REAL,
            zoom INTEGER,
            speed REAL,
            trackID VARCHAR(30)
        );
        CREATE UNIQUE INDEX IF NOT EXISTS idx_markers_unique ON markers(doseRate, date, lon, lat, countRate, zoom, speed, trackID);

        CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon);
				CREATE INDEX IF NOT EXISTS idx_markers_trackid_bounds ON markers (trackID, lat, lon);
        CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID);
        `

	case "sqlite": // SQLite
		schema = `
        CREATE TABLE IF NOT EXISTS markers (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            doseRate REAL,
            date INTEGER,
            lon REAL,
            lat REAL,
            countRate REAL,
            zoom INTEGER,
            speed REAL,
            trackID TEXT
        );
        CREATE UNIQUE INDEX IF NOT EXISTS idx_markers_unique ON markers(doseRate, date, lon, lat, countRate, zoom, speed, trackID);
        CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon);
				CREATE INDEX IF NOT EXISTS idx_markers_trackid_bounds ON markers (trackID, lat, lon);
        CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID);
        `

       case "genji": // Genji embedded DB
	schema = `
CREATE TABLE IF NOT EXISTS markers (
    id        INTEGER PRIMARY KEY,              -- обязателен и НЕ генерится сам
    doseRate  REAL,
    date      INTEGER,
    lon       REAL,
    lat       REAL,
    countRate REAL,
    zoom      INTEGER,
    speed     REAL,
    trackID   TEXT
);
/* уникальный ключ по всем измерительным полям —
   дубликаты будем гасить через ON CONFLICT DO NOTHING */
CREATE UNIQUE INDEX IF NOT EXISTS idx_markers_unique
       ON markers(doseRate,date,lon,lat,countRate,zoom,speed,trackID);

CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers(trackID);
`
 

	default:
		return fmt.Errorf("unsupported database type: %s", config.DBType)
	}

	// Execute the schema creation
	_, err := db.DB.Exec(schema)
	if err != nil {
		return fmt.Errorf("error initializing database schema: %v", err)
	}

	return nil
}

// SaveMarkerAtomic inserts a marker inside an open tx (or db) and ignores duplicates.
// Works for: pgx | genji | sqlite. No pre-SELECT, no mutexes — pure SQL.
func (db *Database) SaveMarkerAtomic(
	exec sqlExecutor,        // *sql.Tx inside batch OR *sql.DB for one-shot
	m Marker, dbType string, // dbType: "pgx" | "genji" | "sqlite"
) error {

	// Generate local id for non-Postgres drivers
	if dbType != "pgx" {
		m.ID = <-db.idGenerator
	}

	switch dbType {

	case "pgx": // PostgreSQL
		_, err := exec.Exec(`
INSERT INTO markers
      (doseRate,date,lon,lat,countRate,zoom,speed,trackID)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT ON CONSTRAINT idx_markers_unique DO NOTHING`,
			m.DoseRate, m.Date, m.Lon, m.Lat,
			m.CountRate, m.Zoom, m.Speed, m.TrackID)
		return err

	default: // genji & sqlite — «?» placeholders
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

// sqlExecutor is satisfied by both *sql.Tx and *sql.DB
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

// GetMarkersByZoomBoundsSpeed — z/bounds/date/speed фильтр
func (db *Database) GetMarkersByZoomBoundsSpeed(
	zoom int,
	minLat, minLon, maxLat, maxLon float64,
	dateFrom, dateTo int64,               // ⏱️ NOVO
	speedRanges []SpeedRange,
	dbType string,
) ([]Marker, error) {

	ph := func(n int) string {
		if dbType == "pgx" { return fmt.Sprintf("$%d", n) }
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

	// ---- фильтр по времени --------------------------------------
	if dateFrom > 0 {
		sb.WriteString(" AND date >= " + ph(len(args)+1))
		args = append(args, dateFrom)
	}
	if dateTo > 0 {
		sb.WriteString(" AND date <= " + ph(len(args)+1))
		args = append(args, dateTo)
	}

	// ---- фильтр скоростей ---------------------------------------
	if len(speedRanges) > 0 {
		sb.WriteString(" AND (")
		for i, r := range speedRanges {
			if i > 0 { sb.WriteString(" OR ") }
			sb.WriteString("speed BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
			args = append(args, r.Min, r.Max)
		}
		sb.WriteString(")")
	}

	query := fmt.Sprintf(`SELECT id,doseRate,date,lon,lat,countRate,zoom,speed,trackID
	                      FROM markers WHERE %s;`, sb.String())

	rows, err := db.DB.Query(query, args...)
	if err != nil { return nil, fmt.Errorf("query: %w", err) }
	defer rows.Close()

	var out []Marker
	for rows.Next() {
		var m Marker
		if err := rows.Scan(&m.ID,&m.DoseRate,&m.Date,&m.Lon,&m.Lat,
		                    &m.CountRate,&m.Zoom,&m.Speed,&m.TrackID); err != nil {
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
// ------------------------------------------------------------------
func (db *Database) GetMarkersByTrackIDZoomBoundsSpeed(
	trackID string,
	zoom int,
	minLat, minLon, maxLat, maxLon float64,
	dateFrom, dateTo int64,           // ⏱️ NOVO
	speedRanges []SpeedRange,
	dbType string,
) ([]Marker, error) {

	ph := func(n int) string {
		if dbType == "pgx" { return fmt.Sprintf("$%d", n) }
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
		sb.WriteString(" AND (")
		for i, r := range speedRanges {
			if i > 0 { sb.WriteString(" OR ") }
			sb.WriteString("speed BETWEEN " + ph(len(args)+1) + " AND " + ph(len(args)+2))
			args = append(args, r.Min, r.Max)
		}
		sb.WriteString(")")
	}

	query := fmt.Sprintf(`SELECT id,doseRate,date,lon,lat,countRate,zoom,speed,trackID
	                      FROM markers WHERE %s;`, sb.String())

	rows, err := db.DB.Query(query, args...)
	if err != nil { return nil, fmt.Errorf("query: %w", err) }
	defer rows.Close()

	var out []Marker
	for rows.Next() {
		var m Marker
		if err := rows.Scan(&m.ID,&m.DoseRate,&m.Date,&m.Lon,&m.Lat,
		                    &m.CountRate,&m.Zoom,&m.Speed,&m.TrackID); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

