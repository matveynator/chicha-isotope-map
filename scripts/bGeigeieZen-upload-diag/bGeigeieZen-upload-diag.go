package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
	"chicha-isotope-map/pkg/database/drivers"
	"chicha-isotope-map/pkg/logger"
)

// ---- CLI flags (independent of the main app) ----
var (
	port          = flag.Int("port", 8877, "Port for running the diagnostics server")
	dbType        = flag.String("db-type", "sqlite", "Database driver: chai | sqlite | duckdb | pgx | clickhouse")
	dbPath        = flag.String("db-path", "", "Database file path (for sqlite/chai/duckdb)")
	dbHost        = flag.String("db-host", "127.0.0.1", "DB host (pgx or clickhouse)")
	dbPort        = flag.Int("db-port", 5432, "DB port (pgx defaults to 5432, clickhouse commonly uses 9000)")
	dbUser        = flag.String("db-user", "postgres", "DB user (pgx or clickhouse)")
	dbPass        = flag.String("db-pass", "", "DB password (pgx or clickhouse)")
	dbName        = flag.String("db-name", "IsotopePathways", "DB name (pgx or clickhouse)")
	pgSSL         = flag.String("pg-ssl-mode", "prefer", "PostgreSQL SSL mode")
	clickhouseTLS = flag.Bool("clickhouse-secure", false, "Enable TLS for clickhouse connections")
)

var db *database.Database

func init() {
	// Diagnostics may be run with "go run bGeigeieZen-upload-diag.go" so we
	// register drivers here to keep behaviour identical to full builds.
	drivers.Ready()
}

// ---- Logging helper compatible with main app ----
func logT(trackID, component, format string, v ...any) {
	line := fmt.Sprintf("[%-6s][%s] %s", trackID, component, fmt.Sprintf(format, v...))
	logger.Append(trackID, line)
}

// ---- Small helpers reused from the main app ----
func GenerateSerialNumber() string {
	const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const maxLength = 6

	timestamp := uint64(time.Now().UnixNano() / 1e6)
	encoded := ""
	base := uint64(len(base62Chars))
	for timestamp > 0 && len(encoded) < maxLength {
		remainder := timestamp % base
		encoded = string(base62Chars[remainder]) + encoded
		timestamp = timestamp / base
	}
	for len(encoded) < maxLength {
		encoded += string(base62Chars[time.Now().UnixNano()%int64(len(base62Chars))])
	}
	return encoded
}

// ---- Diag structures and helpers ----

type diagStats struct {
	Records int                 `json:"records"`
	Parsed  int                 `json:"parsed"`
	Skipped int                 `json:"skipped"`
	Reasons map[string]int      `json:"reasons"`
	Samples map[string][]string `json:"samples"`
}

type diagResult struct {
	File     string           `json:"file"`
	Format   string           `json:"formatDetected"`
	Status   string           `json:"status"` // ok | error | skipped
	Error    string           `json:"error,omitempty"`
	TrackURL string           `json:"trackURL,omitempty"`
	Bounds   *database.Bounds `json:"bounds,omitempty"`
	Stats    *diagStats       `json:"stats,omitempty"`
}

type diagResponse struct {
	Status  string       `json:"status"` // ok | mixed | error
	Results []diagResult `json:"results"`
}

