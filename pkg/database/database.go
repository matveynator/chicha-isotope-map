package database

import (
	"database/sql"
	"fmt"
	"log"
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
        CREATE INDEX IF NOT EXISTS idx_markers_unique ON markers (doseRate, date, lon, lat, countRate, zoom, speed, trackID);
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
        CREATE INDEX IF NOT EXISTS idx_markers_unique ON markers (doseRate, date, lon, lat, countRate, zoom, speed, trackID);
        CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon);
				CREATE INDEX IF NOT EXISTS idx_markers_trackid_bounds ON markers (trackID, lat, lon);
        CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID);
        `

	case "genji": // Genji
		schema = `
        CREATE TABLE IF NOT EXISTS markers (
            id INTEGER PRIMARY KEY,
            doseRate REAL,
            date INTEGER,
            lon REAL,
            lat REAL,
            countRate REAL,
            zoom INTEGER,
            speed REAL,
            trackID TEXT
        );
        CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID);
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

// SaveMarkerAtomic saves a marker atomically using the idGenerator channel for unique IDs.
func (db *Database) SaveMarkerAtomic(marker Marker, dbType string) error {
	var count int
	var query string

	// Determine SQL query based on database type
	switch dbType {
	case "pgx":
		query = `
		SELECT COUNT(1)
		FROM markers
		WHERE doseRate = $1 AND date = $2 AND lon = $3 AND lat = $4 AND countRate = $5 AND zoom = $6 AND speed = $7 AND trackID = $8`
	default:
		query = `
		SELECT COUNT(1)
		FROM markers
		WHERE doseRate = ? AND date = ? AND lon = ? AND lat = ? AND countRate = ? AND zoom = ? AND speed = ? AND trackID = ?`
	}

	// Execute query to check if marker exists
	err := db.DB.QueryRow(query,
		marker.DoseRate,
		marker.Date,
		marker.Lon,
		marker.Lat,
		marker.CountRate,
		marker.Zoom,
		marker.Speed,
		marker.TrackID).Scan(&count)

	if err != nil {
		return err
	}

	// If marker exists, return nil
	if count > 0 {
		log.Printf("Marker (%f, %d, %f, %f, %f, %d, %f, %s) already exists.\n", marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate, marker.Zoom, marker.Speed, marker.TrackID)
		return nil
	}

	// Get next ID from generator if not using SERIAL (PostgreSQL)
	if dbType != "pgx" {
		nextID := <-db.idGenerator
		marker.ID = nextID
	}

	// Insert new marker with generated ID
	switch dbType {
	case "pgx":
		query = `
		INSERT INTO markers (doseRate, date, lon, lat, countRate, zoom, speed, trackID)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	default:
		query = `
		INSERT INTO markers (id, doseRate, date, lon, lat, countRate, zoom, speed, trackID)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}

	if dbType == "pgx" {
		_, err = db.DB.Exec(query,
			marker.DoseRate,
			marker.Date,
			marker.Lon,
			marker.Lat,
			marker.CountRate,
			marker.Zoom,
			marker.Speed,
			marker.TrackID)
	} else {
		_, err = db.DB.Exec(query,
			marker.ID,
			marker.DoseRate,
			marker.Date,
			marker.Lon,
			marker.Lat,
			marker.CountRate,
			marker.Zoom,
			marker.Speed,
			marker.TrackID)
	}

	if err != nil {
		return fmt.Errorf("error inserting marker: %v", err)
	}

	return nil
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
    zoom    int,
    minLat, minLon, maxLat, maxLon float64,
    dbType  string,
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
    default:    // SQLite / Genji
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

