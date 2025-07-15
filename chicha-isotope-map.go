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

//go:embed public_html/*
var content embed.FS

var doseData database.Data

var dbType = flag.String("db-type", "sqlite", "Type of the database driver: genji, sqlite, or pgx (postgresql)")
var dbPath = flag.String("db-path", "", "Path to the database file(defaults to the current folder, applicable for genji, sqlite drivers.)")
var dbHost = flag.String("db-host", "127.0.0.1", "Database host (applicable for pgx driver)")
var dbPort = flag.Int("db-port", 5432, "Database port (applicable for pgx driver)")
var dbUser = flag.String("db-user", "postgres", "Database user (applicable for pgx driver)")
var dbPass = flag.String("db-pass", "", "Database password (applicable for pgx driver)")
var dbName = flag.String("db-name", "IsotopePathways", "Database name (applicable for pgx driver)")
var pgSSLMode = flag.String("pg-ssl-mode", "prefer", "PostgreSQL SSL mode: disable, allow, prefer, require, verify-ca, or verify-full")
var port = flag.Int("port", 8765, "Port for running the server")
var version = flag.Bool("version", false, "Show the application version")
var CompileVersion = "dev"

var db *database.Database

// ==========
// Константы для слияния маркеров
// ==========
const (
	markerRadiusPx = 10.0 // радиус кружка в пикселях
)

// GenerateSerialNumber генерирует TrackID
func GenerateSerialNumber() string {
	const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const maxLength = 6

	timestamp := uint64(time.Now().UnixNano() / 1e6) // время в мс
	encoded := ""
	base := uint64(len(base62Chars))

	for timestamp > 0 && len(encoded) < maxLength {
		remainder := timestamp % base
		encoded = string(base62Chars[remainder]) + encoded
		timestamp = timestamp / base
	}

	rand.Seed(time.Now().UnixNano())
	for len(encoded) < maxLength {
		encoded += string(base62Chars[rand.Intn(len(base62Chars))])
	}

	return encoded
}

// convertRhToSv и convertSvToRh - вспомогательные функции перевода
func convertRhToSv(markers []database.Marker) []database.Marker {
	filteredMarkers := []database.Marker{}
	const conversionFactor = 0.01 // 1 Rh = 0.01 Sv

	for _, newMarker := range markers {
		newMarker.DoseRate = newMarker.DoseRate * conversionFactor
		filteredMarkers = append(filteredMarkers, newMarker)
	}
	return filteredMarkers
}
func convertSvToRh(markers []database.Marker) []database.Marker {
	filteredMarkers := []database.Marker{}
	const conversionFactor = 100.0 // 1 Sv = 100 Rh

	for _, newMarker := range markers {
		newMarker.DoseRate = newMarker.DoseRate * conversionFactor
		filteredMarkers = append(filteredMarkers, newMarker)
	}
	return filteredMarkers
}

// filterZeroMarkers убирает маркеры с нулевым значением дозы
func filterZeroMarkers(markers []database.Marker) []database.Marker {
	filteredMarkers := []database.Marker{}
	for _, m := range markers {
		if m.DoseRate == 0 {
			continue
		}
		filteredMarkers = append(filteredMarkers, m)
	}
	return filteredMarkers
}

// Проекция Web Mercator приблизительно переводит широту/долготу в "метры".
// Формулы стандартные, здесь используется для перевода в пиксельные координаты.
func latLonToWebMercator(lat, lon float64) (x, y float64) {
	// const радиус Земли для WebMercator
	const originShift = 2.0 * math.Pi * 6378137.0 / 2.0

	x = lon * originShift / 180.0
	y = math.Log(math.Tan((90.0+lat)*math.Pi/360.0)) / (math.Pi / 180.0)
	y = y * originShift / 180.0
	return x, y
}

