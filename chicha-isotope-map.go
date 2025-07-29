package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"net"
	"encoding/json"
	"flag"
	"fmt"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"context"
	"crypto/tls"
	"runtime"
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
	"golang.org/x/crypto/acme/autocert"

	"chicha-isotope-map/pkg/database"
)

//go:embed public_html/*
var content embed.FS

var doseData database.Data

var domain = flag.String("domain", "", "Use 80 and 443 ports. Automatic HTTPS cert via Let's Encrypt.")
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
var defaultLat = flag.Float64("default-lat", 44.08832, "Default map latitude")
var defaultLon = flag.Float64("default-lon", 42.97577, "Default map longitude")
var defaultZoom = flag.Int("default-zoom", 11, "Default map zoom")
var defaultLayer = flag.String("default-layer", "OpenStreetMap", `Default base layer: "OpenStreetMap" or "Google Satellite"`)

var CompileVersion = "dev"

var db *database.Database

// ==========
// Константы для слияния маркеров
// ==========
const (
	markerRadiusPx = 10.0 // радиус кружка в пикселях
  minValidTS     = 1262304000   // 2010-01-01 00:00:00 UTC
)

type SpeedRange struct{ Min, Max float64 }

var speedCatalog = map[string]SpeedRange{
	"ped":   {0, 7},      // < 7 м/с   (~0-25 км/ч)
	"car":   {7, 70},     // 7–70 м/с  (~25-250 км/ч)
	"plane": {70, 500},   // > 70 м/с  (~250-1800 км/ч)
}


// withServerHeader оборачивает любой http.Handler, добавляя
// заголовок "Server: chicha-isotope-map/<CompileVersion>".
//
// На запрос HEAD к “/” сразу отвечает 200 OK без тела, чтобы
// показать, что сервис жив.

func withServerHeader(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "chicha-isotope-map/"+CompileVersion)

		if r.Method == http.MethodHead && r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ServeHTTP(w, r)
	})
}



// serveWithDomain запускает:
//   • :80  — ACME HTTP-01 + 301-redirect на https://<domain>/…
//   • :443 — HTTPS с автоматическими сертификатами Let’s Encrypt.
//
// Новое: если autocert не может выдать cert (любой host/SNI),
//        сервер всё-таки отдаёт ранее полученный fallback-cert,
//        тем самым устраняя «host not configured» в логах.
//
// Совместимость: TLS ≥ 1.0, ALPN h2/http1.1/http1.0.
// Все ошибки только логируются.

func serveWithDomain(domain string, handler http.Handler) {
	// ----------- ACME manager -----------
	certMgr := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache("certs"),
		HostPolicy: func(ctx context.Context, host string) error {
			// Разрешаем голый и www.<domain>
			if host == domain || host == "www."+domain {
				return nil
			}
			// IP-адрес? — не блокируем, просто не пытаемся получить cert.
			if net.ParseIP(host) != nil {
				return nil
			}
			return errors.New("acme/autocert: host not configured")
		},
	}

	// ----------- :80 (challenge + redirect) -----------
	go func() {
		mux80 := http.NewServeMux()
		mux80.Handle("/.well-known/acme-challenge/", certMgr.HTTPHandler(nil))
		mux80.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + domain + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})

		log.Printf("HTTP  server (ACME+redirect) ➜ :80")
		if err := (&http.Server{
			Addr:              ":80",
			Handler:           mux80,
			ReadHeaderTimeout: 10 * time.Second,
		}).ListenAndServe(); err != nil {
			log.Printf("HTTP  server error: %v", err)
		}
	}()

	// ----------- ежедневная проверка сертификата -----------
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for range t.C {
			if _, err := certMgr.GetCertificate(&tls.ClientHelloInfo{ServerName: domain}); err != nil {
				log.Printf("autocert renewal check: %v", err)
			}
		}
	}()

	// ----------- :443 (HTTPS) -----------
	tlsCfg := certMgr.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS10
	tlsCfg.NextProtos = append([]string{"http/1.0"}, tlsCfg.NextProtos...)

	// fallback-сертификат для IP / странных SNI
	var defaultCert *tls.Certificate
	go func() {
		for defaultCert == nil {
			if c, err := certMgr.GetCertificate(&tls.ClientHelloInfo{ServerName: domain}); err == nil {
				defaultCert = c
			}
			time.Sleep(time.Minute)
		}
	}()
	tlsCfg.GetCertificate = func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := certMgr.GetCertificate(chi)
		if err == nil {
			return c, nil
		}
		// Любой сбой — пытаемся отдать fallback-cert (если уже есть)
		if defaultCert != nil {
			return defaultCert, nil
		}
		// Пока fallback нет — повторяем оригинальную ошибку
		return nil, err
	}

	log.Printf("HTTPS server for %s ➜ :443 (TLS ≥1.0, ALPN h2/http1.1/1.0)", domain)
	if err := (&http.Server{
		Addr:              ":443",
		Handler:           handler,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
	}).ListenAndServeTLS("", ""); err != nil {
		log.Printf("HTTPS server error: %v", err)
	}
}


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


