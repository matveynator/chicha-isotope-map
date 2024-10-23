package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"isotope-pathways/pkg/config"
	"isotope-pathways/pkg/database"
)

// Embedding files from the public_html folder for static serving
//
//go:embed public_html/*
var content embed.FS

// Variable to store the loaded radiation dose data
var doseData database.Data

// Flags to specify database type, path, and server port
var dbType = flag.String("db-type", "genji", "Type of the database driver: genji, sqlite, or pgx (postgresql)")
var dbPath = flag.String("db-path", "", "Path to the database file(defaults to the current folder, applicable for genji, sqlite drivers.)")
var dbHost = flag.String("db-host", "127.0.0.1", "Database host (applicable for pgx driver)")
var dbPort = flag.Int("db-port", 5432, "Database port (applicable for pgx driver)")
var dbUser = flag.String("db-user", "postgres", "Database user (applicable for pgx driver)")
var dbPass = flag.String("db-pass", "", "Database password (applicable for pgx driver)")
var dbName = flag.String("db-name", "IsotopePathways", "Database name (applicable for pgx driver)")
var pgSSLMode = flag.String("pg-ssl-mode", "prefer", "PostgreSQL SSL mode: disable, allow, prefer, require, verify-ca, or verify-full")
var port = flag.Int("port", 8765, "Port for running the server")
var version = flag.Bool("version", false, "Show the application version")

// Database instance
var db *database.Database

// =====================
// DATA PROCESSING HELPERS
// =====================

// Check if two markers are equal based on their attributes
func areMarkersEqual(m1, m2 database.Marker) bool {
	return m1.DoseRate == m2.DoseRate &&
	m1.Date == m2.Date &&
	m1.Lon == m2.Lon &&
	m1.Lat == m2.Lat &&
	m1.CountRate == m2.CountRate
}

// Filter unique markers by removing those with zero radiation dose and duplicates
func filterUniqueMarkers(markers []database.Marker) []database.Marker {
	var filteredMarkers []database.Marker

	for _, newMarker := range markers {
		if newMarker.DoseRate == 0 {
			continue // Ignore markers with zero dose
		}

		isDuplicate := false
		for _, existingMarker := range filteredMarkers {
			if areMarkersEqual(newMarker, existingMarker) {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			filteredMarkers = append(filteredMarkers, newMarker)
		}
	}

	return filteredMarkers
}

// Load data from a file, filter out empty values and duplicates
func loadDataFromFile(filename string) (database.Data, error) {
	var data database.Data

	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return data, err
	}
	defer file.Close()

	// Read the file content into bytes
	byteValue, err := ioutil.ReadAll(file)
	if err != nil {
		return data, err
	}

	// Parse the JSON data
	err = json.Unmarshal(byteValue, &data)
	if err != nil {
		return data, err
	}

	// Filter unique markers
	data.Markers = filterUniqueMarkers(data.Markers)

	return data, nil
}

// Parse a string value into a float64
func parseFloat(value string) float64 {
	parsedValue, _ := strconv.ParseFloat(value, 64)
	return parsedValue
}

// =====================
// TRANSLATION SUPPORT
// =====================

// Variable to store language translations
var translations map[string]map[string]string