// webMercatorToPixel переводит Web Mercator координаты (x,y) в пиксели на данном зуме.
func webMercatorToPixel(x, y float64, zoom int) (px, py float64) {
	// тайл 256x256, увеличиваем в 2^zoom
	scale := math.Exp2(float64(zoom))
	px = (x + 2.0*math.Pi*6378137.0/2.0) / (2.0 * math.Pi * 6378137.0) * 256.0 * scale
	py = (2.0*math.Pi*6378137.0/2.0 - y) / (2.0 * math.Pi * 6378137.0) * 256.0 * scale
	return
}

// latLonToPixel - удобная обёртка
func latLonToPixel(lat, lon float64, zoom int) (px, py float64) {
	x, y := latLonToWebMercator(lat, lon)
	return webMercatorToPixel(x, y, zoom)
}

// pixelToLatLon - нужно, чтобы в процессе усреднения вернуть обратно координаты
func pixelToLatLon(px, py float64, zoom int) (lat, lon float64) {
	scale := math.Exp2(float64(zoom))

	// Обратное преобразование к Web Mercator
	x := px/(256.0*scale)*(2.0*math.Pi*6378137.0) - (2.0*math.Pi*6378137.0)/2.0
	y := (2.0*math.Pi*6378137.0)/2.0 - py/(256.0*scale)*(2.0*math.Pi*6378137.0)

	// Web Mercator -> lat, lon
	lon = (x / (2.0 * math.Pi * 6378137.0)) * 180.0
	lat = (y / (2.0 * math.Pi * 6378137.0)) * 180.0
	lat = 180.0 / math.Pi * (2.0*math.Atan(math.Exp(lat*math.Pi/180.0)) - math.Pi/2.0)
	return lat, lon
}

// mergeMarkersByZoom “сливает” (усредняет) маркеры, которые пересекаются в пиксельных координатах
// на текущем зуме. Если расстояние между центрами меньше 2*markerRadiusPx (плюс 1px “запас”), то объединяем.
func mergeMarkersByZoom(markers []database.Marker, zoom int, radiusPx float64) []database.Marker {
	if len(markers) == 0 {
		return nil
	}

	// Сначала готовим структуру с пиксельными координатами
	type markerPixel struct {
		Marker    database.Marker
		Px, Py    float64
		MergedIdx int // -1, если ни с кем ещё не сливался
	}

	mPixels := make([]markerPixel, len(markers))
	for i, m := range markers {
		px, py := latLonToPixel(m.Lat, m.Lon, zoom)
		mPixels[i] = markerPixel{
			Marker:    m,
			Px:        px,
			Py:        py,
			MergedIdx: -1,
		}
	}

	var result []database.Marker

	// Жадно идём по списку, сливаем близкие друг к другу
	for i := 0; i < len(mPixels); i++ {
		if mPixels[i].MergedIdx != -1 {
			// уже слит с кем-то
			continue
		}
		// начинаем новый кластер
		cluster := []markerPixel{mPixels[i]}
		mPixels[i].MergedIdx = i

		// проверяем всех последующих
		for j := i + 1; j < len(mPixels); j++ {
			if mPixels[j].MergedIdx != -1 {
				continue
			}
			dist := math.Hypot(mPixels[i].Px-mPixels[j].Px, mPixels[i].Py-mPixels[j].Py)
			if dist < 2.0*radiusPx {
				// Сливаем
				cluster = append(cluster, mPixels[j])
				mPixels[j].MergedIdx = i // значит, слит к кластеру i
			}
		}

		// Усредняем данные кластера
		var sumLat, sumLon, sumDose, sumCount float64
		var latestDate int64
		for _, c := range cluster {
			sumLat += c.Marker.Lat
			sumLon += c.Marker.Lon
			sumDose += c.Marker.DoseRate
			sumCount += c.Marker.CountRate
			// возьмём дату последнего
			if c.Marker.Date > latestDate {
				latestDate = c.Marker.Date
			}
		}
		n := float64(len(cluster))
		avgLat := sumLat / n
		avgLon := sumLon / n
		avgDose := sumDose / n
		avgCount := sumCount / n

		var sumSpeed float64
		for _, c := range cluster {
			sumSpeed += c.Marker.Speed
		}
		avgSpeed := sumSpeed / n

		// Создаём новый слитый маркер
		newMarker := database.Marker{
			Lat:       avgLat,
			Lon:       avgLon,
			DoseRate:  avgDose,
			CountRate: avgCount,
			Date:      latestDate,
			Speed:   avgSpeed,
			Zoom:    zoom,
			TrackID: cluster[0].Marker.TrackID, // берем хотя бы у первого
		}
		result = append(result, newMarker)
	}

	return result
}