// NEW ────────────────
func isValidDate(ts int64) bool {
    // допустимо «сегодня плюс сутки» с учётом часовых поясов
    max := time.Now().Add(24 * time.Hour).Unix()
    return ts >= minValidTS && ts <= max
}

func filterInvalidDateMarkers(markers []database.Marker) []database.Marker {
    out := markers[:0]
    for _, m := range markers {
        if isValidDate(m.Date) {
            out = append(out, m)
        }
    }
    return out
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
			Speed:     avgSpeed,
			Zoom:      zoom,
			TrackID:   cluster[0].Marker.TrackID, // берем хотя бы у первого
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
		dist := haversineDistance(prev.Lat, prev.Lon, curr.Lat, curr.Lon)
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


// -----------------------------------------------------------------------------
// extractDoseRate — извлекает дозу из фрагмента текста.
//  • «12.3 µR/h»  → 0.123 µSv/h   (1 µR/h ≈ 0.01 µSv/h)
//  • «0.136 uSv/h» → 0.136 µSv/h  (Safecast)
// -----------------------------------------------------------------------------
func extractDoseRate(s string) float64 {
	µr := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*µ?R/h`)
	if m := µr.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) / 100.0 // µR/h → µSv/h
	}

	usv := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*uSv/h`)
	if m := usv.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1])          // уже в µSv/h
	}
	return 0
}

