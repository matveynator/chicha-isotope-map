package kmlarchive

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// ========================
// Archive generation logic
// ========================

// Info describes the current archive snapshot. We expose the on-disk path so
// HTTP handlers can stream straight from disk without buffering the entire
// tarball in memory, following the Go proverb "Share memory by communicating".
type Info struct {
	Path    string
	ModTime time.Time
}

// Generator continuously maintains a tar.gz bundle of KML exports for all
// tracks stored in the database. Synchronisation happens via channels so we
// rely on message passing instead of mutexes.
type Generator struct {
	requests chan chan result
	done     chan struct{}
}

type result struct {
	info Info
	err  error
}

// Start launches the background worker.
// The worker exports every known track into temporary KML files, packs them
// into a tar.gz archive, and atomically replaces the destination file once the
// build succeeds. We trigger the initial build synchronously so the file exists
// before the HTTP layer starts serving requests, matching the user's request to
// have a fresh archive on startup.
func Start(
	ctx context.Context,
	db *database.Database,
	dbType string,
	destPath string,
	refreshInterval time.Duration,
	logf func(string, ...any),
) *Generator {
	requests := make(chan chan result)
	done := make(chan struct{})
	buildRequests := make(chan struct{}, 1)
	buildResults := make(chan result, 1)

	destPath = filepath.Clean(destPath)

	triggerBuild := func() {
		select {
		case buildRequests <- struct{}{}:
		default:
		}
	}

	// Builder goroutine keeps disk IO and database work away from the main
	// coordination loop so Fetch calls stay responsive.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-buildRequests:
				res := runBuild(ctx, db, dbType, destPath)
				if logf != nil {
					if res.err != nil {
						logf("kml archive rebuild failed: %v", res.err)
					} else {
						logf("kml archive ready: %s", res.info.Path)
					}
				}
				select {
				case <-ctx.Done():
					return
				case buildResults <- res:
				}
			}
		}
	}()

	// Synchronous build on startup so consumers immediately see a file.
	initial := runBuild(ctx, db, dbType, destPath)
	if logf != nil {
		if initial.err != nil {
			logf("kml archive initial build failed: %v", initial.err)
		} else {
			logf("kml archive initialised: %s", initial.info.Path)
		}
	}

	// Coordinator goroutine multiplexes ticker events and HTTP requests.
	go func() {
		defer close(done)

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		current := initial

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				triggerBuild()
			case res := <-buildResults:
				current = res
			case ch := <-requests:
				if (current.info.Path == "" && current.err == nil) || current.err != nil {
					triggerBuild()
					select {
					case <-ctx.Done():
						ch <- result{err: ctx.Err()}
						close(ch)
						return
					case res := <-buildResults:
						current = res
					}
				}
				ch <- current
				close(ch)
			}
		}
	}()

	return &Generator{requests: requests, done: done}
}

// Fetch returns the current archive info, building it on-demand if necessary.
func (g *Generator) Fetch(ctx context.Context) (Info, error) {
	respCh := make(chan result, 1)

	select {
	case <-ctx.Done():
		return Info{}, ctx.Err()
	case <-g.done:
		return Info{}, fmt.Errorf("archive generator stopped")
	case g.requests <- respCh:
	}

	select {
	case <-ctx.Done():
		return Info{}, ctx.Err()
	case <-g.done:
		return Info{}, fmt.Errorf("archive generator stopped")
	case res := <-respCh:
		return res.info, res.err
	}
}

// ============================
// Archive build implementation
// ============================

// runBuild wraps buildArchive so the builder goroutine stays small.
func runBuild(ctx context.Context, db *database.Database, dbType, destPath string) result {
	path, modTime, err := buildArchive(ctx, db, dbType, destPath)
	if err != nil {
		return result{err: err}
	}
	return result{info: Info{Path: path, ModTime: modTime}}
}

