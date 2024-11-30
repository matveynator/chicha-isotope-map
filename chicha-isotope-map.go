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
	"math"
	"math/rand"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// Embedding files from the public_html folder for static serving
//
//go:embed public_html/*
var content embed.FS

// Variable to store the loaded radiation dose data
var doseData database.Data

// Глобальная переменная для версии компиляции
var CompileVersion = "dev"

// Flags для конфигурации приложения
var (
	version   = flag.Bool("version", false, "Print the version and exit")
	dbType    = flag.String("dbtype", "sqlite", "Database type (sqlite, genji, pgx)")
	dbPath    = flag.String("dbpath", "database.sqlite", "Path to the database file")
	dbHost    = flag.String("dbhost", "localhost", "Database host (PostgreSQL)")
	dbPort    = flag.Int("dbport", 5432, "Database port (PostgreSQL)")
	dbUser    = flag.String("dbuser", "user", "Database user (PostgreSQL)")
	dbPass    = flag.String("dbpass", "password", "Database password (PostgreSQL)")
	dbName    = flag.String("dbname", "isotope_map", "Database name (PostgreSQL)")
	pgSSLMode = flag.String("pgsslmode", "disable", "PostgreSQL SSL mode")
	port      = flag.Int("port", 8765, "Port to run the web server on")
)

// Database instance
var db *database.Database

// =====================
// DATA PROCESSING HELPERS
// =====================

// GenerateSerialNumber генерирует серийный номер длиной не более 6 символов
func GenerateSerialNumber() string {
	const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const maxLength = 6

	// Получаем текущее время в миллисекундах (чтобы сократить размер числа)
	timestamp := uint64(time.Now().UnixNano() / 1e6)

	// Кодируем временную метку в Base62
	encoded := ""
	base := uint64(len(base62Chars))
	for timestamp > 0 && len(encoded) < maxLength {
		remainder := timestamp % base
		encoded = string(base62Chars[remainder]) + encoded
		timestamp = timestamp / base
	}

	// Если длина меньше 6, добавляем случайные символы
	rand.Seed(time.Now().UnixNano())
	for len(encoded) < maxLength {
		encoded += string(base62Chars[rand.Intn(len(base62Chars))])
	}

	return encoded
}

// Function to convert Rh to Sv
func convertRhToSv(markers []database.Marker) []database.Marker {
	conversionFactor := 0.01 // Example conversion factor
	filteredMarkers := []database.Marker{}

	for _, newMarker := range markers {
		// Convert DoseRate to Sv
		newMarker.DoseRate = newMarker.DoseRate * conversionFactor
		filteredMarkers = append(filteredMarkers, newMarker)
	}

	return filteredMarkers
}

// Filter ZERO markers
func filterZeroMarkers(markers []database.Marker) []database.Marker {
	filteredMarkers := []database.Marker{}

	for _, newMarker := range markers {
		if newMarker.DoseRate == 0 {
			continue // Ignore markers with zero dose
		} else {
			filteredMarkers = append(filteredMarkers, newMarker)
		}
	}

	return filteredMarkers
}

// haversineDistance calculates the distance between two geographic points.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters
	phi1 := lat1 * math.Pi / 180.0
	phi2 := lat2 * math.Pi / 180.0
	deltaPhi := (lat2 - lat1) * math.Pi / 180.0
	deltaLambda := (lon2 - lon1) * math.Pi / 180.0

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// calculateSpeedForMarkers calculates the average speed using a sliding window.
func calculateSpeedForMarkers(markers []database.Marker, windowSize int) []database.Marker {
	// Check if there are enough markers
	if len(markers) < 2 {
		for i := range markers {
			markers[i].Speed = 0
		}
		return markers
	}

	// Sort markers by date
	sort.Slice(markers, func(i, j int) bool {
		return markers[i].Date < markers[j].Date
	})

	// Initialize slices for distances and time differences
	var distances []float64
	var timeDiffs []float64

	// Calculate distances and time differences between consecutive markers
	for i := 1; i < len(markers); i++ {
		prevMarker := markers[i-1]
		currMarker := markers[i]

		distance := haversineDistance(prevMarker.Lat, prevMarker.Lon, currMarker.Lat, currMarker.Lon)
		timeDiff := float64(currMarker.Date - prevMarker.Date)

		if timeDiff > 0 {
			distances = append(distances, distance)
			timeDiffs = append(timeDiffs, timeDiff)
		} else {
			distances = append(distances, 0)
			timeDiffs = append(timeDiffs, 1) // Avoid division by zero
		}
	}

	// Calculate average speed for each marker using sliding window
	for i := 0; i < len(markers); i++ {
		start := max(0, i-windowSize+1)
		end := i

		var totalDistance float64
		var totalTime float64

		for j := start; j <= end && j < len(distances); j++ {
			totalDistance += distances[j]
			totalTime += timeDiffs[j]
		}

		var avgSpeed float64
		if totalTime > 0 {
			avgSpeed = totalDistance / totalTime
		} else {
			avgSpeed = 0
		}

		markers[i].Speed = avgSpeed
	}

	// Set speed of the first marker to 0
	if len(markers) > 0 {
		markers[0].Speed = 0
	}

	return markers
}

