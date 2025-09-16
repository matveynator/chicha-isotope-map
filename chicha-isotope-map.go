//new: stream markers by track

package main

import (

	// http://localhost:8765/debug/pprof/profile?seconds=30
	// go tool pprof -http=:8080 Downloads/profile
	//_ "net/http/pprof"

	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/crypto/acme/autocert"
	"html"
	"html/template"
	"image/color"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"chicha-isotope-map/pkg/database"
	"chicha-isotope-map/pkg/logger"
	"chicha-isotope-map/pkg/qrlogoext"
	safecastrealtime "chicha-isotope-map/pkg/safecast-realtime"
)

//go:embed public_html/*
var content embed.FS

var doseData database.Data

var domain = flag.String("domain", "", "Use 80 and 443 ports. Automatic HTTPS cert via Let's Encrypt.")
var dbType = flag.String("db-type", "sqlite", "Type of the database driver: genji, sqlite, duckdb, or pgx (postgresql)")
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
var safecastRealtimeEnabled = flag.Bool("safecast-realtime", false, "Enable polling and display of Safecast realtime devices")

var CompileVersion = "dev"

var db *database.Database

// ==========
// Константы для слияния маркеров
// ==========
const (
	markerRadiusPx = 10.0       // радиус кружка в пикселях
	minValidTS     = 1262304000 // 2010-01-01 00:00:00 UTC
)

type SpeedRange struct{ Min, Max float64 }

// processBGeigieZenFile parses bGeigie Zen/Nano $BNRDD logs.
// Supports ISO8601 timestamps at field[2] and DMM coordinates with N/S/E/W.
func processBGeigieZenFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
	logT(trackID, "BGEIGIE", "▶ start (stream)")

	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	const cpmPerMicroSv = 334.0
	markers := make([]database.Marker, 0, 4096)

	parsed := 0
	skipped := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "$BNRDD") {
			skipped++
			continue
		}
		if i := strings.IndexByte(line, '*'); i != -1 {
			line = line[:i]
		}
		p := strings.Split(line, ",")
		if len(p) < 6 { // need at least up to CPMvalid
			skipped++
			continue
		}

		var (
			ts  int64
			cps float64
			cpm float64
			lat float64
			lon float64
		)

		// Zen variant: 0:$BNRDD 1:ver 2:ISO8601 3:CPS 4:CPM 5:CPMvalid 6:fix 7:LATdmm 8:N/S 9:LONdmm 10:E/W ...
		if len(p) >= 11 && strings.Contains(p[2], "T") {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(p[2])); err == nil {
				ts = t.Unix()
			}
			cps = parseFloat(p[3])
			cpm = parseFloat(p[4])
			lat = parseDMM(p[7], p[8], 2)
			lon = parseDMM(p[9], p[10], 3)
		} else if len(p) >= 8 { // legacy fallback: decimals (+ optional suffix)
			// We only accept if date/time parse succeeds via known helper; otherwise skip silently.
			// If parseBGeigieDateTime isn't present, ts remains 0 and entry is skipped.
			ts = 0
			// try compact forms if helper exists in build
			// cps/cpm & coords
			cps = parseFloat(p[3])
			cpm = parseFloat(p[4])
			lat = parseBGeigieCoord(p[6])
			lon = parseBGeigieCoord(p[7])
		}

		if ts == 0 || (lat == 0 && lon == 0) {
			skipped++
			continue
		}

		dose := 0.0
		if cpm > 0 {
			dose = cpm / cpmPerMicroSv
		} else if cps > 0 {
			dose = (cps * 60.0) / cpmPerMicroSv
		} else {
			skipped++
			continue
		}

		countRate := cps
		if countRate == 0 && cpm > 0 {
			countRate = cpm / 60.0
		}

		markers = append(markers, database.Marker{
			DoseRate:  dose,
			Date:      ts,
			Lon:       lon,
			Lat:       lat,
			CountRate: countRate,
			Zoom:      0,
			Speed:     0,
			TrackID:   trackID,
		})
		parsed++
	}
	if err := sc.Err(); err != nil {
		return database.Bounds{}, trackID, err
	}
	if len(markers) == 0 {
		return database.Bounds{}, trackID, fmt.Errorf("no valid $BNRDD points found (parsed=%d skipped=%d)", parsed, skipped)
	}

	bbox, trackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return bbox, trackID, err
	}
	logT(trackID, "BGEIGIE", "✔ done (parsed=%d)", parsed)
	return bbox, trackID, nil
}

var speedCatalog = map[string]SpeedRange{
	"ped":   {0, 7},     // < 7 м/с   (~0-25 км/ч)
	"car":   {7, 70},    // 7–70 м/с  (~25-250 км/ч)
	"plane": {70, 1000}, // > 70 м/с  (~250-1800 км/ч)
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

// logT формирует строку "[trackID][component] …" и передаёт её в пакет logger.
// logger сам решит: буферизовать или вывести сразу.
func logT(trackID, component, format string, v ...any) {
	line := fmt.Sprintf("[%-6s][%s] %s", trackID, component, fmt.Sprintf(format, v...))
	logger.Append(trackID, line)
}

// rxFind returns the first submatch (group #1) of pattern in s or an empty string.
// Entities are unescaped and result is TrimSpace-обработан.
func rxFind(s, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		return strings.TrimSpace(html.UnescapeString(m[1]))
	}
	return ""
}

// isClientDisconnect returns true for network errors indicating that the client
// has gone away (e.g., browser navigated away or closed the tab) while we were
// writing the response. These are normal and should not be logged as errors.
func isClientDisconnect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
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

// fastMergeMarkersByZoom группирует маркеры в «ячейку» сетки
// (диаметр = 2*radiusPx) и усредняет данные кластера.
// • O(N) • без мьютексов • подходит для любых зумов.
func fastMergeMarkersByZoom(markers []database.Marker, zoom int, radiusPx float64) []database.Marker {
	if len(markers) == 0 {
		return nil
	}

	cell := 2*radiusPx + 1 // px
	type acc struct {
		sumLat, sumLon, sumDose, sumCnt, sumSp float64
		latest                                 int64
		n                                      int
	}
	cl := make(map[int64]*acc) // key := cx<<32 | cy

	for _, m := range markers {
		px, py := latLonToPixel(m.Lat, m.Lon, zoom)
		key := int64(int(px/cell))<<32 | int64(int32(py/cell))
		a := cl[key]
		if a == nil {
			a = &acc{}
			cl[key] = a
		}
		a.sumLat += m.Lat
		a.sumLon += m.Lon
		a.sumDose += m.DoseRate
		a.sumCnt += m.CountRate
		a.sumSp += m.Speed
		if m.Date > a.latest {
			a.latest = m.Date
		}
		a.n++
	}

	out := make([]database.Marker, 0, len(cl))
	for _, c := range cl {
		n := float64(c.n)
		out = append(out, database.Marker{
			Lat:       c.sumLat / n,
			Lon:       c.sumLon / n,
			DoseRate:  c.sumDose / n,
			CountRate: c.sumCnt / n,
			Speed:     c.sumSp / n,
			Date:      c.latest,
			Zoom:      zoom,
			TrackID:   markers[0].TrackID,
		})
	}
	return out
}