// Load translations from the embedded file system
func loadTranslations(fs embed.FS, filename string) {
	file, err := fs.Open(filename) // Open the file from the embedded FS
	if err != nil {
		log.Fatalf("Error opening translation file: %v", err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file) // Read data from the file
	if err != nil {
		log.Fatalf("Error reading translation file: %v", err)
	}

	err = json.Unmarshal(data, &translations) // Parse JSON data
	if err != nil {
		log.Fatalf("Error parsing translations: %v", err)
	}
}

// Get the preferred language from the HTTP request's Accept-Language header
func getPreferredLanguage(r *http.Request) string {
	// Example of Accept-Language: "en-US,en;q=0.9,fr;q=0.8"
	langHeader := r.Header.Get("Accept-Language")

	// Supported languages list
	supportedLanguages := []string{
		"en", "zh", "es", "hi", "ar", "fr", "ru", "pt", "de", "ja", "tr", "it",
		"ko", "pl", "uk", "mn", "no", "fi", "ka", "sv", "he", "nl", "el", "hu",
		"cs", "ro", "th", "vi", "id", "ms", "bg", "lt", "et", "lv", "sl",
	}

	// Split languages by comma
	langs := strings.Split(langHeader, ",")

	// Try to find a matching language from the supported list
	for _, lang := range langs {
		lang = strings.TrimSpace(strings.SplitN(lang, ";", 2)[0]) // Remove priority (e.g., ";q=0.9")
		for _, supported := range supportedLanguages {
			if strings.HasPrefix(lang, supported) {
				return supported
			}
		}
	}

	// Default to English if no match found
	return "en"
}

// =====================
// FILE PARSING
// =====================

// Helper to extract dose rate from description text
func extractDoseRate(description string) float64 {
	re := regexp.MustCompile(`(\d+(\.\d+)?) µR/h`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		return parseFloat(match[1]) / 100 // Convert from µR/h to µSv/h
	}
	return 0
}

// Helper to extract count rate from description text
func extractCountRate(description string) float64 {
	re := regexp.MustCompile(`(\d+(\.\d+)?) cps`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		return parseFloat(match[1])
	}
	return 0
}

// Helper to estimate the time zone based on longitude
func getTimeZoneByLongitude(lon float64) *time.Location {
	switch {
		case lon >= 37 && lon <= 60: // Moscow and part of Russia
		loc, _ := time.LoadLocation("Europe/Moscow")
		return loc
		case lon >= -9 && lon <= 3: // Central Europe
		loc, _ := time.LoadLocation("Europe/Berlin")
		return loc
		case lon >= -180 && lon < -60: // North America
		loc, _ := time.LoadLocation("America/New_York")
		return loc
		default: // Default to UTC
		loc, _ := time.LoadLocation("UTC")
		return loc
	}
}

// Helper to parse a date in the format "Feb 3, 2024 19:44:03"
func parseDate(description string, loc *time.Location) int64 {
	re := regexp.MustCompile(`<b>([A-Za-z]{3} \d{1,2}, \d{4} \d{2}:\d{2}:\d{2})<\/b>`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		dateString := match[1]
		layout := "Jan 2, 2006 15:04:05"
		t, err := time.ParseInLocation(layout, dateString, loc)
		if err == nil {
			return t.Unix() // Return Unix timestamp
		}
		log.Println("Error parsing date:", err)
	}
	return 0
}

// Parse a KML file and extract markers
func parseKML(data []byte) ([]database.Marker, error) {
	var markers []database.Marker
	var longitudes []float64

	coordinatePattern := regexp.MustCompile(`<coordinates>(.*?)<\/coordinates>`)
	descriptionPattern := regexp.MustCompile(`<description><!\[CDATA\[(.*?)\]\]><\/description>`)

	coordinates := coordinatePattern.FindAllStringSubmatch(string(data), -1)
	descriptions := descriptionPattern.FindAllStringSubmatch(string(data), -1)

	// Collect longitudes to determine time zone
	for i := 0; i < len(coordinates) && i < len(descriptions); i++ {
		coords := strings.Split(strings.TrimSpace(coordinates[i][1]), ",")
		if len(coords) >= 2 {
			lon := parseFloat(coords[0])
			lat := parseFloat(coords[1])
			longitudes = append(longitudes, lon)

			doseRate := extractDoseRate(descriptions[i][1])
			countRate := extractCountRate(descriptions[i][1])

			// Create a marker (we'll add the timestamp later)
			marker := database.Marker{
				DoseRate:  doseRate,
				Lat:       lat,
				Lon:       lon,
				CountRate: countRate,
			}
			markers = append(markers, marker)
		}
	}

	// Calculate average longitude to determine the time zone
	var avgLon float64
	for _, lon := range longitudes {
		avgLon += lon
	}
	avgLon /= float64(len(longitudes))

	// Get time zone based on the average longitude
	loc := getTimeZoneByLongitude(avgLon)

	// Now parse the date for each marker
	for i := range markers {
		markers[i].Date = parseDate(descriptions[i][1], loc)
	}

	return markers, nil
}

// Process and extract data from a KML file
func processKMLFile(file multipart.File) (uniqueMarkers []database.Marker) {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Error reading KML file:", err)
		return
	}

	markers, err := parseKML(data)
	if err != nil {
		log.Println("Error parsing KML file:", err)
		return
	}

	uniqueMarkers = filterUniqueMarkers(markers)
	doseData.Markers = append(doseData.Markers, uniqueMarkers...)

	// Save the markers to the database
	for _, marker := range uniqueMarkers {
		// Pass the dbType to SaveMarker
		err = db.SaveMarker(marker, *dbType)
		if err != nil {
			log.Fatalf("Error saving marker: %v", err)
		}
	}
	return
}