// Helper function to find the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// calculateZoomMarkers processes markers to create data for each zoom level using spatial clustering.
func calculateZoomMarkers(markers []database.Marker) []database.Marker {
	resultMarkers := []database.Marker{}

	maxClusterZoomLevel := 17
	numZoomLevels := 20

	// Sort markers once before starting goroutines
	sortedMarkers := make([]database.Marker, len(markers))
	copy(sortedMarkers, markers)
	sort.Slice(sortedMarkers, func(i, j int) bool {
		if sortedMarkers[i].Lat == sortedMarkers[j].Lat {
			return sortedMarkers[i].Lon < sortedMarkers[j].Lon
		}
		return sortedMarkers[i].Lat < sortedMarkers[j].Lat
	})

	// Channel to collect results from goroutines
	resultsChan := make(chan []database.Marker, numZoomLevels)

	// Start goroutines for each zoom level
	for zoomLevel := 1; zoomLevel <= numZoomLevels; zoomLevel++ {
		zoomLevel := zoomLevel // Capture zoomLevel for the goroutine
		go func() {
			var zoomMarkers []database.Marker

			if zoomLevel <= maxClusterZoomLevel {
				// For zoom levels ≤ maxClusterZoomLevel, perform clustering
				distanceThreshold := getDistanceThresholdForZoom(zoomLevel)

				// Use the pre-sorted list of markers
				clusters := clusterMarkers(sortedMarkers, distanceThreshold)

				// Process each cluster to calculate average values
				for _, cluster := range clusters {
					avgDoseRate, avgCountRate, avgSpeed, avgLat, avgLon, date := calculateAverages(cluster)

					newMarker := database.Marker{
						DoseRate:  avgDoseRate,
						Date:      date,
						Lon:       avgLon,
						Lat:       avgLat,
						CountRate: avgCountRate,
						Zoom:      zoomLevel,
						Speed:     avgSpeed,
					}
					zoomMarkers = append(zoomMarkers, newMarker)
				}
			} else {
				// For zoom levels > maxClusterZoomLevel, use all markers without clustering
				zoomMarkers = make([]database.Marker, len(markers))
				for i, m := range markers {
					newMarker := m
					newMarker.Zoom = zoomLevel
					zoomMarkers[i] = newMarker
				}

				// Calculate speed for these markers
				windowSize := 10
				zoomMarkers = calculateSpeedForMarkers(zoomMarkers, windowSize)
			}

			// Send results to the channel
			resultsChan <- zoomMarkers
		}()
	}

	// Collect results from all goroutines
	for i := 0; i < numZoomLevels; i++ {
		zoomMarkers := <-resultsChan
		resultMarkers = append(resultMarkers, zoomMarkers...)
	}

	// Close the channel
	close(resultsChan)

	return resultMarkers
}