func bgnParseDMM(val, hemi string, degDigits int) float64 {
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

func bgnParseFloat(s string) float64 { v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return v }

// bgnParseCoord parses numbers with optional N/S/E/W suffix
func bgnParseCoord(s string) float64 {
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

// bgnParseDateTime supports YYYYMMDD + HHMMSS(.sss) and DDMMYY + HHMMSS(.sss)
func bgnParseDateTime(d, t string) int64 {
	d = strings.TrimSpace(d)
	t = strings.TrimSpace(t)
	if len(d) == 8 { // YYYYMMDD
		if ts, err := time.Parse("20060102 150405.000", d+" "+t); err == nil {
			return ts.Unix()
		}
		if ts, err := time.Parse("20060102 150405", d+" "+t); err == nil {
			return ts.Unix()
		}
	}
	if len(d) == 6 { // DDMMYY
		dd, mm, yy := d[0:2], d[2:4], d[4:6]
		yyyy := "20" + yy
		ymd := yyyy + mm + dd
		if ts, err := time.Parse("20060102 150405.000", ymd+" "+t); err == nil {
			return ts.Unix()
		}
		if ts, err := time.Parse("20060102 150405", ymd+" "+t); err == nil {
			return ts.Unix()
		}
	}
	return 0
}

// ---- Core processing (copied from main, distilled) ----

func processAndStoreMarkers(
	markers []database.Marker,
	initTrackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
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
	probe := pickIdentityProbe(markers, 128)
	if existing, err := db.DetectExistingTrackID(probe, 10, dbType); err != nil {
		return bbox, trackID, err
	} else if existing != "" {
		logT(trackID, "Store", "reuse existing trackID %s", existing)
		trackID = existing
	} else {
		logT(trackID, "Store", "unique track, creating")
	}

	for i := range markers {
		markers[i].TrackID = trackID
	}

	markers = filterZeroMarkers(markers)
	markers = filterInvalidDateMarkers(markers)
	if len(markers) == 0 {
		return bbox, trackID, fmt.Errorf("all markers filtered out")
	}

	markers = calculateSpeedForMarkers(markers)
	allZoom := precomputeMarkersForAllZoomLevels(markers)
	logT(trackID, "Store", "precomputed %d zoom-markers", len(allZoom))

	if strings.EqualFold(dbType, "clickhouse") {
		if err := db.InsertMarkersBulk(nil, allZoom, dbType, 1000); err != nil {
			return bbox, trackID, fmt.Errorf("bulk insert: %w", err)
		}
	} else {
		tx, err := db.DB.Begin()
		if err != nil {
			return bbox, trackID, err
		}
		if err := db.InsertMarkersBulk(tx, allZoom, dbType, 1000); err != nil {
			_ = tx.Rollback()
			return bbox, trackID, fmt.Errorf("bulk insert: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return bbox, trackID, err
		}
	}
	logT(trackID, "Store", "stored %d markers", len(allZoom))
	return bbox, trackID, nil
}

func pickIdentityProbe(src []database.Marker, limit int) []database.Marker {
	if limit <= 0 || len(src) == 0 {
		return nil
	}
	tmp := make([]database.Marker, 0, min(len(src), limit*2))
	for _, m := range src {
		if m.DoseRate != 0 || m.CountRate != 0 {
			tmp = append(tmp, m)
		}
	}
	if len(tmp) == 0 {
		tmp = src
	}
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

func filterZeroMarkers(markers []database.Marker) []database.Marker {
	out := markers[:0]
	for _, m := range markers {
		if m.DoseRate != 0 {
			out = append(out, m)
		}
	}
	return out
}

const minValidTS = 1262304000 // 2010-01-01 00:00:00 UTC

func isValidDate(ts int64) bool {
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

func calculateSpeedForMarkers(markers []database.Marker) []database.Marker {
	if len(markers) == 0 {
		return markers
	}
	// sort by time
	sortByDate(markers)
	scale := 1.0
	if markers[0].Date > 1_000_000_000_000 || markers[len(markers)-1].Date > 1_000_000_000_000 {
		scale = 1000.0
	}
	dtSec := func(prev, curr int64) float64 {
		if curr <= prev {
			return 0
		}
		return float64(curr-prev) / scale
	}
	const maxSpeed = 1000.0
	for i := 1; i < len(markers); i++ {
		dt := dtSec(markers[i-1].Date, markers[i].Date)
		if dt <= 0 {
			continue
		}
		dist := haversineDistance(markers[i-1].Lat, markers[i-1].Lon, markers[i].Lat, markers[i].Lon)
		v := dist / dt
		if v >= 0 && v <= maxSpeed {
			markers[i].Speed = v
		} else {
			markers[i].Speed = 0
		}
	}
	if len(markers) > 1 && markers[0].Speed == 0 {
		markers[0].Speed = markers[1].Speed
	}
	lastWith := -1
	for i := 0; i < len(markers); {
		if markers[i].Speed > 0 {
			lastWith = i
			i++
			continue
		}
		start := i
		for i < len(markers) && markers[i].Speed == 0 {
			i++
		}
		end := i - 1
		nextWith := -1
		if i < len(markers) && markers[i].Speed > 0 {
			nextWith = i
		}
		var fill float64
		switch {
		case lastWith != -1 && nextWith != -1:
			dt := dtSec(markers[lastWith].Date, markers[nextWith].Date)
			if dt > 0 {
				dist := haversineDistance(markers[lastWith].Lat, markers[lastWith].Lon, markers[nextWith].Lat, markers[nextWith].Lon)
				fill = dist / dt
			}
		case lastWith != -1:
			fill = markers[lastWith].Speed
		case nextWith != -1:
			fill = markers[nextWith].Speed
		}
		if fill > 0 && fill <= maxSpeed {
			for j := start; j <= end; j++ {
				markers[j].Speed = fill
			}
		}
	}
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
			dist := haversineDistance(markers[0].Lat, markers[0].Lon, markers[len(markers)-1].Lat, markers[len(markers)-1].Lon)
			v := dist / totalDt
			if v >= 0 && v <= 1000.0 {
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

func sortByDate(markers []database.Marker) {
	// simple insertion sort (N is typically moderate here)
	for i := 1; i < len(markers); i++ {
		j := i
		for j > 0 && markers[j-1].Date > markers[j].Date {
			markers[j-1], markers[j] = markers[j], markers[j-1]
			j--
		}
	}
}

func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	phi1, phi2 := lat1*math.Pi/180, lat2*math.Pi/180
	dPhi, dLambda := (lat2-lat1)*math.Pi/180, (lon2-lon1)*math.Pi/180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * R * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func latLonToWebMercator(lat, lon float64) (x, y float64) {
	const originShift = 2.0 * math.Pi * 6378137.0 / 2.0
	x = lon * originShift / 180.0
	y = math.Log(math.Tan((90.0+lat)*math.Pi/360.0)) / (math.Pi / 180.0)
	y = y * originShift / 180.0
	return x, y
}

func webMercatorToPixel(x, y float64, zoom int) (px, py float64) {
	scale := math.Exp2(float64(zoom))
	px = (x + 2.0*math.Pi*6378137.0/2.0) / (2.0 * math.Pi * 6378137.0) * 256.0 * scale
	py = (2.0*math.Pi*6378137.0/2.0 - y) / (2.0 * math.Pi * 6378137.0) * 256.0 * scale
	return
}

func latLonToPixel(lat, lon float64, zoom int) (px, py float64) {
	x, y := latLonToWebMercator(lat, lon)
	return webMercatorToPixel(x, y, zoom)
}

func radiusForZoom(zoom int) float64 { return 10.0 * float64(zoom) / 20.0 }

func precomputeMarkersForAllZoomLevels(src []database.Marker) []database.Marker {
	type acc struct {
		sumLat, sumLon, sumDose, sumCnt, sumSp float64
		latest                                 int64
		n                                      int
	}
	cellFor := func(z int) float64 { return 2*radiusForZoom(z) + 1 }
	var res []database.Marker
	for z := 1; z <= 20; z++ {
		cl := make(map[int64]*acc)
		cell := cellFor(z)
		for _, m := range src {
			px, py := latLonToPixel(m.Lat, m.Lon, z)
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
		for _, c := range cl {
			n := float64(c.n)
			res = append(res, database.Marker{Lat: c.sumLat / n, Lon: c.sumLon / n, DoseRate: c.sumDose / n, CountRate: c.sumCnt / n, Speed: c.sumSp / n, Date: c.latest, Zoom: z, TrackID: src[0].TrackID})
		}
	}
	return res
}

// ---- HTTP handlers ----

func uploadDiagHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "multipart parse error", http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		http.Error(w, "no files selected", http.StatusBadRequest)
		return
	}

	results := make([]diagResult, 0, len(files))
	okCount := 0
	for _, fh := range files {
		res := diagResult{File: fh.Filename}
		f, _ := fh.Open()
		defer f.Close()

		switch strings.ToLower(filepath.Ext(fh.Filename)) {
		case ".log", ".txt":
			bbox, trackURL, stats, err := handleBGeigieDiag(fh.Filename, f)
			res.Format = "bgeigie_bnrdd"
			res.Stats = stats
			if err != nil {
				res.Status = "error"
				res.Error = err.Error()
			} else {
				res.Status = "ok"
				res.Bounds = &bbox
				res.TrackURL = trackURL
				okCount++
			}
		default:
			res.Status = "skipped"
			res.Format = "unknown_or_not_supported_here"
		}
		results = append(results, res)
	}

	resp := diagResponse{Status: "mixed", Results: results}
	if okCount == len(results) {
		resp.Status = "ok"
	} else if okCount == 0 {
		resp.Status = "error"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleBGeigieDiag(filename string, file multipart.File) (database.Bounds, string, *diagStats, error) {
	stats := &diagStats{Records: 0, Parsed: 0, Reasons: map[string]int{}, Samples: map[string][]string{}}
	trackID := GenerateSerialNumber()

	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	markers := make([]database.Marker, 0, 4096)
	bbox := database.Bounds{MinLat: 90, MinLon: 180, MaxLat: -90, MaxLon: -180}

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		stats.Records++
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "$BNRDD") {
			incReason(stats, "wrong_sentence", line)
			continue
		}
		if i := strings.IndexByte(line, '*'); i != -1 {
			line = line[:i]
		}
		parts := strings.Split(line, ",")
		if len(parts) < 6 {
			incReason(stats, "too_few_fields", line)
			continue
		}

		var (
			ts  int64
			cps float64
			cpm float64
			lat float64
			lon float64
		)

		// Zen ISO8601 + DMM variant: 3:CPM 4:CPS 5:TotalCounts.
		// We favour CPM for stability so the derived µSv/h values do not spike when CPS fluctuates.
		if len(parts) >= 11 && strings.Contains(parts[2], "T") {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[2])); err == nil {
				ts = t.Unix()
			}
			if ts == 0 {
				incReason(stats, "invalid_timestamp", line)
				continue
			}
			cpm = bgnParseFloat(parts[3])
			cps = bgnParseFloat(parts[4])
			lat = bgnParseDMM(parts[7], parts[8], 2)
			lon = bgnParseDMM(parts[9], parts[10], 3)
			if lat == 0 && lon == 0 {
				incReason(stats, "invalid_coords", line)
				continue
			}
		} else {
			// Legacy: decimals or suffixed
			if len(parts) < 8 {
				incReason(stats, "too_few_fields", line)
				continue
			}
			utcDate := parts[1]
			utcTime := parts[2]
			ts = bgnParseDateTime(utcDate, utcTime)
			if ts == 0 {
				incReason(stats, "invalid_timestamp", line)
				continue
			}
			cpm = bgnParseFloat(parts[3])
			cps = bgnParseFloat(parts[4])
			lat = bgnParseCoord(parts[6])
			lon = bgnParseCoord(parts[7])
			if lat == 0 && lon == 0 {
				incReason(stats, "invalid_coords", line)
				continue
			}
		}

		const cpmPerMicroSv = 334.0
		dose := 0.0
		if cpm > 0 {
			dose = cpm / cpmPerMicroSv
		} else if cps > 0 {
			dose = (cps * 60.0) / cpmPerMicroSv
		} else {
			incReason(stats, "missing_cpm_cps", line)
			continue
		}

		countRate := cps
		if countRate == 0 && cpm > 0 {
			countRate = cpm / 60.0
		}

		m := database.Marker{DoseRate: dose, Date: ts, Lon: lon, Lat: lat, CountRate: countRate, Zoom: 0, Speed: 0, TrackID: trackID}
		markers = append(markers, m)
		stats.Parsed++

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
	if err := sc.Err(); err != nil {
		return bbox, "", stats, err
	}

	if len(markers) == 0 {
		return bbox, "", stats, fmt.Errorf("no valid $BNRDD points in %s", filename)
	}

	storedBBox, tid, err := processAndStoreMarkers(markers, trackID, db, *dbType)
	if err != nil {
		return bbox, "", stats, err
	}
	bbox = storedBBox

	trackURL := fmt.Sprintf("/trackid/%s?minLat=%f&minLon=%f&maxLat=%f&maxLon=%f&zoom=14&layer=%s", tid, bbox.MinLat, bbox.MinLon, bbox.MaxLat, bbox.MaxLon, "OpenStreetMap")

	logT(tid, "BGEIGIE", "file=%s parsed=%d skipped=%d reasons=%v", filename, stats.Parsed, stats.Records-stats.Parsed, stats.Reasons)
	return bbox, trackURL, stats, nil
}

func incReason(st *diagStats, reason, line string) {
	st.Skipped++
	st.Reasons[reason]++
	lst := st.Samples[reason]
	if len(lst) < 3 {
		if len(line) > 180 {
			line = line[:180]
		}
		st.Samples[reason] = append(lst, line)
	}
}

// ---- main entry: init DB, schema, start server ----
func main() {
	flag.Parse()

	cfg := database.Config{
		DBType:      *dbType,
		DBPath:      *dbPath,
		DBHost:      *dbHost,
		DBPort:      *dbPort,
		DBUser:      *dbUser,
		DBPass:      *dbPass,
		DBName:      *dbName,
		PGSSLMode:   *pgSSL,
		ClickSecure: *clickhouseTLS,
		Port:        *port,
	}

	var err error
	db, err = database.NewDatabase(cfg)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	if err := db.InitSchema(cfg); err != nil {
		log.Fatalf("init schema: %v", err)
	}
	// build helpful indexes in background (non-blocking)
	db.EnsureIndexesAsync(context.Background(), cfg, func(format string, v ...any) {
		log.Printf(format, v...)
	})

	http.HandleFunc("/upload_diag", uploadDiagHandler)
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Diagnostics server listening on %s (/upload_diag)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