func calculateSpeedForMarkers(markers []database.Marker) []database.Marker {
    if len(markers) == 0 { // <-- добавлено
        return markers
    }

    sort.Slice(markers, func(i, j int) bool { return markers[i].Date < markers[j].Date })

    for i := 1; i < len(markers); i++ {
        prev, curr := markers[i-1], markers[i]
        dist     := haversineDistance(prev.Lat, prev.Lon, curr.Lat, curr.Lon)
        timeDiff := curr.Date - prev.Date
        if timeDiff > 0 {
            speed := dist / float64(timeDiff)
            if speed >= 0 && speed <= 300 {
                markers[i].Speed = speed
            } else {
                markers[i].Speed = markers[i-1].Speed
            }
        }
    }

    // Обновлённая защита
    if len(markers) > 1 {
        markers[0].Speed = markers[1].Speed
    } else {
        markers[0].Speed = 0
    }
    return markers
}

func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000
	phi1, phi2 := lat1*math.Pi/180, lat2*math.Pi/180
	dPhi, dLambda := (lat2-lat1)*math.Pi/180, (lon2-lon1)*math.Pi/180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * R * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// precomputeMarkersForAllZoomLevels вычисляет заранее маркеры для всех (1..20) уровней зума
// и возвращает итоговый массив
// Базовый радиус, при котором на 20-м зуме маркеры совсем не сливаются

func radiusForZoom(zoom int) float64 {
    // линейная шкала: z=20 → 10 px, z=10 → 5 px, z=5 → 2.5 px …
    return markerRadiusPx * float64(zoom) / 20.0
}

func precomputeMarkersForAllZoomLevels(src []database.Marker) []database.Marker {
    var out []database.Marker
    for z := 1; z <= 20; z++ {
        merged := mergeMarkersByZoom(src, z, radiusForZoom(z))
        out = append(out, merged...)
    }
    return out
}

// =====================
// Транслейт
// =====================
var translations map[string]map[string]string