// getDistanceThresholdForZoom returns the distance threshold in meters for clustering at a given zoom level
func getDistanceThresholdForZoom(zoomLevel int) float64 {
	switch zoomLevel {
	case 1:
		return 10000000 // 10,000 km
	case 2:
		return 5000000 // 5,000 km
	case 3:
		return 2500000 // 2,500 km
	case 4:
		return 1000000 // 1,000 km
	case 5:
		return 500000 // 500 km
	case 6:
		return 100000 // 100 km
	case 7:
		return 10000 // 10 km
	case 8:
		return 7500 // 7.5 km
	case 9:
		return 5000 // 5 km
	case 10:
		return 3000 // 3 km
	case 11:
		return 1000 // 1 km
	case 12:
		return 700 // 700 m
	case 13:
		return 400 // 400 m
	case 14:
		return 100 // 100 m
	case 15:
		return 50 // 50 m
	case 16:
		return 25 // 25 m
	case 17:
		return 17 // 17 m
	default:
		return 0 // No clustering needed for zoom levels ≥18
	}
}

// clusterMarkers clusters markers based on the distance threshold
func clusterMarkers(markers []database.Marker, distanceThreshold float64) [][]database.Marker {
	var clusters [][]database.Marker
	visited := make([]bool, len(markers))

	for i := 0; i < len(markers); i++ {
		if visited[i] {
			continue
		}
		cluster := []database.Marker{markers[i]}
		visited[i] = true

		for j := i + 1; j < len(markers); j++ {
			if visited[j] {
				continue
			}
			distance := haversineDistance(markers[i].Lat, markers[i].Lon, markers[j].Lat, markers[j].Lon)
			if distance <= distanceThreshold {
				cluster = append(cluster, markers[j])
				visited[j] = true
				if len(cluster) >= 10 { // Limit cluster size to 10 markers
					break
				}
			}
		}
		clusters = append(clusters, cluster)
	}

	return clusters
}

// calculateAverages calculates average values for a cluster of markers
func calculateAverages(markers []database.Marker) (avgDoseRate, avgCountRate, avgSpeed, avgLat, avgLon float64, date int64) {
	var sumDoseRate, sumCountRate, sumSpeed, sumLat, sumLon float64
	var totalTime float64

	// Sort markers by date
	sort.Slice(markers, func(i, j int) bool {
		return markers[i].Date < markers[j].Date
	})

	for i, m := range markers {
		sumDoseRate += m.DoseRate
		sumCountRate += m.CountRate
		sumLat += m.Lat
		sumLon += m.Lon

		if i > 0 {
			prevM := markers[i-1]
			distance := haversineDistance(prevM.Lat, prevM.Lon, m.Lat, m.Lon)
			timeDiff := float64(m.Date - prevM.Date)

			if timeDiff > 0 {
				speed := distance / timeDiff
				sumSpeed += speed
				totalTime += timeDiff
			}
		}
	}

	n := float64(len(markers))
	avgDoseRate = sumDoseRate / n
	avgCountRate = sumCountRate / n
	avgLat = sumLat / n
	avgLon = sumLon / n

	if totalTime > 0 {
		avgSpeed = sumSpeed / (n - 1)
	} else {
		avgSpeed = 0
	}

	// Use the date of the last marker
	date = markers[len(markers)-1].Date

	return
}