// buildArchive streams track summaries from the database, exports each track to
// a temporary KML file, and writes them into a tar.gz bundle. We only replace
// the destination after the build succeeds so clients never observe a partial
// archive.
func buildArchive(ctx context.Context, db *database.Database, dbType, destPath string) (string, time.Time, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", time.Time{}, fmt.Errorf("create archive directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), "kml-*.tar.gz")
	if err != nil {
		return "", time.Time{}, fmt.Errorf("tmp archive: %w", err)
	}

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}

	gz := gzip.NewWriter(tmpFile)
	tarw := tar.NewWriter(gz)

	pageSize := 256
	startAfter := ""
	buildCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			tarw.Close()
			gz.Close()
			cleanup()
			return "", time.Time{}, ctx.Err()
		default:
		}

		summariesCh, errCh := db.StreamTrackSummaries(buildCtx, startAfter, pageSize, dbType)
		fetched := 0
		var lastID string

		for summary := range summariesCh {
			fetched++
			lastID = summary.TrackID
			if err := appendTrack(buildCtx, tarw, db, dbType, summary); err != nil {
				cancel()
				tarw.Close()
				gz.Close()
				cleanup()
				return "", time.Time{}, err
			}
		}

		if err := <-errCh; err != nil {
			cancel()
			tarw.Close()
			gz.Close()
			cleanup()
			return "", time.Time{}, err
		}

		if fetched < pageSize || lastID == "" {
			break
		}
		startAfter = lastID
	}

	if err := tarw.Close(); err != nil {
		gz.Close()
		cleanup()
		return "", time.Time{}, fmt.Errorf("close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		cleanup()
		return "", time.Time{}, fmt.Errorf("close gzip: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return "", time.Time{}, fmt.Errorf("close archive file: %w", err)
	}

	if err := replaceFile(tmpFile.Name(), destPath); err != nil {
		cleanup()
		return "", time.Time{}, err
	}

	info, err := os.Stat(destPath)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("stat archive: %w", err)
	}

	return destPath, info.ModTime(), nil
}