func loadTranslations(fs embed.FS, filename string) {
	file, err := fs.Open(filename)
	if err != nil {
		log.Fatalf("Error opening translation file: %v", err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("Error reading translation file: %v", err)
	}

	err = json.Unmarshal(data, &translations)
	if err != nil {
		log.Fatalf("Error parsing translations: %v", err)
	}
}

func getPreferredLanguage(r *http.Request) string {
	langHeader := r.Header.Get("Accept-Language")
	supportedLanguages := []string{
		"en", "zh", "es", "hi", "ar", "fr", "ru", "pt", "de", "ja", "tr", "it",
		"ko", "pl", "uk", "mn", "no", "fi", "ka", "sv", "he", "nl", "el", "hu",
		"cs", "ro", "th", "vi", "id", "ms", "bg", "lt", "et", "lv", "sl",
	}
	langs := strings.Split(langHeader, ",")
	for _, lang := range langs {
		lang = strings.TrimSpace(strings.SplitN(lang, ";", 2)[0])
		for _, supported := range supportedLanguages {
			if strings.HasPrefix(lang, supported) {
				return supported
			}
		}
	}
	return "en"
}

// =====================
// Парсинг файлов
// =====================
func parseFloat(value string) float64 {
	parsedValue, _ := strconv.ParseFloat(value, 64)
	return parsedValue
}

func extractDoseRate(description string) float64 {
	re := regexp.MustCompile(`(\d+(\.\d+)?) µR/h`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		// µR/h -> µSv/h
		return parseFloat(match[1]) / 100.0
	}
	return 0
}

func extractCountRate(description string) float64 {
	re := regexp.MustCompile(`(\d+(\.\d+)?) cps`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		return parseFloat(match[1])
	}
	return 0
}

func getTimeZoneByLongitude(lon float64) *time.Location {
	switch {
	case lon >= -10 && lon <= 0:
		loc, _ := time.LoadLocation("Europe/London")
		return loc
	case lon > 0 && lon <= 15:
		loc, _ := time.LoadLocation("Europe/Berlin")
		return loc
	case lon > 15 && lon <= 30:
		loc, _ := time.LoadLocation("Europe/Kiev")
		return loc
	case lon > 30 && lon <= 45:
		loc, _ := time.LoadLocation("Europe/Moscow")
		return loc
	case lon > 45 && lon <= 60:
		loc, _ := time.LoadLocation("Asia/Yekaterinburg")
		return loc
	case lon > 60 && lon <= 90:
		loc, _ := time.LoadLocation("Asia/Novosibirsk")
		return loc
	case lon > 90 && lon <= 120:
		loc, _ := time.LoadLocation("Asia/Irkutsk")
		return loc
	case lon > 120 && lon <= 135:
		loc, _ := time.LoadLocation("Asia/Yakutsk")
		return loc
	case lon > 135 && lon <= 180:
		loc, _ := time.LoadLocation("Asia/Vladivostok")
		return loc

	case lon >= -180 && lon < -150:
		loc, _ := time.LoadLocation("America/Anchorage")
		return loc
	case lon >= -150 && lon < -120:
		loc, _ := time.LoadLocation("America/Los_Angeles")
		return loc
	case lon >= -120 && lon < -90:
		loc, _ := time.LoadLocation("America/Denver")
		return loc
	case lon >= -90 && lon < -60:
		loc, _ := time.LoadLocation("America/Chicago")
		return loc
	case lon >= -60 && lon < -30:
		loc, _ := time.LoadLocation("America/New_York")
		return loc
	case lon >= -30 && lon < 0:
		loc, _ := time.LoadLocation("America/Halifax")
		return loc

	case lon >= 60 && lon < 75:
		loc, _ := time.LoadLocation("Asia/Karachi")
		return loc
	case lon >= 75 && lon < 90:
		loc, _ := time.LoadLocation("Asia/Kolkata")
		return loc
	case lon >= 90 && lon < 105:
		loc, _ := time.LoadLocation("Asia/Dhaka")
		return loc
	case lon >= 105 && lon < 120:
		loc, _ := time.LoadLocation("Asia/Bangkok")
		return loc
	case lon >= 120 && lon < 135:
		loc, _ := time.LoadLocation("Asia/Shanghai")
		return loc
	case lon >= 135 && lon < 150:
		loc, _ := time.LoadLocation("Asia/Tokyo")
		return loc
	case lon >= 150 && lon <= 180:
		loc, _ := time.LoadLocation("Australia/Sydney")
		return loc

	default:
		loc, _ := time.LoadLocation("UTC")
		return loc
	}
}

func parseDate(description string, loc *time.Location) int64 {
	re := regexp.MustCompile(`<b>([A-Za-z]{3} \d{1,2}, \d{4} \d{2}:\d{2}:\d{2})<\/b>`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		dateString := match[1]
		layout := "Jan 2, 2006 15:04:05"
		t, err := time.ParseInLocation(layout, dateString, loc)
		if err == nil {
			return t.Unix()
		}
		log.Println("Error parsing date:", err)
	}
	return 0
}

func parseKML(data []byte) ([]database.Marker, error) {
	var markers []database.Marker
	var longitudes []float64

	coordinatePattern := regexp.MustCompile(`<coordinates>(.*?)<\/coordinates>`)
	descriptionPattern := regexp.MustCompile(`<description><!\[CDATA\[(.*?)\]\]><\/description>`)

	coordinates := coordinatePattern.FindAllStringSubmatch(string(data), -1)
	descriptions := descriptionPattern.FindAllStringSubmatch(string(data), -1)

	for i := 0; i < len(coordinates) && i < len(descriptions); i++ {
		coords := strings.Split(strings.TrimSpace(coordinates[i][1]), ",")
		if len(coords) >= 2 {
			lon := parseFloat(coords[0])
			lat := parseFloat(coords[1])
			longitudes = append(longitudes, lon)

			doseRate := extractDoseRate(descriptions[i][1])
			countRate := extractCountRate(descriptions[i][1])

			marker := database.Marker{
				DoseRate:  doseRate,
				Lat:       lat,
				Lon:       lon,
				CountRate: countRate,
			}
			markers = append(markers, marker)
		}
	}

	var avgLon float64
	for _, lon := range longitudes {
		avgLon += lon
	}
	if len(longitudes) > 0 {
		avgLon /= float64(len(longitudes))
	}
	loc := getTimeZoneByLongitude(avgLon)

	for i := range markers {
		markers[i].Date = parseDate(descriptions[i][1], loc)
	}
	return markers, nil
}
func processAndStoreMarkers(markers []database.Marker, trackID string, db *database.Database, dbType string) error {
	// Устанавливаем TrackID всем маркерам
	for i := range markers {
		markers[i].TrackID = trackID
	}

	// Фильтрация нулевых значений
	markers = filterZeroMarkers(markers)

	// Расчёт скорости
	markers = calculateSpeedForMarkers(markers)

	// Предварительный расчёт для зумов
	allZoomMarkers := precomputeMarkersForAllZoomLevels(markers)

	// Сохраняем маркеры в БД
	for _, m := range allZoomMarkers {
		if err := db.SaveMarkerAtomic(m, dbType); err != nil {
			return fmt.Errorf("error saving marker: %v", err)
		}
	}
	return nil
}

// parseTextRCTRK - парсинг .rctrk текстового
func parseTextRCTRK(data []byte) ([]database.Marker, error) {
	var markers []database.Marker
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 || strings.HasPrefix(line, "Timestamp") || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			log.Printf("Skipping line %d: insufficient fields. Line: %s\n", i+1, line)
			continue
		}
		timeStampStr := fields[1] + " " + fields[2]
		layout := "2006-01-02 15:04:05"
		parsedTime, err := time.Parse(layout, timeStampStr)
		if err != nil {
			log.Printf("Skipping line %d: error parsing time. %v\n", i+1, err)
			continue
		}
		lat := parseFloat(fields[3])
		lon := parseFloat(fields[4])
		if lat == 0 || lon == 0 {
			log.Printf("Skipping line %d: invalid coordinates.\n", i+1)
			continue
		}
		doseRate := parseFloat(fields[6])
		countRate := parseFloat(fields[7])
		if doseRate < 0 || countRate < 0 {
			log.Printf("Skipping line %d: negative dose or count.\n", i+1)
			continue
		}
		marker := database.Marker{
			DoseRate:  doseRate / 100.0,
			CountRate: countRate,
			Lat:       lat,
			Lon:       lon,
			Date:      parsedTime.Unix(),
		}
		markers = append(markers, marker)
	}
	return markers, nil
}