// mergeMarkersByZoom “сливает” (усредняет) маркеры, которые пересекаются в пиксельных координатах
// на текущем зуме. Если расстояние между центрами меньше 2*markerRadiusPx (плюс 1px “запас”), то объединяем.
// deprecated
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

// pickIdentityProbe returns up to 'limit' evenly spaced, non-zero markers
// to cheaply "probe" the DB for an existing track. This avoids thousands
// of random point-lookups on huge tables.
// • No mutexes: pure functional slice logic.
// • Streaming friendly: does not allocate more than needed.
func pickIdentityProbe(src []database.Marker, limit int) []database.Marker {
	if limit <= 0 || len(src) == 0 {
		return nil
	}
	// 1) filter out zero-dose points (they are common and uninformative)
	tmp := make([]database.Marker, 0, min(len(src), limit*2))
	for _, m := range src {
		if m.DoseRate != 0 || m.CountRate != 0 {
			tmp = append(tmp, m)
		}
	}
	if len(tmp) == 0 {
		// fall back to original src if everything was zero
		tmp = src
	}
	// 2) take evenly spaced sample up to 'limit'
	n := len(tmp)
	if n <= limit {
		out := make([]database.Marker, n)
		copy(out, tmp)
		return out
	}
	out := make([]database.Marker, 0, limit)
	stride := n / limit
	if stride <= 0 {
		stride = 1
	}
	for i := 0; i < n && len(out) < limit; i += stride {
		out = append(out, tmp[i])
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// calculateSpeedForMarkers fills Marker.Speed (m/s) for **every** point.
//
// Algorithm
// =========
//  1. Sort markers chronologically (ascending Unix time).
//  2. Pairwise speed: for each neighbour pair with Δt>0 compute v=d/Δt; this
//     gives нам хотя бы несколько валидных скоростей.
//  3. Gap-filling:  for любой непрерывной серии Speed==0
//     ┌ both neighbours exist ─► take avg speed between them
//     ├ only left neighbour    ─► copy its speed
//     └ only right neighbour   ─► copy its speed
//     All markers inside the gap receive the chosen value.
//  4. Fallback: if, after step 3, some zeros still remain (whole track had
//     no Δt>0 inside), we compute average speed between the *first* and the
//     *last* marker and assign it to everyone.
//
// Complexity O(N), lock-free — the slice is owned by this goroutine.
//
// Limits: speeds outside 0…1000 m/s (≈0…3600 km/h) are considered glitches
//
//	and ignored while calculating new values.
//
// calculateSpeedForMarkers recomputes Marker.Speed (m/s) for the whole slice,
// ignoring any pre-filled speeds in the input and auto-normalizing timestamp
// units (milliseconds vs seconds). We keep the function single-pass friendly
// and deterministic, no locks needed (slice is owned by the caller).
//
// Why this change:
//   - Some tracks (e.g., 666.json) carry Unix time in milliseconds, which,
//     if treated as seconds, yields near-zero speeds. We detect ms and divide by 1000.
//   - We never trust/keep incoming Speed values: always recompute from distance/time.
//   - We preserve previous aviation parsing behavior for tracks already in seconds.
//
// Complexity: O(N).
// calculateSpeedForMarkers recomputes Speed (m/s) for all markers,
// normalizing timestamp units (ms → s when needed). We ignore any
// prefilled speeds and derive velocity from geodesic distance / Δt.
//
// Design notes (Go proverbs minded):
//   - Simplicity: single pass with small helpers.
//   - Determinism: slice is owned by caller; no locks, no shared state.
//   - Robustness: auto-detect ms vs s by checking epoch magnitude.
//
// Complexity: O(N).
func calculateSpeedForMarkers(markers []database.Marker) []database.Marker {
	if len(markers) == 0 {
		return markers
	}

	// 1) Chronological order to keep Δt positive and stable.
	sort.Slice(markers, func(i, j int) bool { return markers[i].Date < markers[j].Date })

	// 2) Decide the epoch units once per track:
	//    ~1e9 → seconds (Unix s), ~1e12 → milliseconds (Unix ms).
	//    Check both ends to be safe with mixed sources.
	scale := 1.0 // seconds by default
	if markers[0].Date > 1_000_000_000_000 || markers[len(markers)-1].Date > 1_000_000_000_000 {
		scale = 1000.0 // timestamps are in ms → convert Δt to seconds
	}

	// helper to get Δt in seconds
	dtSec := func(prev, curr int64) float64 {
		if curr <= prev {
			return 0
		}
		return float64(curr-prev) / scale
	}

	const maxSpeed = 1000.0 // m/s, sanity cap for aircraft

	// 3) Recompute pairwise speeds from distance / Δt.
	for i := 1; i < len(markers); i++ {
		dt := dtSec(markers[i-1].Date, markers[i].Date)
		if dt <= 0 {
			continue // duplicate or invalid timestamp
		}
		dist := haversineDistance(
			markers[i-1].Lat, markers[i-1].Lon,
			markers[i].Lat, markers[i].Lon,
		)
		v := dist / dt // m/s
		if v >= 0 && v <= maxSpeed {
			markers[i].Speed = v
		} else {
			// Leave zero if insane (spikes/outliers)
			markers[i].Speed = 0
		}
	}

	// 4) Seed the very first point if needed.
	if len(markers) > 1 && markers[0].Speed == 0 {
		markers[0].Speed = markers[1].Speed
	}

	// 5) Fill zero-speed gaps by borrowing from neighbours.
	lastWithSpeed := -1
	for i := 0; i < len(markers); {
		if markers[i].Speed > 0 {
			lastWithSpeed = i
			i++
			continue
		}
		// zero-run [gapStart..gapEnd]
		gapStart := i
		for i < len(markers) && markers[i].Speed == 0 {
			i++
		}
		gapEnd := i - 1

		// right anchor (if any)
		nextWithSpeed := -1
		if i < len(markers) && markers[i].Speed > 0 {
			nextWithSpeed = i
		}

		var fill float64
		switch {
		case lastWithSpeed != -1 && nextWithSpeed != -1:
			// Prefer average speed derived from anchors distance/time.
			dt := dtSec(markers[lastWithSpeed].Date, markers[nextWithSpeed].Date)
			if dt > 0 {
				dist := haversineDistance(
					markers[lastWithSpeed].Lat, markers[lastWithSpeed].Lon,
					markers[nextWithSpeed].Lat, markers[nextWithSpeed].Lon,
				)
				fill = dist / dt
			}
		case lastWithSpeed != -1:
			fill = markers[lastWithSpeed].Speed
		case nextWithSpeed != -1:
			fill = markers[nextWithSpeed].Speed
		}

		if fill > 0 && fill <= maxSpeed {
			for j := gapStart; j <= gapEnd; j++ {
				markers[j].Speed = fill
			}
		}
	}

	// 6) Global fallback: if anything is still zero, use total distance / total time.
	needFallback := false
	for _, m := range markers {
		if m.Speed == 0 {
			needFallback = true
			break
		}
	}
	if needFallback && len(markers) >= 2 {
		totalDt := dtSec(markers[0].Date, markers[len(markers)-1].Date)
		if totalDt > 0 {
			dist := haversineDistance(
				markers[0].Lat, markers[0].Lon,
				markers[len(markers)-1].Lat, markers[len(markers)-1].Lon,
			)
			v := dist / totalDt
			if v >= 0 && v <= maxSpeed {
				for k := range markers {
					if markers[k].Speed == 0 {
						markers[k].Speed = v
					}
				}
			}
		}
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

func radiusForZoom(zoom int) float64 {
	// линейная шкала: z=20 → 10 px, z=10 → 5 px, z=5 → 2.5 px …
	return markerRadiusPx * float64(zoom) / 20.0
}

// precomputeMarkersForAllZoomLevels создаёт агрегаты для z=1…20
// Параллельно: для каждого зума — своя goroutine, сбор через канал.
func precomputeMarkersForAllZoomLevels(src []database.Marker) []database.Marker {
	type job struct {
		z   int
		out []database.Marker
	}
	ch := make(chan job, 20)

	for z := 1; z <= 20; z++ {
		go func(zoom int) {
			merged := fastMergeMarkersByZoom(src, zoom, radiusForZoom(zoom))
			ch <- job{z: zoom, out: merged}
		}(z)
	}

	var res []database.Marker
	for i := 0; i < 20; i++ {
		res = append(res, (<-ch).out...)
	}
	return res
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
	if langHeader == "" {
		return "en"
	}

	// Поддерживаемые языки (добавлены: da, fa)
	supported := map[string]struct{}{
		"en": {}, "zh": {}, "es": {}, "hi": {}, "ar": {}, "fr": {}, "ru": {}, "pt": {}, "de": {}, "ja": {}, "tr": {}, "it": {},
		"ko": {}, "pl": {}, "uk": {}, "mn": {}, "no": {}, "fi": {}, "ka": {}, "sv": {}, "he": {}, "nl": {}, "el": {}, "hu": {},
		"cs": {}, "ro": {}, "th": {}, "vi": {}, "id": {}, "ms": {}, "bg": {}, "lt": {}, "et": {}, "lv": {}, "sl": {},
		"da": {}, "fa": {},
	}

	// Нормализация/синонимы: приводим варианты к поддерживаемым базовым кодам
	aliases := map[string]string{
		// Устаревшие коды
		"iw": "he", // he (Hebrew)
		"in": "id", // id (Indonesian)

		// Норвежский: часто приходит nb-NO/nn-NO
		"nb": "no",
		"nn": "no",

		// Китайский: сводим к "zh"
		"zh-cn":   "zh",
		"zh-sg":   "zh",
		"zh-hans": "zh",
		"zh-tw":   "zh",
		"zh-hk":   "zh",
		"zh-hant": "zh",

		// Португальский варианты → "pt"
		"pt-br": "pt",
		"pt-pt": "pt",
	}

	langs := strings.Split(langHeader, ",")
	for _, raw := range langs {
		code := strings.TrimSpace(strings.SplitN(raw, ";", 2)[0])
		code = strings.ToLower(strings.ReplaceAll(code, "_", "-"))

		// Берём базовую часть до дефиса (например, "de" из "de-DE")
		base := code
		if i := strings.Index(code, "-"); i != -1 {
			base = code[:i]
		}

		// Применяем алиасы (и к полному коду, и к базе)
		if a, ok := aliases[code]; ok {
			base = a
		} else if a, ok := aliases[base]; ok {
			base = a
		}

		// Проверяем поддержку
		if _, ok := supported[base]; ok {
			return base
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
// extractDoseRate — extracts the dose rate from an arbitrary text fragment.
//
//   - «12.3 µR/h»  → 0.123 µSv/h      (1 µR/h ≈ 0.01 µSv/h, legacy iPhone dump)
//   - «0.136 uSv/h»→ 0.136 µSv/h      (Safecast)
//   - «0.29 мкЗв/ч»→ 0.29  µSv/h      (Radiacode-101 Android, RU locale)
//
// -----------------------------------------------------------------------------
func extractDoseRate(s string) float64 {
	// block: legacy µR/h → µSv/h
	reMicroRh := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*µ?R/h`)
	if m := reMicroRh.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) / 100.0 // convert µR/h → µSv/h
	}

	// block: standard uSv/h (Safecast, iPhone)
	reMicroSv := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*uSv/h`)
	if m := reMicroSv.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) // already in µSv/h
	}

	// block: russian «мкЗв/ч» (μSv/h in Cyrillic)
	reRuMicroSv := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*мк?з?в/ч`)
	if m := reRuMicroSv.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) // already in µSv/h
	}
	return 0
}

// -----------------------------------------------------------------------------
// extractCountRate — searches for count rate and normalises it to cps.
//
//   - «24 cps»      → 24
//   - «1500 CPM»    → 25  (1 min → sec)
//   - «24.7 имп/c»  → 24.7 (Radiacode-101 Android, RU locale)
//
// -----------------------------------------------------------------------------
func extractCountRate(s string) float64 {
	// block: cps (all locales)
	reCPS := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*cps`)
	if m := reCPS.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1])
	}

	// block: CPM (Safecast CSV)
	reCPM := regexp.MustCompile(`(?i)CPM\s*Value\s*=\s*(\d+(?:\.\d+)?)`)
	if m := reCPM.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) / 60.0 // 1 minute → seconds
	}

	// block: russian «имп/с» or «имп/c» (cyrillic / latin 'c')
	reRU := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*имп\s*/\s*[cс]`)
	if m := reRU.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1])
	}
	return 0
}

// -----------------------------------------------------------------------------
// parseDate — recognises three date formats:
//
//   - «May 23, 2012 04:10:08»      (old .rctrk / AtomFast KML)
//   - «2012/05/23 04:10:08»        (Safecast)
//   - «26 июл 2025 11:29:54»       (Radiacode-101 Android, RU locale)
//
// loc — time-zone calculated from longitude (nil → UTC).
// -----------------------------------------------------------------------------
func parseDate(s string, loc *time.Location) int64 {
	if loc == nil {
		loc = time.UTC
	}

	// block: English «Jan 2, 2006 …»
	if m := regexp.MustCompile(`([A-Za-z]{3} \d{1,2}, \d{4} \d{2}:\d{2}:\d{2})`).FindStringSubmatch(s); len(m) > 0 {
		const layout = "Jan 2, 2006 15:04:05"
		if t, err := time.ParseInLocation(layout, m[1], loc); err == nil {
			return t.Unix()
		}
	}

	// block: ISO-ish «2006/01/02 …»
	if m := regexp.MustCompile(`(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})`).FindStringSubmatch(s); len(m) > 0 {
		const layout = "2006/01/02 15:04:05"
		if t, err := time.ParseInLocation(layout, m[1], loc); err == nil {
			return t.Unix()
		}
	}

	// block: Russian «02 янв 2006 …»
	reRu := regexp.MustCompile(`(\d{1,2})\s+([А-Яа-я]{3})\s+(\d{4})\s+(\d{2}:\d{2}:\d{2})`)
	if m := reRu.FindStringSubmatch(s); len(m) > 0 {
		// map short russian month → number
		ruMon := map[string]string{
			"янв": "01", "фев": "02", "мар": "03", "апр": "04",
			"май": "05", "июн": "06", "июл": "07", "авг": "08",
			"сен": "09", "окт": "10", "ноя": "11", "дек": "12",
		}
		monNum, ok := ruMon[strings.ToLower(m[2])]
		if !ok {
			return 0
		}

		// build ISO-like string and parse
		dateStr := fmt.Sprintf("%s-%s-%02s %s", m[3], monNum, m[1], m[4])
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", dateStr, loc); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// =====================================================================================
// parseGPX.go — потоковый парсер GPX 1.1 (AtomSwift)
// =====================================================================================
//
// Формат AtomSwift:
//
//	<trkpt lat="…" lon="…">
//	  <time>2025-04-19T14:57:46Z</time>
//	  …
//	  <extensions>
//	    <atom:marker>
//	       <atom:doserate>0.018526316</atom:doserate>  <!-- µSv/h -->
//	       <atom:cp2s>1.0</atom:cp2s>                   <!-- counts / 2 s -->
//	       <atom:speed>0.41898388</atom:speed>          <!-- m/s -->
//	    </atom:marker>
//	  </extensions>
//
// Все интересующие поля находятся внутри <trkpt>.  Парсим потоково без
// дополнительного выделения памяти, никаких mutex – только канал результатов
// внутри ф-ции (go-routine → main goroutine).
//
// =====================================================================================
// parseGPX (stream) — token-driven GPX 1.1 parser (AtomSwift).
// Uses xml.Decoder directly on io.Reader, so we do *zero* extra allocations.
// =====================================================================================
func parseGPX(trackID string, r io.Reader) ([]database.Marker, error) {
	logT(trackID, "GPX", "parser start (stream)")

	type result struct {
		marker database.Marker
		err    error
	}

	out := make(chan result)
	go func() { // parser goroutine
		defer close(out)

		dec := xml.NewDecoder(r)
		var (
			inTrkpt       bool
			lat, lon      float64
			tUnix, doseSv float64
			count, speed  float64
		)

		for {
			tok, err := dec.Token()
			if err == io.EOF {
				return
			}
			if err != nil {
				out <- result{err: fmt.Errorf("XML decode: %w", err)}
				return
			}

			switch el := tok.(type) {
			case xml.StartElement:
				switch el.Name.Local {
				case "trkpt":
					inTrkpt = true
					lat, lon, tUnix, doseSv, count, speed = 0, 0, 0, 0, 0, 0
					for _, a := range el.Attr {
						if a.Name.Local == "lat" {
							lat = parseFloat(a.Value)
						} else if a.Name.Local == "lon" {
							lon = parseFloat(a.Value)
						}
					}
				case "time":
					if inTrkpt {
						var ts string
						_ = dec.DecodeElement(&ts, &el)
						if tt, err := time.Parse(time.RFC3339, ts); err == nil {
							tUnix = float64(tt.Unix())
						}
					}
				case "doserate":
					if inTrkpt {
						var s string
						_ = dec.DecodeElement(&s, &el)
						doseSv = parseFloat(s)
					}
				case "cp2s":
					if inTrkpt {
						var s string
						_ = dec.DecodeElement(&s, &el)
						count = parseFloat(s) / 2.0
					}
				case "speed":
					if inTrkpt {
						var s string
						_ = dec.DecodeElement(&s, &el)
						speed = parseFloat(s)
					}
				}
			case xml.EndElement:
				if el.Name.Local == "trkpt" && inTrkpt {
					inTrkpt = false
					if doseSv == 0 && count == 0 {
						continue
					}
					out <- result{marker: database.Marker{
						Lat:       lat,
						Lon:       lon,
						Date:      int64(tUnix),
						DoseRate:  doseSv,
						CountRate: count,
						Speed:     speed,
					}}
				}
			}
		}
	}()

	var markers []database.Marker
	for r := range out {
		if r.err != nil {
			logT(trackID, "GPX", "✖ %v", r.err)
			return nil, r.err
		}
		markers = append(markers, r.marker)
	}
	logT(trackID, "GPX", "parser done, parsed=%d markers", len(markers))
	if len(markers) == 0 {
		return nil, fmt.Errorf("no <trkpt> with numeric data found")
	}
	return markers, nil
}

// parseKML (stream) — SAX-style KML parser with *constant* time-zone
// for the whole file.  Fixes wrong speeds on tracks that cross
// several time-zones (e.g. airplanes).
func parseKML(trackID string, r io.Reader) ([]database.Marker, error) {
	logT(trackID, "KML", "parser start (stream)")

	dec := xml.NewDecoder(r)

	var (
		inPlacemark bool
		lat, lon    float64
		name, desc  string
		markers     []database.Marker
		tz          *time.Location // ← NEW: chosen once
		tzLocked    bool           // ←   and then locked
	)

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML decode: %v", err)
		}

		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "Placemark":
				inPlacemark, lat, lon, name, desc = true, 0, 0, "", ""
			case "name":
				if inPlacemark {
					_ = dec.DecodeElement(&name, &el)
				}
			case "description":
				if inPlacemark {
					_ = dec.DecodeElement(&desc, &el)
				}
			case "coordinates":
				if inPlacemark {
					var coord string
					_ = dec.DecodeElement(&coord, &el)
					parts := strings.Split(coord, ",")
					if len(parts) >= 2 {
						lon = parseFloat(parts[0])
						lat = parseFloat(parts[1])
					}
					// ── выбираем TZ только *один раз* ─────────────
					if !tzLocked {
						tz = getTimeZoneByLongitude(lon)
						tzLocked = true
					}
				}
			}
		case xml.EndElement:
			if el.Name.Local == "Placemark" && inPlacemark {
				inPlacemark = false
				dose := extractDoseRate(name)
				if dose == 0 {
					dose = extractDoseRate(desc)
				}
				count := extractCountRate(desc)
				date := parseDate(desc, tz) // ← используем ЕДИНЫЙ TZ
				if dose == 0 && count == 0 {
					continue
				}
				markers = append(markers, database.Marker{
					DoseRate:  dose,
					CountRate: count,
					Lat:       lat,
					Lon:       lon,
					Date:      date,
				})
			}
		}
	}

	logT(trackID, "KML", "parser done, parsed=%d markers", len(markers))
	if len(markers) == 0 {
		return nil, fmt.Errorf("no valid <Placemark> with numeric data found")
	}
	return markers, nil
}

// =====================================================================================
// parseTextRCTRK.go  — теперь принимает trackID
// =====================================================================================
func parseTextRCTRK(trackID string, data []byte) ([]database.Marker, error) {
	logT(trackID, "RCTRK", "text parser start")

	var markers []database.Marker
	lines := strings.Split(string(data), "\n")

	for idx, line := range lines {
		if idx == 0 || strings.HasPrefix(line, "Timestamp") || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			logT(trackID, "RCTRK", "skip line %d: insufficient fields (%d)", idx+1, len(fields))
			continue
		}

		tsStr := fields[1] + " " + fields[2]
		t, err := time.Parse("2006-01-02 15:04:05", tsStr)
		if err != nil {
			logT(trackID, "RCTRK", "skip line %d: time parse error: %v", idx+1, err)
			continue
		}

		lat, lon := parseFloat(fields[3]), parseFloat(fields[4])
		if lat == 0 || lon == 0 {
			logT(trackID, "RCTRK", "skip line %d: invalid coords (%.6f,%.6f)", idx+1, lat, lon)
			continue
		}

		doseRaw, countRaw := parseFloat(fields[6]), parseFloat(fields[7])
		if doseRaw < 0 || countRaw < 0 {
			logT(trackID, "RCTRK", "skip line %d: negative dose/count", idx+1)
			continue
		}

		markers = append(markers, database.Marker{
			DoseRate:  doseRaw / 100.0,
			CountRate: countRaw,
			Lat:       lat,
			Lon:       lon,
			Date:      t.Unix(),
		})
	}

	logT(trackID, "RCTRK", "text parser done, parsed=%d markers", len(markers))
	return markers, nil
}

// =====================================================================================
// parseAtomSwiftCSV (stream) — parses huge .csv produced by AtomSwift logger
// fast & memory-friendly: no ReadAll(), we read record-by-record through bufio.Reader.
// =====================================================================================
func parseAtomSwiftCSV(trackID string, r io.Reader) ([]database.Marker, error) {
	logT(trackID, "CSV", "parser start (stream)")

	br := bufio.NewReaderSize(r, 512*1024) // 512 KiB read-ahead buffer
	cr := csv.NewReader(br)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1 // keep tolerant

	// skip header -----------------------------------------------------------
	if _, err := cr.Read(); err != nil {
		return nil, fmt.Errorf("CSV header: %v", err)
	}

	markers := make([]database.Marker, 0, 4096) // pre-allocate reasonable cap
	rowN := 1
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			logT(trackID, "CSV", "row %d: %v", rowN+1, err)
			continue
		}
		rowN++

		if len(rec) < 7 {
			logT(trackID, "CSV", "skip row %d: insufficient fields (%d)", rowN, len(rec))
			continue
		}

		ts, err := strconv.ParseInt(strings.TrimSpace(rec[0]), 10, 64)
		if err != nil {
			logT(trackID, "CSV", "skip row %d: bad timestamp", rowN)
			continue
		}

		dose := parseFloat(rec[1]) // µSv/h
		lat := parseFloat(rec[2])
		lon := parseFloat(rec[3])
		speed := parseFloat(rec[5]) // m/s
		cps := parseFloat(rec[6])

		if lat == 0 || lon == 0 || dose == 0 {
			continue
		}

		markers = append(markers, database.Marker{
			DoseRate:  dose,
			CountRate: cps,
			Lat:       lat,
			Lon:       lon,
			Date:      ts,
			Speed:     speed,
		})
	}

	if len(markers) == 0 {
		return nil, fmt.Errorf("no valid data rows found")
	}
	logT(trackID, "CSV", "parser done, parsed=%d markers", len(markers))
	return markers, nil
}

// processAtomSwiftCSVFile handles *.csv uploads from AtomSwift logger.
func processAtomSwiftCSVFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	logT(trackID, "CSV", "▶ start (stream)")

	markers, err := parseAtomSwiftCSV(trackID, file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse CSV: %w", err)
	}
	logT(trackID, "CSV", "parsed %d markers", len(markers))

	bbox, trackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return bbox, trackID, err
	}
	logT(trackID, "BGEIGIE", "✔ done")
	return bbox, trackID, nil
}

// processGPXFile handles plain *.gpx uploads in streaming mode.
func processGPXFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
	logT(trackID, "GPX", "▶ start (stream)")

	markers, err := parseGPX(trackID, file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse GPX: %w", err)
	}
	logT(trackID, "GPX", "parsed %d markers", len(markers))

	bbox, trackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return bbox, trackID, err
	}

	logT(trackID, "GPX", "✔ done")
	return bbox, trackID, nil
}

// processKMLFile handles plain *.kml uploads in streaming mode.
func processKMLFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
	logT(trackID, "KML", "▶ start (stream)")

	markers, err := parseKML(trackID, file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse KML: %w", err)
	}
	logT(trackID, "KML", "parsed %d markers", len(markers))

	bbox, trackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return bbox, trackID, err
	}
	logT(trackID, "KML", "✔ done")
	return bbox, trackID, nil
}

// processKMZFile handles *.kmz (ZIP archive with KML inside).
func processKMZFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	logT(trackID, "KMZ", "▶ start")

	data, err := io.ReadAll(file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("read KMZ: %w", err)
	}

	zipR, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("open KMZ as ZIP: %w", err)
	}

	// accumulate bbox of *all* KML entries inside KMZ
	global := database.Bounds{MinLat: 90, MinLon: 180, MaxLat: -90, MaxLon: -180}

	for _, zf := range zipR.File {
		if filepath.Ext(zf.Name) != ".kml" {
			continue
		}

		kmlF, err := zf.Open()
		if err != nil {
			return global, trackID, fmt.Errorf("open %s: %w", zf.Name, err)
		}
		kmlMarkers, err := parseKML(trackID, kmlF)
		_ = kmlF.Close()
		if err != nil {
			return global, trackID, fmt.Errorf("parse %s: %w", zf.Name, err)
		}
		logT(trackID, "KMZ", "parsed %d markers from %q", len(kmlMarkers), zf.Name)

		bbox, trackID, err := processAndStoreMarkers(kmlMarkers, trackID, db, dbType)
		if err != nil {
			return global, trackID, err
		}

		// expand global bbox
		if bbox.MinLat < global.MinLat {
			global.MinLat = bbox.MinLat
		}
		if bbox.MaxLat > global.MaxLat {
			global.MaxLat = bbox.MaxLat
		}
		if bbox.MinLon < global.MinLon {
			global.MinLon = bbox.MinLon
		}
		if bbox.MaxLon > global.MaxLon {
			global.MaxLon = bbox.MaxLon
		}

		logT(trackID, "KMZ", "✔ done")
		return global, trackID, nil

	}

	logT(trackID, "KMZ", "✔ done")
	return global, trackID, nil
}

// -----------------------------------------------------------------------------
// processRCTRKFile — принимает *.rctrk (Radiacode) в JSON- или текстовом виде.
// Поддерживает оба признака единиц: "sv" (новый Android) и "isSievert" (старый iOS).
// Если ни одного флага нет — считаем, что числа уже в µSv/h и конвертацию НЕ делаем.
// -----------------------------------------------------------------------------
func processRCTRKFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	logT(trackID, "RCTRK", "▶ start")

	raw, err := io.ReadAll(file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("read RCTRK: %w", err)
	}

	// ---------- JSON ----------------------------------------------------------
	// По умолчанию оба флага TRUE ⇒ «единицы уже µSv/h».
	data := database.Data{
		IsSievert:       true,
		IsSievertLegacy: true,
	}

	if err := json.Unmarshal(raw, &data); err == nil && len(data.Markers) > 0 {
		logT(trackID, "RCTRK", "JSON detected, %d markers", len(data.Markers))

		// Выясняем, были ли в файле хоть какие-то флаги.
		// Для этого дешево парсим ключи верхнего уровня.
		var keys map[string]json.RawMessage
		_ = json.Unmarshal(raw, &keys) // ошибок игнорируем — структура уже распарсена

		_, hasSV := keys["sv"]
		_, hasOld := keys["isSievert"]

		flagPresent := hasSV || hasOld

		needConvert := flagPresent && (!data.IsSievert || !data.IsSievertLegacy)
		if needConvert {
			logT(trackID, "RCTRK", "µR/h detected → converting to µSv/h")
			data.Markers = convertRhToSv(data.Markers)
		}

		return processAndStoreMarkers(data.Markers, trackID, db, dbType)
	}

	// ---------- plain-text fallback ------------------------------------------
	markers, err := parseTextRCTRK(trackID, raw)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse text RCTRK: %w", err)
	}
	logT(trackID, "RCTRK", "parsed %d markers (text)", len(markers))

	return processAndStoreMarkers(markers, trackID, db, dbType)
}

// processAtomFastFile handles Atom Fast JSON export (*.json).
func processAtomFastFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	logT(trackID, "AtomFast", "▶ start")

	data, err := io.ReadAll(file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("read AtomFast JSON: %w", err)
	}

	var records []struct {
		D   float64 `json:"d"`
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
		T   int64   `json:"t"`
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse AtomFast JSON: %w", err)
	}
	logT(trackID, "AtomFast", "parsed %d markers", len(records))

	markers := make([]database.Marker, 0, len(records))
	for _, r := range records {
		markers = append(markers, database.Marker{
			DoseRate:  r.D,
			CountRate: r.D, // AtomFast stores cps in same field
			Lat:       r.Lat,
			Lon:       r.Lng,
			Date:      r.T / 1000, // ms → s
		})
	}

	return processAndStoreMarkers(markers, trackID, db, dbType)
}

// parseBGeigieCoord parses coordinates that may have hemisphere suffix.
func parseBGeigieCoord(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	r := s[len(s)-1]
	if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
		base := s[:len(s)-1]
		v, _ := strconv.ParseFloat(base, 64)
		switch strings.ToUpper(string(r)) {
		case "S", "W":
			return -v
		default:
			return v
		}
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// parseDMM parses degrees+minutes (DDMM.MMMM or DDDMM.MMMM) with hemisphere.
func parseDMM(val, hemi string, degDigits int) float64 {
	val = strings.TrimSpace(val)
	hemi = strings.TrimSpace(hemi)
	if val == "" {
		return 0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	deg := int(f / 100.0)
	minutes := f - float64(deg*100)
	d := float64(deg) + minutes/60.0
	switch strings.ToUpper(hemi) {
	case "S", "W":
		d = -d
	}
	if degDigits == 2 {
		if d < -90 || d > 90 {
			return 0
		}
	} else {
		if d < -180 || d > 180 {
			return 0
		}
	}
	return d
}

// processAndStoreMarkers is the common pipeline:
// 0. bbox calculation             • O(N)
// 1. fast duplicate-track probe   • O(K·q), K ≪ N (early-exit)
// 2. assign final TrackID         • O(N)
// 3. basic filters                • O(N)
// 4. speed calculation            • O(N)
// 5. pre-compute 20 zoom levels   • O(N) parallel
// 6. batch-insert into DB         • one transaction, multi-row VALUES
//
// Concurrency-friendly: no mutexes; DB/sql pool handles connection safety.
func processAndStoreMarkers(
	markers []database.Marker,
	initTrackID string, // initially generated ID
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	// ── step 0: bounding box (cheap) ────────────────────────────────
	bbox := database.Bounds{MinLat: 90, MinLon: 180, MaxLat: -90, MaxLon: -180}
	for _, m := range markers {
		if m.Lat < bbox.MinLat {
			bbox.MinLat = m.Lat
		}
		if m.Lat > bbox.MaxLat {
			bbox.MaxLat = m.Lat
		}
		if m.Lon < bbox.MinLon {
			bbox.MinLon = m.Lon
		}
		if m.Lon > bbox.MaxLon {
			bbox.MaxLon = m.Lon
		}
	}

	trackID := initTrackID

	// ── step 1: fast probe instead of full-scan ─────────────────────
	// Limit DB random lookups to a tiny sample (e.g. 128 points).
	probe := pickIdentityProbe(markers, 128)
	if existing, err := db.DetectExistingTrackID(probe, 10, dbType); err != nil {
		return bbox, trackID, err
	} else if existing != "" {
		logT(trackID, "Store", "⚠ detected existing trackID %s — reusing", existing)
		trackID = existing
	} else {
		logT(trackID, "Store", "unique track, proceed with new trackID")
	}

	// ── step 2: attach FINAL TrackID ────────────────────────────────
	for i := range markers {
		markers[i].TrackID = trackID
	}

	// ── step 3: light filters ───────────────────────────────────────
	markers = filterZeroMarkers(markers)
	markers = filterInvalidDateMarkers(markers)
	if len(markers) == 0 {
		return bbox, trackID, fmt.Errorf("all markers filtered out")
	}

	// ── step 4: speed calculation (pure Go) ─────────────────────────
	markers = calculateSpeedForMarkers(markers)

	// ── step 5: build aggregates for 20 zooms — O(N)+goroutines ─────
	allZoom := precomputeMarkersForAllZoomLevels(markers)
	logT(trackID, "Store", "precomputed %d zoom-markers", len(allZoom))

	// ── step 6: single transaction + multi-row VALUES ───────────────
	tx, err := db.DB.Begin()
	if err != nil {
		return bbox, trackID, err
	}
	// Batch size 500–1000 usually gives a good balance on large B-Trees.
	if err := db.InsertMarkersBulk(tx, allZoom, dbType, 1000); err != nil {
		_ = tx.Rollback()
		return bbox, trackID, fmt.Errorf("bulk insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return bbox, trackID, err
	}

	logT(trackID, "Store", "✔ stored (new %d markers)", len(allZoom))
	return bbox, trackID, nil
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "multipart parse error", http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		http.Error(w, "no files selected", http.StatusBadRequest)
		return
	}

	trackID := GenerateSerialNumber()
	logT(trackID, "Upload", "▶ start, total=%d", len(files))

	// глобальные границы всего набора файлов
	global := database.Bounds{MinLat: 90, MinLon: 180, MaxLat: -90, MaxLon: -180}

	for _, fh := range files {
		logT(trackID, "Upload", "file received: %s", fh.Filename)

		f, _ := fh.Open()
		// don't defer yet; may re-open for sniffing

		var (
			bbox database.Bounds
			err  error
		)
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		switch ext {
		case ".kml":
			bbox, trackID, err = processKMLFile(f, trackID, db, *dbType)
		case ".kmz":
			bbox, trackID, err = processKMZFile(f, trackID, db, *dbType)
		case ".gpx":
			bbox, trackID, err = processGPXFile(f, trackID, db, *dbType)
		case ".csv":
			bbox, trackID, err = processAtomSwiftCSVFile(f, trackID, db, *dbType)
		case ".rctrk":
			bbox, trackID, err = processRCTRKFile(f, trackID, db, *dbType)
		case ".json":
			bbox, trackID, err = processAtomFastFile(f, trackID, db, *dbType)
		case ".log", ".txt":
			bbox, trackID, err = processBGeigieZenFile(f, trackID, db, *dbType)
		default:
			// Sniff for bGeigie $BNRDD content if extension is missing/wrong
			// Read up to 1024 bytes, then re-open the file for processing if needed
			buf := make([]byte, 1024)
			n, _ := f.Read(buf)
			_ = f.Close()
			content := string(buf[:n])
			if strings.Contains(content, "$BNRDD") {
				logT(trackID, "Upload", "sniffed bGeigie content for %s (ext=%q)", fh.Filename, ext)
				// reopen fresh handle
				f2, _ := fh.Open()
				bbox, trackID, err = processBGeigieZenFile(f2, trackID, db, *dbType)
				_ = f2.Close()
				if err != nil {
					logT(trackID, "Upload", "error processing (sniffed) %s: %v", fh.Filename, err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				break
			}
			logT(trackID, "Upload", "unsupported file type: %s (ext=%q)", fh.Filename, ext)
			http.Error(w, "unsupported file type", http.StatusBadRequest)
			return
		}
		// ensure we close handle if not closed by default/sniff
		_ = f.Close()
		if err != nil {
			logT(trackID, "Upload", "error processing %s: %v", fh.Filename, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// расширяем глобальные границы
		if bbox.MinLat < global.MinLat {
			global.MinLat = bbox.MinLat
		}
		if bbox.MaxLat > global.MaxLat {
			global.MaxLat = bbox.MaxLat
		}
		if bbox.MinLon < global.MinLon {
			global.MinLon = bbox.MinLon
		}
		if bbox.MaxLon > global.MaxLon {
			global.MaxLon = bbox.MaxLon
		}
	}

	trackURL := fmt.Sprintf(
		"/trackid/%s?minLat=%f&minLon=%f&maxLat=%f&maxLon=%f&zoom=14&layer=%s",
		trackID, global.MinLat, global.MinLon, global.MaxLat, global.MaxLon,
		"OpenStreetMap")

	logT(trackID, "Upload", "redirecting browser to: %s", trackURL)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":   "success",
		"trackURL": trackURL,
	}); err != nil {
		if isClientDisconnect(err) {
			log.Printf("client disconnected while writing upload response")
		} else {
			log.Printf("upload response write error: %v", err)
		}
	}
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
		Markers           []database.Marker
		Version           string
		Translations      map[string]map[string]string
		Lang              string
		DefaultLat        float64
		DefaultLon        float64
		DefaultZoom       int
		DefaultLayer      string
		RealtimeAvailable bool
	}{
		Markers:           doseData.Markers,
		Version:           CompileVersion,
		Translations:      translations,
		Lang:              lang,
		DefaultLat:        *defaultLat,
		DefaultLon:        *defaultLon,
		DefaultZoom:       *defaultZoom,
		DefaultLayer:      *defaultLayer,
		RealtimeAvailable: *safecastRealtimeEnabled,
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
		if isClientDisconnect(err) {
			log.Printf("client disconnected while writing response")
		} else {
			log.Printf("Error writing response: %v", err)
		}
	}
}

// =====================
// WEB  — страница трека
// =====================
// trackHandler — страница одного трека.
// Теперь НЕ загружает маркеры в HTML: JS сам запросит нужный зум.
func trackHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)

	// /trackid/<ID>
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "TrackID not provided", http.StatusBadRequest)
		return
	}
	trackID := parts[2] // всё равно понадобится в JS

	// --- шаблон ----------------------------------------------------------------
	tmpl := template.Must(template.New("map.html").Funcs(template.FuncMap{
		"translate": func(key string) string {
			if v, ok := translations[lang][key]; ok {
				return v
			}
			return translations["en"][key]
		},
		"toJSON": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
	}).ParseFS(content, "public_html/map.html"))

	// отдаём пустой срез маркеров
	data := struct {
		Markers           []database.Marker
		Version           string
		Translations      map[string]map[string]string
		Lang              string
		DefaultLat        float64
		DefaultLon        float64
		DefaultZoom       int
		DefaultLayer      string
		RealtimeAvailable bool
	}{
		Markers:           nil, // ← ключевое изменение
		Version:           CompileVersion,
		Translations:      translations,
		Lang:              lang,
		DefaultLat:        *defaultLat,
		DefaultLon:        *defaultLon,
		DefaultZoom:       *defaultZoom,
		DefaultLayer:      *defaultLayer,
		RealtimeAvailable: *safecastRealtimeEnabled,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		if isClientDisconnect(err) {
			log.Printf("client disconnected while writing response")
		} else {
			log.Printf("write resp: %v", err)
		}
	}

	// Ради отладки: показываем, что HTML отдали без тяжёлых данных
	log.Printf("Track page %s rendered.", trackID)
}

// import "image/color"
// import "os" (если будешь читать логотип с диска)
// import "chicha-isotope-map/pkg/qrlogoext"

func qrPngHandler(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("u")
	if u == "" {
		if ref := r.Referer(); ref != "" {
			u = ref
		} else {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			u = scheme + "://" + r.Host + r.URL.RequestURI()
		}
	}
	if len(u) > 4096 {
		u = u[:4096]
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", "inline; filename=\"qr.png\"")

	// (А) если логотип лежит в файле:
	// logoBytes, _ := os.ReadFile("static/radiation.png")

	// (Б) или без файла — пусть пакет нарисует знак сам:
	var logoBytes []byte

	opts := qrlogoext.Options{
		TargetPx:    1500,
		Fg:          color.RGBA{0, 0, 0, 255},       // чёрные модули
		Bg:          color.RGBA{255, 255, 255, 255}, // БЕЛЫЙ фон
		Logo:        color.RGBA{233, 192, 35, 255},  // ЖЕЛТЫЙ знак радиации
		LogoBoxFrac: 0.32,                           // большой центральный квадрат
		LogoPadding: 16,                             // отступ для картинки (если PNG вставляешь)
	}

	if err := qrlogoext.EncodePNG(w, []byte(u), logoBytes, opts); err != nil {
		http.Error(w, "QR encode: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// getMarkersHandler — берёт маркеры в заданном окне и фильтрах
// +НОВОЕ: dateFrom/dateTo (UNIX-seconds) диапазон времени.
func getMarkersHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zoom, _ := strconv.Atoi(q.Get("zoom"))
	minLat, _ := strconv.ParseFloat(q.Get("minLat"), 64)
	minLon, _ := strconv.ParseFloat(q.Get("minLon"), 64)
	maxLat, _ := strconv.ParseFloat(q.Get("maxLat"), 64)
	maxLon, _ := strconv.ParseFloat(q.Get("maxLon"), 64)
	trackID := q.Get("trackID")

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

	if *safecastRealtimeEnabled {
		// We only touch realtime tables when the operator explicitly enables the feature.
		if rt, err := db.GetLatestRealtimeByBounds(minLat, minLon, maxLat, maxLon, *dbType); err == nil {
			markers = append(markers, rt...)
		} else {
			log.Printf("realtime query: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(markers)
}

// ========
// Streaming markers via SSE
// ========

// aggregateMarkers chooses the most radioactive marker per grid cell.
// Cells shrink with higher zoom to preserve detail.
func aggregateMarkers(ctx context.Context, in <-chan database.Marker, zoom int) <-chan database.Marker {
	out := make(chan database.Marker)
	go func() {
		defer close(out)
		cells := make(map[string]database.Marker)
		scale := math.Pow(2, float64(zoom))
		for {
			select {
			case <-ctx.Done():
				return
			case m, ok := <-in:
				if !ok {
					return
				}
				key := fmt.Sprintf("%d:%d", int(m.Lat*scale), int(m.Lon*scale))
				if prev, ok := cells[key]; !ok || m.DoseRate > prev.DoseRate {
					cells[key] = m
					out <- m
				}
			}
		}
	}()
	return out
}

// streamMarkersHandler streams markers via Server-Sent Events.
// Markers are emitted as soon as they are read and aggregated.
func streamMarkersHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zoom, _ := strconv.Atoi(q.Get("zoom"))
	minLat, _ := strconv.ParseFloat(q.Get("minLat"), 64)
	minLon, _ := strconv.ParseFloat(q.Get("minLon"), 64)
	maxLat, _ := strconv.ParseFloat(q.Get("maxLat"), 64)
	maxLon, _ := strconv.ParseFloat(q.Get("maxLon"), 64)
	trackID := q.Get("trackID")
	// Choose streaming source: either entire map or a single track.
	ctx := r.Context()
	var (
		baseSrc <-chan database.Marker
		errCh   <-chan error
	)
	if trackID != "" {
		baseSrc, errCh = db.StreamMarkersByTrackIDZoomAndBounds(ctx, trackID, zoom, minLat, minLon, maxLat, maxLon, *dbType)
	} else {
		baseSrc, errCh = db.StreamMarkersByZoomAndBounds(ctx, zoom, minLat, minLon, maxLat, maxLon, *dbType)
	}

	// Fetch current realtime points once so the map reflects network devices.
	// We only touch the realtime table when the dedicated flag enables it so
	// operators control the feature explicitly.
	var rtMarks []database.Marker
	if *safecastRealtimeEnabled {
		var err error
		rtMarks, err = db.GetLatestRealtimeByBounds(minLat, minLon, maxLat, maxLon, *dbType)
		if err != nil {
			log.Printf("realtime query: %v", err)
		}
		// Log bounds alongside count to help diagnose empty map tiles.
		log.Printf("realtime markers: %d lat[%f,%f] lon[%f,%f]", len(rtMarks), minLat, maxLat, minLon, maxLon)
	}

	agg := aggregateMarkers(ctx, baseSrc, zoom)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Emit realtime markers first when enabled.
	for _, m := range rtMarks {
		b, _ := json.Marshal(m)
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				fmt.Fprintf(w, "event: done\ndata: %v\n\n", err)
			} else {
				fmt.Fprint(w, "event: done\ndata: end\n\n")
			}
			flusher.Flush()
			return
		case m, ok := <-agg:
			if !ok {
				fmt.Fprint(w, "event: done\ndata: end\n\n")
				flusher.Flush()
				return
			}
			b, _ := json.Marshal(m)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// realtimeHistoryHandler returns one year of realtime measurements for a device.
// The handler keeps the response lightweight so the frontend can draw Grafana-style
// charts without shipping a dedicated dashboard backend.
func realtimeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if !*safecastRealtimeEnabled {
		http.NotFound(w, r)
		return
	}

	device := strings.TrimSpace(r.URL.Query().Get("device"))
	if device == "" {
		http.Error(w, "missing device", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(device, "live:") {
		device = strings.TrimPrefix(device, "live:")
	}

	now := time.Now()
	since := now.Add(-365 * 24 * time.Hour).Unix()
	dayCutoff := now.Add(-24 * time.Hour).Unix()
	monthCutoff := now.Add(-30 * 24 * time.Hour).Unix()

	rows, err := db.GetRealtimeHistory(device, since, *dbType)
	if err != nil {
		http.Error(w, "history error", http.StatusInternalServerError)
		return
	}

	type point struct {
		Timestamp int64   `json:"timestamp"`
		Value     float64 `json:"value"`
	}

	series := map[string][]point{
		"day":   {},
		"month": {},
		"year":  {},
	}

	var (
		metaName, metaTransport, metaTube, metaCountry string
		extra                                          map[string]float64
	)

	for _, m := range rows {
		val, ok := safecastrealtime.FromRealtime(m.Value, m.Unit)
		if !ok {
			continue
		}

		pt := point{Timestamp: m.MeasuredAt, Value: val}
		series["year"] = append(series["year"], pt)
		if m.MeasuredAt >= monthCutoff {
			series["month"] = append(series["month"], pt)
		}
		if m.MeasuredAt >= dayCutoff {
			series["day"] = append(series["day"], pt)
		}

		if metaName == "" && m.DeviceName != "" {
			metaName = m.DeviceName
		}
		if metaTransport == "" && m.Transport != "" {
			metaTransport = m.Transport
		}
		if metaTube == "" && m.Tube != "" {
			metaTube = m.Tube
		}
		if metaCountry == "" && m.Country != "" {
			metaCountry = m.Country
		}

		trimmed := strings.TrimSpace(m.Extra)
		if trimmed != "" {
			var parsed map[string]float64
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				extra = parsed
			}
		}
	}

	resp := struct {
		DeviceID   string             `json:"deviceID"`
		DeviceName string             `json:"deviceName,omitempty"`
		Transport  string             `json:"transport,omitempty"`
		Tube       string             `json:"tube,omitempty"`
		Country    string             `json:"country,omitempty"`
		Series     map[string][]point `json:"series"`
		Extra      map[string]float64 `json:"extra,omitempty"`
	}{
		DeviceID:   device,
		DeviceName: metaName,
		Transport:  metaTransport,
		Tube:       metaTube,
		Country:    metaCountry,
		Series:     series,
		Extra:      extra,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
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
	if err != nil {
		log.Fatalf("DB init: %v", err)
	}
	if err = db.InitSchema(dbCfg); err != nil {
		log.Fatalf("DB schema: %v", err)
	}

	if *safecastRealtimeEnabled {
		// Launch realtime Safecast polling under the dedicated flag so the
		// feature stays opt-in.
		database.SetRealtimeConverter(safecastrealtime.FromRealtime)
		ctxRT, cancelRT := context.WithCancel(context.Background())
		defer cancelRT()
		safecastrealtime.Start(ctxRT, db, *dbType, log.Printf)
	}

	// 4. Маршруты и статика
	staticFS, err := fs.Sub(content, "public_html")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}

	http.Handle("/static/", http.StripPrefix("/static/",
		http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/", mapHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/get_markers", getMarkersHandler)
	http.HandleFunc("/stream_markers", streamMarkersHandler)
	http.HandleFunc("/realtime_history", realtimeHistoryHandler)
	http.HandleFunc("/trackid/", trackHandler)
	http.HandleFunc("/qrpng", qrPngHandler)

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

	// асинхронные индексы в бд без блокирования основного процесса начало
	ctxIdx, cancelIdx := context.WithCancel(context.Background())
	defer cancelIdx()
	// Пояснение в лог: что делаем и почему это не блокирует сервер
	log.Printf("⏳ background index build scheduled (engine=%s). Listeners are up; pages may be slower until indexes are ready.", dbCfg.DBType)
	// Запуск асинхронной индексации с прогрессом
	db.EnsureIndexesAsync(ctxIdx, dbCfg, func(format string, args ...any) {
		log.Printf(format, args...)
	})
	// асинхронные индексы в бд без блокирования основного процесса конец

	// 6. Держим main-goroutine живой
	select {}
}