// Helper function to parse a float from string
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
	// Europe and Africa
	case lon >= -10 && lon <= 0: // UK, Portugal
		loc, _ := time.LoadLocation("Europe/London")
		return loc
	case lon > 0 && lon <= 15: // Central Europe (Germany, France, Spain)
		loc, _ := time.LoadLocation("Europe/Berlin")
		return loc
	case lon > 15 && lon <= 30: // Eastern Europe (Ukraine, Romania, Greece)
		loc, _ := time.LoadLocation("Europe/Kiev")
		return loc
	case lon > 30 && lon <= 45: // Further East (Moscow, parts of Russia)
		loc, _ := time.LoadLocation("Europe/Moscow")
		return loc
	case lon > 45 && lon <= 60: // Ural region
		loc, _ := time.LoadLocation("Asia/Yekaterinburg")
		return loc
	case lon > 60 && lon <= 90: // Western Siberia
		loc, _ := time.LoadLocation("Asia/Novosibirsk")
		return loc
	case lon > 90 && lon <= 120: // Eastern Siberia
		loc, _ := time.LoadLocation("Asia/Irkutsk")
		return loc
	case lon > 120 && lon <= 135: // Far East (Yakutsk)
		loc, _ := time.LoadLocation("Asia/Yakutsk")
		return loc
	case lon > 135 && lon <= 180: // Far East (Vladivostok)
		loc, _ := time.LoadLocation("Asia/Vladivostok")
		return loc

	// Americas
	case lon >= -180 && lon < -150: // Alaska
		loc, _ := time.LoadLocation("America/Anchorage")
		return loc
	case lon >= -150 && lon < -120: // Pacific Time (USA West Coast)
		loc, _ := time.LoadLocation("America/Los_Angeles")
		return loc
	case lon >= -120 && lon < -90: // Mountain Time
		loc, _ := time.LoadLocation("America/Denver")
		return loc
	case lon >= -90 && lon < -60: // Central Time
		loc, _ := time.LoadLocation("America/Chicago")
		return loc
	case lon >= -60 && lon < -30: // Eastern Time (East Coast USA)
		loc, _ := time.LoadLocation("America/New_York")
		return loc
	case lon >= -30 && lon < 0: // Atlantic Time (Eastern Canada)
		loc, _ := time.LoadLocation("America/Halifax")
		return loc

	// Asia
	case lon >= 60 && lon < 75: // Pakistan
		loc, _ := time.LoadLocation("Asia/Karachi")
		return loc
	case lon >= 75 && lon < 90: // India
		loc, _ := time.LoadLocation("Asia/Kolkata")
		return loc
	case lon >= 90 && lon < 105: // Bangladesh
		loc, _ := time.LoadLocation("Asia/Dhaka")
		return loc
	case lon >= 105 && lon < 120: // Thailand, Vietnam
		loc, _ := time.LoadLocation("Asia/Bangkok")
		return loc
	case lon >= 120 && lon < 135: // China
		loc, _ := time.LoadLocation("Asia/Shanghai")
		return loc
	case lon >= 135 && lon < 150: // Japan
		loc, _ := time.LoadLocation("Asia/Tokyo")
		return loc
	case lon >= 150 && lon <= 180: // Australia East Coast
		loc, _ := time.LoadLocation("Australia/Sydney")
		return loc

	// Default to UTC for undefined regions
	default:
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
func processKMLFile(file multipart.File, trackID string) (uniqueMarkers []database.Marker) {

	uniqueMarkers = []database.Marker{}

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

	// Assign TrackID to each marker
	for i := range markers {
		markers[i].TrackID = trackID
	}

	uniqueMarkers = calculateZoomMarkers(filterZeroMarkers(markers))
	doseData.Markers = append(doseData.Markers, uniqueMarkers...)

	// Save the markers to the database
	for _, marker := range uniqueMarkers {
		err = db.SaveMarkerAtomic(marker, *dbType)
		if err != nil {
			log.Fatalf("Error saving marker: %v", err)
		}
	}
	return
}

// Process and extract data from a KMZ file
func processKMZFile(file multipart.File, trackID string) (uniqueMarkers []database.Marker) {

	uniqueMarkers = []database.Marker{}

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

			// Assign TrackID to each marker
			for i := range markers {
				markers[i].TrackID = trackID
			}

			uniqueMarkers = append(uniqueMarkers, calculateZoomMarkers(filterZeroMarkers(markers))...)

			doseData.Markers = append(doseData.Markers, uniqueMarkers...)

			// Save markers to the database
			for _, marker := range uniqueMarkers {
				err = db.SaveMarkerAtomic(marker, *dbType)
				if err != nil {
					log.Fatalf("Error saving marker: %v", err)
				}
			}
		}
	}
	return
}