func processKMLFile(file multipart.File, trackID string, db *database.Database, dbType string) error {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error reading KML file: %v", err)
	}
	markers, err := parseKML(data)
	if err != nil {
		return fmt.Errorf("error parsing KML file: %v", err)
	}

	return processAndStoreMarkers(markers, trackID, db, dbType)
}

func processKMZFile(file multipart.File, trackID string, db *database.Database, dbType string) error {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error reading KMZ file: %v", err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("error opening KMZ file as ZIP: %v", err)
	}

	for _, zipFile := range zipReader.File {
		if filepath.Ext(zipFile.Name) == ".kml" {
			kmlFile, err := zipFile.Open()
			if err != nil {
				return fmt.Errorf("error opening KML inside KMZ: %v", err)
			}
			defer kmlFile.Close()

			kmlData, err := io.ReadAll(kmlFile)
			if err != nil {
				return fmt.Errorf("error reading KML inside KMZ: %v", err)
			}
			markers, err := parseKML(kmlData)
			if err != nil {
				return fmt.Errorf("error parsing KML inside KMZ: %v", err)
			}

			if err := processAndStoreMarkers(markers, trackID, db, dbType); err != nil {
				return err
			}
		}
	}
	return nil
}

func processRCTRKFile(file multipart.File, trackID string, db *database.Database, dbType string) error {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error reading RCTRK file: %v", err)
	}
	rctrkData := database.Data{IsSievert: true}

	if err := json.Unmarshal(data, &rctrkData); err == nil {
		markers := rctrkData.Markers
		if !rctrkData.IsSievert {
			markers = convertRhToSv(markers)
		}
		return processAndStoreMarkers(markers, trackID, db, dbType)
	}

	markers, err := parseTextRCTRK(data)
	if err != nil {
		return fmt.Errorf("error parsing text RCTRK file: %v", err)
	}

	return processAndStoreMarkers(markers, trackID, db, dbType)
}