// -----------------------------------------------------------------------------
// extractCountRate — ищет счёт • cps • CPM и нормирует к cps.
// -----------------------------------------------------------------------------
func extractCountRate(s string) float64 {
	cps := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*cps`)
	if m := cps.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1])
	}

	cpm := regexp.MustCompile(`(?i)CPM\s*Value\s*=\s*(\d+(?:\.\d+)?)`)
	if m := cpm.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) / 60.0 // 1 мин → секунды
	}
	return 0
}

// -----------------------------------------------------------------------------
// parseDate — поддерживает оба формата:
//  • «May 23, 2012 04:10:08»   (старый .rctrk / AtomFast KML)
//  • «2012/05/23 04:10:08»     (Safecast)
// loc — часовой пояс, рассчитанный по долготе (можно nil → UTC).
// -----------------------------------------------------------------------------
func parseDate(s string, loc *time.Location) int64 {
	if loc == nil {
		loc = time.UTC
	}

	if m := regexp.MustCompile(`([A-Za-z]{3} \d{1,2}, \d{4} \d{2}:\d{2}:\d{2})`).FindStringSubmatch(s); len(m) > 0 {
		const layout = "Jan 2, 2006 15:04:05"
		if t, err := time.ParseInLocation(layout, m[1], loc); err == nil {
			return t.Unix()
		}
	}
	if m := regexp.MustCompile(`(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})`).FindStringSubmatch(s); len(m) > 0 {
		const layout = "2006/01/02 15:04:05"
		if t, err := time.ParseInLocation(layout, m[1], loc); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// -----------------------------------------------------------------------------
// parseKML — универсальный парсер KML (обычный + Safecast).
// Для каждой <Placemark> ищет:
//  • координату  (первые lon,lat)
//  • дозу        — <name> 0.123 uSv/h          или «… µR/h» / «… µSv/h» в <description>
//  • счёт        — «… cps» или «CPM Value = …»
//  • дату        — как в parseDate()
// -----------------------------------------------------------------------------
func parseKML(data []byte) ([]database.Marker, error) {
	var markers []database.Marker

	placemarkRe := regexp.MustCompile(`(?s)<Placemark[^>]*>(.*?)</Placemark>`)
	for _, pm := range placemarkRe.FindAllStringSubmatch(string(data), -1) {
		seg := pm[1]

		// --- координаты ---------------------------------------------------
		coordRe := regexp.MustCompile(`<coordinates>\s*([-\d.]+),([-\d.]+)`)
		cm := coordRe.FindStringSubmatch(seg)
		if len(cm) < 3 {
			continue // без координат — бесполезно
		}
		lon, lat := parseFloat(cm[1]), parseFloat(cm[2])

		// --- текстовые поля ----------------------------------------------
		name := ""
		if m := regexp.MustCompile(`<name>([^<]+)</name>`).FindStringSubmatch(seg); len(m) > 1 {
			name = strings.TrimSpace(m[1])
		}
		desc := ""
		if m := regexp.MustCompile(`(?s)<description[^>]*>(.*?)</description>`).FindStringSubmatch(seg); len(m) > 1 {
			desc = strings.TrimSpace(m[1])
		}

		// --- извлекаем значения ------------------------------------------
		dose   := extractDoseRate(name)
		if dose == 0 {
			dose = extractDoseRate(desc)
		}
		count  := extractCountRate(desc)
		date   := parseDate(desc, getTimeZoneByLongitude(lon))

		markers = append(markers, database.Marker{
			DoseRate:  dose,
			CountRate: count,
			Lat:       lat,
			Lon:       lon,
			Date:      date,
		})
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

	// processAndStoreMarkers()  – сразу после filterZeroMarkers()
	markers = filterInvalidDateMarkers(markers)  // NEW
	if len(markers) == 0 {
		return fmt.Errorf("all markers have invalid dates")
	}

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
// =====================
// WEB  — главная карта
// =====================
func mapHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)

	// Готовим шаблон
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

	// Данные для шаблона
	data := struct {
		Markers      []database.Marker
		Version      string
		Translations map[string]map[string]string
		Lang         string
		DefaultLat   float64
		DefaultLon   float64
		DefaultZoom  int
		DefaultLayer string
	}{
		Markers:      doseData.Markers,
		Version:      CompileVersion,
		Translations: translations,
		Lang:         lang,
		DefaultLat:   *defaultLat,
		DefaultLon:   *defaultLon,
		DefaultZoom:  *defaultZoom,
		DefaultLayer: *defaultLayer,
	}

	// Рендерим в буфер, чтобы не дублировать WriteHeader
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// =====================
// WEB  — страница трека
// =====================
func trackHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)

	// /trackid/<ID>
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "TrackID not provided", http.StatusBadRequest)
		return
	}
	trackID := pathParts[2]

	// Параметры прямоугольника (могут быть нулями)
	minLat, _ := strconv.ParseFloat(r.URL.Query().Get("minLat"), 64)
	minLon, _ := strconv.ParseFloat(r.URL.Query().Get("minLon"), 64)
	maxLat, _ := strconv.ParseFloat(r.URL.Query().Get("maxLat"), 64)
	maxLon, _ := strconv.ParseFloat(r.URL.Query().Get("maxLon"), 64)

	// Маркеры трека в заданных границах (может вернуться пустой срез — это ок)
	markers, err := db.GetMarkersByTrackIDAndBounds(trackID, minLat, minLon, maxLat, maxLon, *dbType)
	if err != nil {
		http.Error(w, "Error fetching markers", http.StatusInternalServerError)
		return
	}

	// Шаблон тот же, что и для главной карты
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
		DefaultLat   float64
		DefaultLon   float64
		DefaultZoom  int
		DefaultLayer string
	}{
		Markers:      markers,
		Version:      CompileVersion,
		Translations: translations,
		Lang:         lang,
		DefaultLat:   *defaultLat,
		DefaultLon:   *defaultLon,
		DefaultZoom:  *defaultZoom,
		DefaultLayer: *defaultLayer,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// getMarkersHandler — берёт маркеры в заданном окне и фильтрах
// +НОВОЕ: dateFrom/dateTo (UNIX-seconds) диапазон времени.
func getMarkersHandler(w http.ResponseWriter, r *http.Request) {
	q              := r.URL.Query()
	zoom, _        := strconv.Atoi(q.Get("zoom"))
	minLat, _      := strconv.ParseFloat(q.Get("minLat"), 64)
	minLon, _      := strconv.ParseFloat(q.Get("minLon"), 64)
	maxLat, _      := strconv.ParseFloat(q.Get("maxLat"), 64)
	maxLon, _      := strconv.ParseFloat(q.Get("maxLon"), 64)
	trackID        := q.Get("trackID")

	// ----- ✈️🚗🚶 фильтр скорости  ---------------------------------
	var sr []database.SpeedRange
	if s := q.Get("speeds"); s != "" {
		for _, tag := range strings.Split(s, ",") {
			if r, ok := speedCatalog[tag]; ok {
				sr = append(sr, database.SpeedRange(r))
			}
		}
	}
	if len(sr) == 0 && q.Get("speeds") != "" { // все выключены
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}

	// ----- ⏱️  фильтр времени  ------------------------------------
	var (
		dateFrom int64
		dateTo   int64
	)
	if s := q.Get("dateFrom"); s != "" {
		dateFrom, _ = strconv.ParseInt(s, 10, 64)
	}
	if s := q.Get("dateTo"); s != "" {
		dateTo, _ = strconv.ParseInt(s, 10, 64)
	}

	// ----- запрос к БД  ------------------------------------------
	var (
		markers []database.Marker
		err     error
	)
	if trackID != "" {
		markers, err = db.GetMarkersByTrackIDZoomBoundsSpeed(
			trackID, zoom, minLat, minLon, maxLat, maxLon,
			dateFrom, dateTo, sr, *dbType)
	} else {
		markers, err = db.GetMarkersByZoomBoundsSpeed(
			zoom, minLat, minLon, maxLat, maxLon,
			dateFrom, dateTo, sr, *dbType)
	}
	if err != nil {
		http.Error(w, "Error fetching markers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(markers)
}

// =====================
// MAIN
// =====================

// main parses flags, initialises the DB & routes, then either
// (a) serves plain HTTP on a custom port, or
// (b) if -domain is given, serves ACME-backed HTTPS on 443 plus
//     an ACME/redirect helper on 80.
//
// If any web-server returns an error it is only logged – the
// application continues running.  A final `select{}` keeps the
// main goroutine alive without resorting to mutexes.




// main: парсинг флагов, инициализация БД и запуск веб-серверов.
// Добавлен withServerHeader для всех запросов.
// =====================
// MAIN
// =====================
func main() {
	// 1. Флаги и версии
	flag.Parse()
	loadTranslations(content, "public_html/translations.json")

	if *version {
		fmt.Printf("chicha-isotope-map version %s\n", CompileVersion)
		return
	}

	// 2. Предупреждение о привилегиях (для :80 / :443)
	if *domain != "" && runtime.GOOS != "windows" && os.Geteuid() != 0 {
		log.Println("⚠  Binding to :80 / :443 requires super-user rights; run with sudo or as root.")
	}

	// 3. База данных
	dbCfg := database.Config{
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
	db, err = database.NewDatabase(dbCfg)
	if err != nil { log.Fatalf("DB init: %v", err) }
	if err = db.InitSchema(dbCfg); err != nil { log.Fatalf("DB schema: %v", err) }

	// 4. Маршруты и статика
	staticFS, err := fs.Sub(content, "public_html")
	if err != nil { log.Fatalf("static fs: %v", err) }

	http.Handle("/static/", http.StripPrefix("/static/",
		http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/",            mapHandler)
	http.HandleFunc("/upload",      uploadHandler)
	http.HandleFunc("/get_markers", getMarkersHandler)
	http.HandleFunc("/trackid/",    trackHandler)

	rootHandler := withServerHeader(http.DefaultServeMux)

	// 5. HTTP/HTTPS-серверы
	if *domain != "" {
		// Двойной сервер :80 + :443 с Let’s Encrypt
		go serveWithDomain(*domain, rootHandler)
	} else {
		// Обычный HTTP на порт из -port
		addr := fmt.Sprintf(":%d", *port)
		go func() {
			log.Printf("HTTP server ➜ http://localhost:%s", addr)
			if err := http.ListenAndServe(addr, rootHandler); err != nil {
				log.Printf("HTTP server error: %v", err)
			}
		}()
	}

	// 6. Держим main-goroutine живой
	select {}
}

