package database

import (
	"database/sql"
	"fmt"
	"log"
)

// Database represents the interface for interacting with the database.
type Database struct {
	DB           *sql.DB    // The underlying SQL database connection
	idSyncChan   chan bool  // Channel for synchronizing ID generation
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

	// Initialize the idSyncChan with a buffer of 1
	return &Database{
		DB:         db,
		idSyncChan: make(chan bool, 1),
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
		countRate REAL  -- Count rate (CPS)
	);
	`
	// Execute the schema creation statement
	_, err := db.DB.Exec(schema)
	return err // Return the error if any
}

// getNextID finds the maximum current id in the 'markers' table and returns the next available ID.
func (db *Database) getNextID() (int64, error) {
	var maxID sql.NullInt64 // Variable to hold the max ID (nullable)

	// Query to find the maximum ID in the 'markers' table
	err := db.DB.QueryRow(`SELECT MAX(id) FROM markers`).Scan(&maxID)
	if err != nil {
		return 0, fmt.Errorf("error retrieving the maximum ID: %v", err)
	}

	// If a valid max ID is found, return the next ID (maxID + 1)
	if maxID.Valid {
		return maxID.Int64 + 1, nil
	}
	return 1, nil // If no records exist, return 1 as the starting ID
}

// SaveMarkerAtomic uses a channel to ensure atomic access to ID generation and insertion.
func (db *Database) SaveMarkerAtomic(marker Marker, dbType string) error {
	var count int
	var query string

	// Заблокировать доступ через канал
	db.idSyncChan <- true
	defer func() {
		<-db.idSyncChan // Освобождение канала после выполнения
	}()

	// Определение SQL-запроса в зависимости от типа базы данных
	switch dbType {
	case "pgx":
		query = `
		SELECT COUNT(1)
		FROM markers
		WHERE doseRate = $1 AND date = $2 AND lon = $3 AND lat = $4 AND countRate = $5`
	default:
		query = `
		SELECT COUNT(1)
		FROM markers
		WHERE doseRate = ? AND date = ? AND lon = ? AND lat = ? AND countRate = ?`
	}

	// Выполнение запроса для проверки существования маркера
	err := db.DB.QueryRow(query,
	marker.DoseRate,
	marker.Date,
	marker.Lon,
	marker.Lat,
	marker.CountRate).Scan(&count)

	if err != nil {
		return err
	}

	// Если маркер уже существует, возвращаем nil
	if count > 0 {
		log.Printf("Marker (%f, %d, %f, %f, %f) already exists.\n", marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate)
		return nil
	}

	// Получаем следующий ID
	nextID, err := db.getNextID()
	if err != nil {
		return fmt.Errorf("error getting the next ID: %v", err)
	}

	// Вставляем новый маркер с вручную сгенерированным ID
	switch dbType {
	case "pgx":
		query = `
		INSERT INTO markers (id, doseRate, date, lon, lat, countRate)
		VALUES ($1, $2, $3, $4, $5, $6)`
	default:
		query = `
		INSERT INTO markers (id, doseRate, date, lon, lat, countRate)
		VALUES (?, ?, ?, ?, ?, ?)`
	}

	_, err = db.DB.Exec(query,
	nextID, marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate)

	return err
}

// LoadMarkers loads all markers from the 'markers' table in the database.
func (db *Database) LoadMarkers() ([]Marker, error) {
	// Query to select all marker attributes from the 'markers' table
	rows, err := db.DB.Query(`
	SELECT id, doseRate, date, lon, lat, countRate FROM markers
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // Ensure rows are closed after the function returns

	var markers []Marker // Slice to hold the loaded markers

	// Iterate over the result set and scan each row into a Marker struct
	for rows.Next() {
		var marker Marker
		if err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate); err != nil {
			return nil, err
		}
		markers = append(markers, marker) // Add each marker to the slice
	}
	return markers, nil // Return the slice of markers
}