func processAtomFastFile(file multipart.File, trackID string, db *database.Database, dbType string) error {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error reading AtomFast JSON file: %v", err)
	}
	var atomFastData []struct {
		D   float64 `json:"d"`
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
		T   int64   `json:"t"`
	}
	if err := json.Unmarshal(data, &atomFastData); err != nil {
		return fmt.Errorf("error parsing AtomFast file: %v", err)
	}
	var markers []database.Marker
	for _, record := range atomFastData {
		markers = append(markers, database.Marker{
			DoseRate:  record.D,
			Date:      record.T / 1000,
			Lon:       record.Lng,
			Lat:       record.Lat,
			CountRate: record.D,
		})
	}

	return processAndStoreMarkers(markers, trackID, db, dbType)
}

// =====================
// Обработка /upload
// =====================
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "Error uploading file", http.StatusInternalServerError)
		return
	}
	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		http.Error(w, "No files selected", http.StatusBadRequest)
		return
	}
	trackID := GenerateSerialNumber()

	var minLat, minLon, maxLat, maxLon float64
	minLat, minLon = 90.0, 180.0
	maxLat, maxLon = -90.0, -180.0

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			http.Error(w, "Error opening file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
		switch ext {
		case ".kml":
			err = processKMLFile(file, trackID, db, *dbType)
		case ".kmz":
			err = processKMZFile(file, trackID, db, *dbType)
		case ".rctrk":
			err = processRCTRKFile(file, trackID, db, *dbType)
		case ".json":
			err = processAtomFastFile(file, trackID, db, *dbType)
		default:
			http.Error(w, "Unsupported file type", http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// после обработки каждого файла, запросим маркеры из БД, чтобы вычислить границы трека
		markers, err := db.GetMarkersByTrackID(trackID, *dbType)
		if err != nil {
			http.Error(w, "Error fetching markers after upload", http.StatusInternalServerError)
			return
		}

		// Обновим границы
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

	if minLat == 90.0 || minLon == 180.0 || maxLat == -90.0 || maxLon == -180.0 {
		log.Println("Error: Unable to calculate bounds, no valid markers found.")
		http.Error(w, "No valid data in file", http.StatusBadRequest)
		return
	}

	log.Printf("Track: %s bounds: minLat=%f, minLon=%f, maxLat=%f, maxLon=%f\n", trackID, minLat, minLon, maxLat, maxLon)
	trackURL := fmt.Sprintf("/trackid/%s?minLat=%f&minLon=%f&maxLat=%f&maxLon=%f&zoom=14&layer=%s",
		trackID, minLat, minLon, maxLat, maxLon, "OpenStreetMap")

	response := map[string]interface{}{
		"status":   "success",
		"trackURL": trackURL,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// =====================
// WEB
// =====================
func mapHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)
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

	if CompileVersion == "dev" {
		CompileVersion = "latest"
	}
	data := struct {
		Markers      []database.Marker
		Version      string
		Translations map[string]map[string]string
		Lang         string
	}{
		Markers:      doseData.Markers, // Можно при желании показать все (или пусто)
		Version:      CompileVersion,
		Translations: translations,
		Lang:         lang,
	}
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func trackHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "TrackID not provided", http.StatusBadRequest)
		return
	}
	trackID := pathParts[2]

	minLatStr := r.URL.Query().Get("minLat")
	minLonStr := r.URL.Query().Get("minLon")
	maxLatStr := r.URL.Query().Get("maxLat")
	maxLonStr := r.URL.Query().Get("maxLon")

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

	// При загрузке мы уже сохранили маркеры с разными зумами в БД.
	// Но сейчас, если фронтенд не даёт zoom, мы можем показать всё (либо zoom=20).
	// По умолчанию выберите нужный zoom или спрашивайте клиента.
	// Для примера пусть берём максимальный zoom=20:
	markers, err := db.GetMarkersByTrackIDAndBounds(trackID, minLat, minLon, maxLat, maxLon, *dbType)
	if err != nil {
		http.Error(w, "Error fetching markers", http.StatusInternalServerError)
		return
	}
	if len(markers) == 0 {
		http.Error(w, "No markers found for this track", http.StatusNotFound)
		return
	}
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
		Markers:      markers,
		Version:      CompileVersion,
		Translations: translations,
		Lang:         lang,
	}
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// getMarkersHandler — если клиент передаёт zoom и границы,
// можно брать именно те маркеры, что соответствуют запрошенному зуму.
// Тогда никакой дополнительной логики в runtime не нужно — маркеры уже подготовлены.
func getMarkersHandler(w http.ResponseWriter, r *http.Request) {
	zoomStr := r.URL.Query().Get("zoom")
	minLat, _ := strconv.ParseFloat(r.URL.Query().Get("minLat"), 64)
	minLon, _ := strconv.ParseFloat(r.URL.Query().Get("minLon"), 64)
	maxLat, _ := strconv.ParseFloat(r.URL.Query().Get("maxLat"), 64)
	maxLon, _ := strconv.ParseFloat(r.URL.Query().Get("maxLon"), 64)
	zoomLevel, _ := strconv.Atoi(zoomStr)
	trackID := r.URL.Query().Get("trackID")

	var markers []database.Marker
	var err error

	if trackID != "" {
		markers, err = db.GetMarkersByTrackIDZoomAndBounds(trackID, zoomLevel, minLat, minLon, maxLat, maxLon, *dbType)
	} else {
		markers, err = db.GetMarkersByZoomAndBounds(zoomLevel, minLat, minLon, maxLat, maxLon, *dbType)
	}

	if err != nil {
		http.Error(w, "Error fetching markers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(markers)
}

// =====================
// MAIN
// =====================
func main() {
	flag.Parse()
	loadTranslations(content, "public_html/translations.json")

	if *version {
		fmt.Printf("isotope-pathways version %s\n", CompileVersion)
		return
	}

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
	if err := db.InitSchema(dbConfig); err != nil {
		log.Fatalf("Error initializing database schema: %v", err)
	}

	staticFiles, err := fs.Sub(content, "public_html")
	if err != nil {
		log.Fatalf("Failed to extract public_html subdirectory: %v", err)
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))
	http.HandleFunc("/", mapHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/get_markers", getMarkersHandler)
	http.HandleFunc("/trackid/", trackHandler)

	log.Printf("Application running at: http://localhost:%d", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