// Process and extract data from a KMZ file (a compressed version of KML)
func processKMZFile(file multipart.File) (uniqueMarkers []database.Marker) {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Error reading KMZ file:", err)
		return
	}

	// Open KMZ as a ZIP archive
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		log.Println("Error opening KMZ file as ZIP:", err)
		return
	}

	// Search for the KML file inside the ZIP archive
	for _, zipFile := range zipReader.File {
		if filepath.Ext(zipFile.Name) == ".kml" {
			kmlFile, err := zipFile.Open()
			if err != nil {
				log.Println("Error opening KML file inside KMZ:", err)
				continue
			}
			defer kmlFile.Close()

			kmlData, err := io.ReadAll(kmlFile)
			if err != nil {
				log.Println("Error reading KML file inside KMZ:", err)
				return
			}

			markers, err := parseKML(kmlData)
			if err != nil {
				log.Println("Error parsing KML file from KMZ:", err)
				return
			}
			uniqueMarkers := filterUniqueMarkers(markers)
			doseData.Markers = append(doseData.Markers, uniqueMarkers...)

			// Save markers to the database
			for _, marker := range markers {
				// Pass the dbType to SaveMarker
				err = db.SaveMarker(marker, *dbType)
				if err != nil {
					log.Fatalf("Error saving marker: %v", err)
				}
			}
		}
	}
	return
}

// =====================
// FILE UPLOAD HANDLERS
// =====================

// parseTextRCTRK parses an RCTRK text file, where each line contains timestamp, coordinates, dose rate, and count rate
func parseTextRCTRK(data []byte) ([]database.Marker, error) {
	var markers []database.Marker
	lines := strings.Split(string(data), "\n")

	// Iterate over each line and parse its contents
	for i, line := range lines {
		// Skip the header and empty lines
		if i == 0 || strings.HasPrefix(line, "Timestamp") || strings.TrimSpace(line) == "" {
			continue
		}

		// Split the line into fields
		fields := strings.Fields(line)
		if len(fields) < 7 {
			log.Printf("Skipping line %d: insufficient fields. Line: %s\n", i+1, line)
			continue // If there are not enough fields, skip the line
		}

		// Parse timestamp (time field is split into date and time parts)
		timeStampStr := fields[1] + " " + fields[2]
		layout := "2006-01-02 15:04:05"
		parsedTime, err := time.Parse(layout, timeStampStr)
		if err != nil {
			log.Printf("Skipping line %d: error parsing time. Line: %s, Error: %v\n", i+1, line, err)
			continue
		}

		// Parse latitude and longitude
		lat := parseFloat(fields[3])
		lon := parseFloat(fields[4])
		if lat == 0 || lon == 0 {
			log.Printf("Skipping line %d: invalid coordinates. Latitude: %v, Longitude: %v\n", i+1, lat, lon)
			continue
		}

		// Parse dose rate and count rate
		doseRate := parseFloat(fields[6])
		countRate := parseFloat(fields[7])
		if doseRate < 0 || countRate < 0 {
			log.Printf("Skipping line %d: invalid DoseRate or CountRate. DoseRate: %v, CountRate: %v\n", i+1, doseRate, countRate)
			continue
		}

		// Create the marker object
		marker := database.Marker{
			DoseRate:  doseRate / 100, // Convert to the correct unit
			CountRate: countRate,
			Lat:       lat,
			Lon:       lon,
			Date:      parsedTime.Unix(), // Convert time to UNIX timestamp
		}
		markers = append(markers, marker)
	}

	return markers, nil
}

