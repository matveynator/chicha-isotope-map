package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// Minimal diagnostics for bGeigie ($BNRDD) uploads.
// Adds a separate endpoint /upload_diag that returns per-file JSON stats
// and logs a short summary to the terminal. Existing /upload stays unchanged.

// diagStats holds per-file parsing diagnostics.
type diagStats struct {
	Records int               `json:"records"`
	Parsed  int               `json:"parsed"`
	Skipped int               `json:"skipped"`
	Reasons map[string]int    `json:"reasons"`
	Samples map[string][]string `json:"samples"`
}

// bgnParseDMM parses degrees+minutes (DDMM.MMMM or DDDMM.MMMM) with hemisphere.
// Example: lat "3428.9629", hemi "N"  -> 34 + 28.9629/60
//          lon "13609.7691", hemi "E" -> 136 + 09.7691/60
// degDigits: 2 for latitude, 3 for longitude (used for validation only).
func bgnParseDMM(val, hemi string, degDigits int) float64 {
    val = strings.TrimSpace(val)
    hemi = strings.TrimSpace(hemi)
    if val == "" { return 0 }
    f, err := strconv.ParseFloat(val, 64)
    if err != nil { return 0 }
    deg := int(f / 100.0)
    minutes := f - float64(deg*100)
    d := float64(deg) + minutes/60.0
    switch strings.ToUpper(hemi) {
    case "S", "W":
        d = -d
    }
    // simple range validation
    if degDigits == 2 { // latitude
        if d < -90 || d > 90 { return 0 }
    } else { // longitude
        if d < -180 || d > 180 { return 0 }
    }
    return d
}

type diagResult struct {
	File    string           `json:"file"`
	Format  string           `json:"formatDetected"`
	Status  string           `json:"status"` // ok | error | skipped
	Error   string           `json:"error,omitempty"`
	TrackURL string          `json:"trackURL,omitempty"`
	Bounds  *database.Bounds `json:"bounds,omitempty"`
	Stats   *diagStats       `json:"stats,omitempty"`
}

type diagResponse struct {
	Status  string       `json:"status"` // ok | mixed | error
	Results []diagResult `json:"results"`
}

func init() {
	// Register diagnostics endpoint without touching main route setup
	http.HandleFunc("/upload_diag", uploadDiagHandler)
}

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

		// Detect Zen ISO8601 + DMM variant: parts[2] like 2025-07-26T06:05:14Z
		if len(parts) >= 11 && strings.Contains(parts[2], "T") {
			// indices: 0:$BNRDD 1:ver 2:ISO8601 3:CPS 4:CPM 5:CPMvalid 6:fix(V/A) 7:LATdmm 8:N/S 9:LONdmm 10:E/W ...
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[2])); err == nil {
				ts = t.Unix()
			}
			if ts == 0 {
				incReason(stats, "invalid_timestamp", line)
				continue
			}
			cps = bgnParseFloat(parts[3])
			cpm = bgnParseFloat(parts[4])
			lat = bgnParseDMM(parts[7], parts[8], 2)
			lon = bgnParseDMM(parts[9], parts[10], 3)
			if lat == 0 && lon == 0 {
				incReason(stats, "invalid_coords", line)
				continue
			}
		} else {
			// Legacy variant: parts[1]=UTCdate, parts[2]=UTCtime, parts[6]=LAT, parts[7]=LON (decimals or suffixed)
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
			cps = bgnParseFloat(parts[3])
			cpm = bgnParseFloat(parts[4])
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

		m := database.Marker{
			DoseRate:  dose,
			Date:      ts,
			Lon:       lon,
			Lat:       lat,
			CountRate: countRate,
			Zoom:      0,
			Speed:     0,
			TrackID:   trackID,
		}
		markers = append(markers, m)
		stats.Parsed++

		if m.Lat < bbox.MinLat { bbox.MinLat = m.Lat }
		if m.Lat > bbox.MaxLat { bbox.MaxLat = m.Lat }
		if m.Lon < bbox.MinLon { bbox.MinLon = m.Lon }
		if m.Lon > bbox.MaxLon { bbox.MaxLon = m.Lon }
	}
	if err := sc.Err(); err != nil {
		return bbox, "", stats, err
	}

	if len(markers) == 0 {
		return bbox, "", stats, fmt.Errorf("no valid $BNRDD points in %s", filename)
	}

	// store via common pipeline (reuses identity-probe & zoom aggregates)
	storedBBox, tid, err := processAndStoreMarkers(markers, trackID, db, *dbType)
	if err != nil {
		return bbox, "", stats, err
	}
	bbox = storedBBox

	trackURL := fmt.Sprintf(
		"/trackid/%s?minLat=%f&minLon=%f&maxLat=%f&maxLon=%f&zoom=14&layer=%s",
		tid, bbox.MinLat, bbox.MinLon, bbox.MaxLat, bbox.MaxLon, "OpenStreetMap",
	)

	// Log concise summary to terminal
	logT(tid, "BGEIGIE", "file=%s parsed=%d skipped=%d reasons=%v",
		filename, stats.Parsed, stats.Records-stats.Parsed, stats.Reasons)
	log.Printf("[%-6s][BGEIGIE] sample invalid lines: %v", tid, samplePreview(stats.Samples))

	return bbox, trackURL, stats, nil
}

func incReason(st *diagStats, reason, line string) {
	st.Skipped++
	st.Reasons[reason]++
	// Keep up to 3 samples per reason
	lst := st.Samples[reason]
	if len(lst) < 3 {
		if len(line) > 180 {
			line = line[:180]
		}
		st.Samples[reason] = append(lst, line)
	}
}

func samplePreview(m map[string][]string) map[string][]string {
	// ensure we don’t flood logs; return as-is (already capped)
	return m
}

func bgnParseFloat(s string) float64 { v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return v }

// bgnParseCoord parses numbers with optional N/S/E/W suffix
func bgnParseCoord(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" { return 0 }
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
