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

// Id generator routine with channel
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
	DBType    string // The type of the database driver (e.g., "sqlite", "genji", or "pgx" (postgres))
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
func (db *Database) InitSchema() error {
	// SQL schema to create the 'markers' table if it doesn't exist
	schema := `
	CREATE TABLE IF NOT EXISTS markers (
		id INTEGER PRIMARY KEY, -- Custom ID generated manually
		doseRate REAL,  -- Radiation dose rate (in µSv/h)
		date INTEGER,   -- Timestamp (UNIX time format)
		lon REAL,       -- Longitude of the marker
		lat REAL,       -- Latitude of the marker
		countRate REAL, -- Count rate (CPS)
		zoom INTEGER,   -- Zoom level (1-20)
		speed REAL      -- Current speed (m/s)  
	);
	`
	// Execute the schema creation statement
	_, err := db.DB.Exec(schema)
	return err // Return the error if any
}

// SaveMarkerAtomic saves a marker atomically using the idGenerator channel for unique IDs.
func (db *Database) SaveMarkerAtomic(marker Marker, dbType string) error {
	var count int
	var query string

	// Определение SQL-запроса в зависимости от типа базы данных
	switch dbType {
	case "pgx":
		query = `
		SELECT COUNT(1)
		FROM markers
		WHERE doseRate = $1 AND date = $2 AND lon = $3 AND lat = $4 AND countRate = $5 AND zoom = $6 AND speed = $7`
	default:
		query = `
		SELECT COUNT(1)
		FROM markers
		WHERE doseRate = ? AND date = ? AND lon = ? AND lat = ? AND countRate = ? AND zoom = ? AND speed = ?`
	}

	// Выполнение запроса для проверки существования маркера
	err := db.DB.QueryRow(query,
		marker.DoseRate,
		marker.Date,
		marker.Lon,
		marker.Lat,
		marker.CountRate,
		marker.Zoom,
		marker.Speed).Scan(&count)

	if err != nil {
		return err
	}

	// Если маркер уже существует, возвращаем nil
	if count > 0 {
		log.Printf("Marker (%f, %d, %f, %f, %f, %d, %f) already exists.\n", marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate, marker.Zoom, marker.Speed)
		return nil
	}

	// Получаем следующий ID из генератора
	nextID := <-db.idGenerator

	// Вставляем новый маркер с вручную сгенерированным ID
	switch dbType {
	case "pgx":
		query = `
		INSERT INTO markers (id, doseRate, date, lon, lat, countRate, zoom, speed)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	default:
		query = `
		INSERT INTO markers (id, doseRate, date, lon, lat, countRate, zoom, speed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	}

	_, err = db.DB.Exec(query, nextID, marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate, marker.Zoom, marker.Speed)

	return err
}

// LoadMarkers loads all markers from the 'markers' table in the database.
func (db *Database) LoadMarkers() ([]Marker, error) {
	// Query to select all marker attributes from the 'markers' table
	rows, err := db.DB.Query(`
	SELECT id, doseRate, date, lon, lat, countRate, zoom, speed FROM markers
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // Ensure rows are closed after the function returns

	var markers []Marker // Slice to hold the loaded markers

	// Iterate over the result set and scan each row into a Marker struct
	for rows.Next() {
		var marker Marker
		if err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate, &marker.Zoom, &marker.Speed); err != nil {
			return nil, err
		}
		markers = append(markers, marker) // Add each marker to the slice
	}
	return markers, nil // Return the slice of markers
}

// GetMarkersByZoomAndBounds retrieves markers filtered by zoom level and geographical bounds.
func (db *Database) GetMarkersByZoomAndBounds(zoom int, minLat, minLon, maxLat, maxLon float64, dbType string) ([]Marker, error) {
	var query string

	// Use appropriate placeholders depending on the database type
	switch dbType {
	case "pgx": // PostgreSQL
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed
		FROM markers
		WHERE zoom = $1 AND lat BETWEEN $2 AND $3 AND lon BETWEEN $4 AND $5;
		`
	default: // SQLite or Genji
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed
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
		err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate, &marker.Zoom, &marker.Speed)
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

