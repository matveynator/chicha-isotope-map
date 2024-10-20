package database

import (
	"database/sql"
	"fmt"
	"log"
)

// Database represents the interface for interacting with the database.
type Database struct {
	DB *sql.DB // The underlying SQL database connection
}

// Config holds the configuration details for initializing the database.
type Config struct {
	DBType string // The type of the database (e.g., "sqlite" or "genji")
	DBPath string // The file path to the database file
	Port   int    // The port number (used in database file naming if needed)
}

// NewDatabase creates and initializes a new database connection.
func NewDatabase(config Config) (*Database, error) {
	var dsn string // Data Source Name, the location of the database file

	// If the database type is "sqlite", set the DSN accordingly
	if config.DBType == "sqlite" {
		dsn = config.DBPath
		if dsn == "" {
			// If no database path is provided, generate a default filename based on the port
			dsn = fmt.Sprintf("database-%d.sqlite", config.Port)
		}
	} else { // For other database types (e.g., Genji), set the DSN
		dsn = config.DBPath
		if dsn == "" {
			// If no database path is provided, generate a default filename based on the port
			dsn = fmt.Sprintf("database-%d.genji", config.Port)
		}
	}

	// Open a connection to the database
	db, err := sql.Open(config.DBType, dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening the database: %v", err)
	}

	// Check the connection to ensure it's working
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error connecting to the database: %v", err)
	}

	// Log the database type and location being used
	log.Printf("Using database: %s at %s", config.DBType, dsn)

	// Return the initialized Database struct
	return &Database{DB: db}, nil
}

// InitSchema initializes the database schema for storing data (markers).
func (db *Database) InitSchema() error {
	// SQL schema to create the 'markers' table if it doesn't exist
	schema := `
  CREATE TABLE IF NOT EXISTS markers (
    id INTEGER PRIMARY KEY,
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

// SaveMarker saves a new marker to the database, assigning it a new ID if it doesn't already exist.
func (db *Database) SaveMarker(marker Marker) error {
	var count int // Variable to count the number of matching records

	// Check if a marker with the same attributes already exists in the database
	err := db.DB.QueryRow(`
    SELECT COUNT(1) 
    FROM markers 
    WHERE doseRate = ? AND date = ? AND lon = ? AND lat = ? AND countRate = ?`,
		marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate).Scan(&count)
	if err != nil {
		return err
	}

	// If a matching marker is found, skip saving it and log the event
	if count > 0 {
		log.Printf("Marker (%f, %d, %f, %f, %f) already exists in the database.\n", marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate)
		return nil
	}

	// Get the next available ID for the new marker
	nextID, err := db.getNextID()
	if err != nil {
		return fmt.Errorf("error getting the next ID: %v", err)
	}

	// Insert the new marker into the 'markers' table
	_, err = db.DB.Exec(`
        INSERT INTO markers (id, doseRate, date, lon, lat, countRate)
        VALUES (?, ?, ?, ?, ?, ?)`,
		nextID, marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate)

	return err // Return any errors encountered during the insert operation
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