// Process a file in RCTRK format (either JSON or text)
func processRCTRKFile(file multipart.File) (uniqueMarkers []database.Marker) {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Error reading RCTRK file:", err)
		return
	}

	// Try parsing as JSON first
	var rctrkData database.Data
	err = json.Unmarshal(data, &rctrkData)
	if err == nil {
		uniqueMarkers = filterUniqueMarkers(rctrkData.Markers)
		doseData.Markers = append(doseData.Markers, uniqueMarkers...)
	} else {
		// If it's not JSON, try parsing as a text format
		markers, err := parseTextRCTRK(data)
		if err != nil {
			log.Println("Error parsing text RCTRK file:", err)
			return
		}
		uniqueMarkers := filterUniqueMarkers(markers)
		doseData.Markers = append(doseData.Markers, uniqueMarkers...)
	}

	// Save markers to the database
	for _, marker := range doseData.Markers {
		// Pass the dbType to SaveMarker
		err = db.SaveMarker(marker, *dbType)
		if err != nil {
			log.Fatalf("Error saving marker: %v", err)
		}
	}
	return
}

// Process AtomFast JSON file format
func processAtomFastFile(file multipart.File) (uniqueMarkers []database.Marker) {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Error reading AtomFast JSON file:", err)
		return
	}

	var atomFastData []struct {
		DV  int     `json:"dv"`  // Device ID
		D   float64 `json:"d"`   // Radiation level (µSv/h)
		R   int     `json:"r"`   // GPS accuracy
		Lat float64 `json:"lat"` // Latitude
		Lng float64 `json:"lng"` // Longitude
		T   int64   `json:"t"`   // Timestamp in Unix format (ms)
	}

	err = json.Unmarshal(data, &atomFastData)
	if err != nil {
		log.Println("Error parsing AtomFast file:", err)
		return
	}

	var markers []database.Marker
	for _, record := range atomFastData {
		marker := database.Marker{
			DoseRate:  record.D,
			Date:      record.T / 1000, // Convert milliseconds to seconds
			Lon:       record.Lng,
			Lat:       record.Lat,
			CountRate: record.D * 100, // AtomFast devices don't provide CPS, assume dose is CPS
		}
		markers = append(markers, marker)
	}

	uniqueMarkers = filterUniqueMarkers(markers)
	doseData.Markers = append(doseData.Markers, uniqueMarkers...)

	// Save markers to the database
	for _, marker := range uniqueMarkers {
		// Pass the dbType to SaveMarker
		err = db.SaveMarker(marker, *dbType)
		if err != nil {
			log.Fatalf("Error saving marker: %v", err)
		}
	}
	return
}