// appendTrack exports a single track into a temporary KML file and appends it
// to the tar writer. We keep the logic streaming to avoid buffering entire
// tracks in memory, leaning on disk as a safety net for huge exports.
func appendTrack(ctx context.Context, tw *tar.Writer, db *database.Database, dbType string, summary database.TrackSummary) error {
	if strings.TrimSpace(summary.TrackID) == "" || summary.MarkerCount == 0 {
		return nil
	}

	tmp, err := os.CreateTemp("", "track-*.kml")
	if err != nil {
		return fmt.Errorf("tmp track %s: %w", summary.TrackID, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	writer := bufio.NewWriter(tmp)
	latest, err := writeTrackKML(ctx, db, dbType, summary, writer)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("write track %s: %w", summary.TrackID, err)
	}
	if err := writer.Flush(); err != nil {
		tmp.Close()
		return fmt.Errorf("flush track %s: %w", summary.TrackID, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close track %s: %w", summary.TrackID, err)
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		return fmt.Errorf("stat track %s: %w", summary.TrackID, err)
	}

	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("open track %s: %w", summary.TrackID, err)
	}

	header := &tar.Header{
		Name: safeTrackFilename(summary),
		Mode: 0o644,
		Size: info.Size(),
	}
	if !latest.IsZero() {
		header.ModTime = latest
	} else {
		header.ModTime = info.ModTime()
	}

	if err := tw.WriteHeader(header); err != nil {
		file.Close()
		return fmt.Errorf("tar header %s: %w", summary.TrackID, err)
	}
	if _, err := io.Copy(tw, file); err != nil {
		file.Close()
		return fmt.Errorf("tar copy %s: %w", summary.TrackID, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close copy %s: %w", summary.TrackID, err)
	}

	return nil
}

// writeTrackKML streams all markers for a track into the writer and returns the
// latest timestamp encountered so callers can use it as the file's modification
// time.
func writeTrackKML(
	ctx context.Context,
	db *database.Database,
	dbType string,
	summary database.TrackSummary,
	w *bufio.Writer,
) (time.Time, error) {
	now := time.Now().UTC()
	if _, err := fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "<kml xmlns=\"http://www.opengis.net/kml/2.2\">\n"); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "  <Document>\n"); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "    <name>%s</name>\n", xmlEscape(summary.TrackID)); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "    <description>Exported %s (%d markers)</description>\n", now.Format(time.RFC3339), summary.MarkerCount); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "    <Folder>\n"); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "      <name>Measurements</name>\n"); err != nil {
		return time.Time{}, err
	}

	latest := time.Time{}
	handler := func(marker database.Marker) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		when := time.Unix(marker.Date, 0).UTC()
		if when.After(latest) {
			latest = when
		}

		if _, err := fmt.Fprintf(w, "      <Placemark>\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "        <name>%s #%d</name>\n", xmlEscape(summary.TrackID), marker.ID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "        <TimeStamp><when>%s</when></TimeStamp>\n", when.Format(time.RFC3339)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "        <ExtendedData>\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "          <Data name=\"doseRate\"><value>%.6f</value></Data>\n", marker.DoseRate); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "          <Data name=\"countRate\"><value>%.6f</value></Data>\n", marker.CountRate); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "          <Data name=\"speed\"><value>%.6f</value></Data>\n", marker.Speed); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "        </ExtendedData>\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "        <Point><coordinates>%.6f,%.6f,0</coordinates></Point>\n", marker.Lon, marker.Lat); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "      </Placemark>\n"); err != nil {
			return err
		}

		return nil
	}

	if err := streamTrackMarkers(ctx, db, dbType, summary, handler); err != nil {
		return time.Time{}, err
	}

	if _, err := fmt.Fprintf(w, "    </Folder>\n"); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "  </Document>\n"); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "</kml>\n"); err != nil {
		return time.Time{}, err
	}

	return latest, nil
}

// streamTrackMarkers fetches markers in bounded chunks so we avoid loading an
// entire track into memory. The callback receives markers in order and can stop
// the stream by returning an error.
func streamTrackMarkers(
	ctx context.Context,
	db *database.Database,
	dbType string,
	summary database.TrackSummary,
	handler func(database.Marker) error,
) error {
	if summary.MarkerCount == 0 {
		return nil
	}

	chunkSize := 1000
	nextID := summary.FirstID
	for nextID <= summary.LastID {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		markersCh, errCh := db.StreamMarkersByTrackRange(ctx, summary.TrackID, nextID, summary.LastID, chunkSize, dbType)
		got := false
		var lastID int64

		for marker := range markersCh {
			got = true
			lastID = marker.ID
			if err := handler(marker); err != nil {
				// Drain the error channel so the goroutine terminates cleanly.
				<-errCh
				return err
			}
		}

		if err := <-errCh; err != nil {
			return err
		}

		if !got {
			break
		}
		nextID = lastID + 1
	}

	return nil
}

// safeTrackFilename normalises track IDs into archive-safe filenames and
// appends the first marker ID to keep names unique even if sanitisation removes
// characters.
func safeTrackFilename(summary database.TrackSummary) string {
	var b strings.Builder
	for _, r := range summary.TrackID {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "track"
	}
	return fmt.Sprintf("%s-%d.kml", name, summary.FirstID)
}

// xmlEscape escapes a string for XML text nodes.
func xmlEscape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}

// replaceFile atomically replaces the destination with the temporary file.
func replaceFile(tmpPath, destPath string) error {
	if err := os.Rename(tmpPath, destPath); err != nil {
		if removeErr := os.Remove(destPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("remove old archive: %w", removeErr)
		}
		if err := os.Rename(tmpPath, destPath); err != nil {
			return fmt.Errorf("replace archive: %w", err)
		}
	}
	return nil
}