// Parse a text RCTRK file, where each line contains timestamp, coordinates, dose rate, and count rate
func parseTextRCTRK(data []byte) ([]database.Marker, error) {
	markers := []database.Marker{}
	lines := strings.Split(string(data), "\n")

	// Iterate over each line and parse its contents
	for i, line := range lines {
		// Skip the header and empty lines
		if i == 0 || strings.HasPrefix(line, "Timestamp") || strings.TrimSpace(line) == "" {
			continue
		}

		// Split the line into fields
		fields := strings.Fields(line)
		if len(fields) < 8 {
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
func processRCTRKFile(file multipart.File, trackID string) (uniqueMarkers []database.Marker) {

	uniqueMarkers = []database.Marker{}

	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Error reading RCTRK file:", err)
		return
	}

	// Try parsing as JSON first

	// Initialize empty rctrkData
	rctrkData := database.Data{
		IsSievert: true, // Set default value
	}

	err = json.Unmarshal(data, &rctrkData)
	if err == nil {

		if rctrkData.IsSievert {
			uniqueMarkers = calculateZoomMarkers(filterZeroMarkers(rctrkData.Markers))
		} else {
			uniqueMarkers = calculateZoomMarkers(filterZeroMarkers(convertRhToSv(rctrkData.Markers)))
		}

		// Assign TrackID to each marker
		for i := range uniqueMarkers {
			uniqueMarkers[i].TrackID = trackID
		}

		doseData.Markers = append(doseData.Markers, uniqueMarkers...)
	} else {
		// If it's not JSON, try parsing as a text format
		markers, err := parseTextRCTRK(data)
		if err != nil {
			log.Println("Error parsing text RCTRK file:", err)
			return
		}

		// Assign TrackID to each marker
		for i := range markers {
			markers[i].TrackID = trackID
		}

		uniqueMarkers = calculateZoomMarkers(filterZeroMarkers(markers))
		doseData.Markers = append(doseData.Markers, uniqueMarkers...)
	}

	// Save markers to the database
	for _, marker := range uniqueMarkers {
		err = db.SaveMarkerAtomic(marker, *dbType)
		if err != nil {
			log.Fatalf("Error saving marker: %v", err)
		}
	}
	return
}

// Process AtomFast JSON file format
func processAtomFastFile(file multipart.File, trackID string) (uniqueMarkers []database.Marker) {

	uniqueMarkers = []database.Marker{}

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
			CountRate: record.D, // AtomFast devices don't provide CPS, assume dose is CPS
			TrackID:   trackID,
		}
		markers = append(markers, marker)
	}

	uniqueMarkers = calculateZoomMarkers(filterZeroMarkers(markers))
	doseData.Markers = append(doseData.Markers, uniqueMarkers...)

	// Save markers to the database
	for _, marker := range uniqueMarkers {
		err = db.SaveMarkerAtomic(marker, *dbType)
		if err != nil {
			log.Fatalf("Error saving marker: %v", err)
		}
	}
	return
}

// =====================
// FILE UPLOAD HANDLERS
// =====================