// Upload handler to process multiple file uploads
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(100 << 20) // Limit file upload to 100MB
	if err != nil {
		http.Error(w, "Error uploading file", http.StatusInternalServerError)
		return
	}

	// Get list of files
	files := r.MultipartForm.File["files[]"] // Expecting "files[]" parameter from the client

	if len(files) == 0 {
		http.Error(w, "No files selected", http.StatusBadRequest)
		return
	}

	// Variables to store coordinates for bounds
	var minLat, minLon, maxLat, maxLon float64
	minLat, minLon = 90.0, 180.0
	maxLat, maxLon = -90.0, -180.0

	// Iterate over files
	for _, fileHeader := range files {
		// Open the file
		file, err := fileHeader.Open()
		if err != nil {
			http.Error(w, "Error opening file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		var markers []database.Marker

		// Determine file type by extension
		ext := filepath.Ext(fileHeader.Filename)
		switch ext {
		case ".kml":
			markers = processKMLFile(file)
		case ".kmz":
			markers = processKMZFile(file)
		case ".rctrk":
			markers = processRCTRKFile(file)
		case ".json":
			markers = processAtomFastFile(file)
		default:
			http.Error(w, "Unsupported file type", http.StatusBadRequest)
			return
		}

		// Calculate bounds for the current file
		for _, marker := range markers {
			if marker.Lat < minLat {
				minLat = marker.Lat
			}
			if marker.Lat > maxLat {
				maxLat = marker.Lat
			}
			if marker.Lon < minLon {
				minLon = marker.Lon
			}
			if marker.Lon > maxLon {
				maxLon = marker.Lon
			}
		}
	}

	// Check if bounds have changed, otherwise return an error
	if minLat == 90.0 || minLon == 180.0 || maxLat == -90.0 || maxLon == -180.0 {
		log.Println("Error: Unable to calculate bounds, no valid markers found.")
		http.Error(w, "No valid data in file", http.StatusBadRequest)
		return
	}

	// Log bounds for debugging
	log.Printf("Bounds calculated: minLat=%f, minLon=%f, maxLat=%f, maxLon=%f", minLat, minLon, maxLat, maxLon)

	// Return the bounds as part of the response
	response := map[string]interface{}{
		"status":  "success",
		"minLat":  minLat,
		"minLon":  minLon,
		"maxLat":  maxLat,
		"maxLon":  maxLon,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// =====================
// WEB SERVER HANDLERS
// =====================

// Function to handle rendering the map with markers
func mapHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)

	// Add function toJSON for use in the template
	tmpl := template.Must(template.New("map.html").Funcs(template.FuncMap{
		"translate": func(key string) string {
			if val, ok := translations[lang][key]; ok {
				return val
			}
			return translations["en"][key] // Fallback to English if not found
		},
		"toJSON": func(data interface{}) (string, error) {
			bytes, err := json.Marshal(data)
			return string(bytes), err
		},
	}).ParseFS(content, "public_html/map.html"))

	if config.CompileVersion == "dev" {
		config.CompileVersion = "latest"
	}

	// Updated struct to include the Lang field
	data := struct {
		Markers      []database.Marker
		Version      string
		Translations map[string]map[string]string // Pass the whole translations map
		Lang         string
	}{
		Markers:      doseData.Markers,
		Version:      config.CompileVersion,
		Translations: translations, // Pass the entire translation map, not just one language
		Lang:         lang,
	}

	// Execute the template and handle errors
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// =====================
// MAIN ENTRY POINT
// =====================

// Main function to initialize the server
func main() {
	flag.Parse()

	// Load translations from the embedded file system
	loadTranslations(content, "public_html/translations.json")

	// Handle version flag
	if *version {
		fmt.Printf("isotope-pathways version %s\n", config.CompileVersion)
		return
	}

	// Initialize the database
	dbConfig := database.Config{
		DBType:    *dbType,
		DBPath:    *dbPath,
		DBHost:    *dbHost,
		DBPort:    *dbPort,
		DBUser:    *dbUser,
		DBPass:    *dbPass,
		DBName:    *dbName,
		PGSSLMode: *pgSSLMode,
		Port:      *port,
	}

	var err error
	db, err = database.NewDatabase(dbConfig)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize the database schema
	if err := db.InitSchema(); err != nil {
		log.Fatalf("Error initializing database schema: %v", err)
	}

	// Load data from the database
	markers, err := db.LoadMarkers()
	if err != nil {
		log.Printf("Error loading markers from database: %v", err)
	} else {
		doseData.Markers = append(doseData.Markers, filterUniqueMarkers(markers)...)
	}

	// Set up the web server

	// Serve static files from /static path
	staticFiles, err := fs.Sub(content, "public_html")
	if err != nil {
		log.Fatalf("Failed to extract public_html subdirectory: %v", err)
	}

	// Serve static content through /static
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))

	http.HandleFunc("/", mapHandler)
	http.HandleFunc("/upload", uploadHandler)
	log.Printf("Application running at: http://localhost:%d", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