// uploadHandler теперь генерирует TrackID и обрабатывает его
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

	// Generate a unique TrackID for the group of uploaded files
	trackID := GenerateSerialNumber()

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
			markers = processKMLFile(file, trackID)
		case ".kmz":
			markers = processKMZFile(file, trackID)
		case ".rctrk":
			markers = processRCTRKFile(file, trackID)
		case ".json":
			markers = processAtomFastFile(file, trackID)
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

	// Generate a unique URL for the TrackID
	trackURL := fmt.Sprintf("/trackid/%s?minLat=%f&minLon=%f&maxLat=%f&maxLon=%f&zoom=%d&layer=%s",
		trackID, minLat, minLon, maxLat, maxLon, 11, "OpenStreetMap") // You can dynamically determine zoom and layer if needed

	response := map[string]interface{}{
		"status":   "success",
		"trackURL": trackURL,
		"minLat":   minLat,
		"minLon":   minLon,
		"maxLat":   maxLat,
		"maxLon":   maxLon,
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

	if CompileVersion == "dev" {
		CompileVersion = "latest"
	}

	// Updated struct to include the Lang field
	data := struct {
		Markers      []database.Marker
		Version      string
		Translations map[string]map[string]string // Pass the whole translations map
		Lang         string
	}{
		Markers:      doseData.Markers,
		Version:      CompileVersion,
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

// Новый обработчик для отображения конкретного трека
func trackHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)

	// Extract TrackID from URL
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "TrackID not provided", http.StatusBadRequest)
		return
	}
	trackID := pathParts[2]

	// Extract query parameters
	minLatStr := r.URL.Query().Get("minLat")
	minLonStr := r.URL.Query().Get("minLon")
	maxLatStr := r.URL.Query().Get("maxLat")
	maxLonStr := r.URL.Query().Get("maxLon")
	zoomStr := r.URL.Query().Get("zoom")

	zoomLevel, err := strconv.Atoi(zoomStr)
	if err != nil {
		http.Error(w, "Invalid zoom level", http.StatusBadRequest)
		return
	}

	minLat, err := strconv.ParseFloat(minLatStr, 64)
	if err != nil {
		http.Error(w, "Invalid minLat", http.StatusBadRequest)
		return
	}

	minLon, err := strconv.ParseFloat(minLonStr, 64)
	if err != nil {
		http.Error(w, "Invalid minLon", http.StatusBadRequest)
		return
	}

	maxLat, err := strconv.ParseFloat(maxLatStr, 64)
	if err != nil {
		http.Error(w, "Invalid maxLat", http.StatusBadRequest)
		return
	}

	maxLon, err := strconv.ParseFloat(maxLonStr, 64)
	if err != nil {
		http.Error(w, "Invalid maxLon", http.StatusBadRequest)
		return
	}

	// Fetch markers by TrackID
	markers, err := db.GetMarkersByTrackID(trackID, *dbType)
	if err != nil {
		http.Error(w, "Error fetching markers", http.StatusInternalServerError)
		return
	}

	// Filter markers by geographical bounds and zoom
	var filteredMarkers []database.Marker
	for _, marker := range markers {
		if marker.Zoom == zoomLevel &&
			marker.Lat >= minLat && marker.Lat <= maxLat &&
			marker.Lon >= minLon && marker.Lon <= maxLon {
			filteredMarkers = append(filteredMarkers, marker)
		}
	}

	// Render the same template but with filtered markers
	tmpl := template.Must(template.New("map.html").Funcs(template.FuncMap{
		"translate": func(key string) string {
			if val, ok := translations[lang][key]; ok {
				return val
			}
			return translations["en"][key]
		},
		"toJSON": func(data interface{}) (string, error) {
			bytes, err := json.Marshal(data)
			return string(bytes), err
		},
	}).ParseFS(content, "public_html/map.html"))

	data := struct {
		Markers      []database.Marker
		Version      string
		Translations map[string]map[string]string
		Lang         string
	}{
		Markers:      filteredMarkers,
		Version:      CompileVersion,
		Translations: translations,
		Lang:         lang,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// Handler to serve markers based on zoom level and bounds
func getMarkersHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	zoomStr := r.URL.Query().Get("zoom")
	minLatStr := r.URL.Query().Get("minLat")
	minLonStr := r.URL.Query().Get("minLon")
	maxLatStr := r.URL.Query().Get("maxLat")
	maxLonStr := r.URL.Query().Get("maxLon")

	zoomLevel, err := strconv.Atoi(zoomStr)
	if err != nil {
		http.Error(w, "Invalid zoom level", http.StatusBadRequest)
		return
	}

	minLat, err := strconv.ParseFloat(minLatStr, 64)
	if err != nil {
		http.Error(w, "Invalid minLat", http.StatusBadRequest)
		return
	}

	minLon, err := strconv.ParseFloat(minLonStr, 64)
	if err != nil {
		http.Error(w, "Invalid minLon", http.StatusBadRequest)
		return
	}

	maxLat, err := strconv.ParseFloat(maxLatStr, 64)
	if err != nil {
		http.Error(w, "Invalid maxLat", http.StatusBadRequest)
		return
	}

	maxLon, err := strconv.ParseFloat(maxLonStr, 64)
	if err != nil {
		http.Error(w, "Invalid maxLon", http.StatusBadRequest)
		return
	}

	// Fetch markers from the database
	markers, err := db.GetMarkersByZoomAndBounds(zoomLevel, minLat, minLon, maxLat, maxLon, *dbType)
	if err != nil {
		http.Error(w, "Error fetching markers", http.StatusInternalServerError)
		return
	}

	// Return markers as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(markers)
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
		fmt.Printf("isotope-pathways version %s\n", CompileVersion)
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
	if err := db.InitSchema(dbConfig); err != nil {
		log.Fatalf("Error initializing database schema: %v", err)
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
	http.HandleFunc("/get_markers", getMarkersHandler)
	http.HandleFunc("/trackid/", trackHandler)

	log.Printf("Application running at: http://localhost:%d", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

