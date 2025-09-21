package jsonarchive

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
	"chicha-isotope-map/pkg/trackjson"
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

// Generator continuously maintains a tar.gz bundle of JSON .cim exports for all
// tracks stored in the database. Synchronisation happens via channels so we rely
// on message passing instead of mutexes.
type Generator struct {
	requests chan chan result
	done     chan struct{}
}

type result struct {
	info Info
	err  error
}

// Start launches the background worker.
// The worker exports every known track into temporary .cim JSON files, packs
// them into a tar.gz archive, and atomically replaces the destination file once
// the build succeeds. The initial build happens in the background so startup
// remains responsive even on large databases while HTTP handlers can still
// block until the first snapshot is ready if they need the data immediately.
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
						logf("json archive rebuild failed: %v", res.err)
					} else {
						logf("json archive ready: %s", res.info.Path)
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

	// Kick off the first build asynchronously so the main goroutine keeps
	// starting up quickly even on very large databases. We still log the
	// scheduling step so operators understand why Fetch may briefly wait.
	triggerBuild()
	if logf != nil {
		logf("json archive initial build scheduled: %s", destPath)
	}

	// Coordinator goroutine multiplexes ticker events and HTTP requests.
	go func() {
		defer close(done)

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		current := result{}
		haveResult := false

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				triggerBuild()
			case res := <-buildResults:
				current = res
				haveResult = true
			case ch := <-requests:
				if !haveResult || (current.info.Path == "" && current.err == nil) || current.err != nil {
					triggerBuild()
					select {
					case <-ctx.Done():
						ch <- result{err: ctx.Err()}
						close(ch)
						return
					case res := <-buildResults:
						current = res
						haveResult = true
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
// a temporary JSON file, and writes them into a tar.gz bundle. We only replace
// the destination after the build succeeds so clients never observe a partial
// archive.
func buildArchive(ctx context.Context, db *database.Database, dbType, destPath string) (string, time.Time, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", time.Time{}, fmt.Errorf("create archive directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), "json-*.tar.gz")
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
		summaries := make([]database.TrackSummary, 0, pageSize)
		var lastID string

		for summary := range summariesCh {
			if err := ctx.Err(); err != nil {
				cancel()
				tarw.Close()
				gz.Close()
				cleanup()
				return "", time.Time{}, err
			}
			lastID = summary.TrackID
			summaries = append(summaries, summary)
		}

		if err := <-errCh; err != nil {
			cancel()
			tarw.Close()
			gz.Close()
			cleanup()
			return "", time.Time{}, err
		}

		// Process the page only after the summary query closes so SQLite releases
		// its connection before we begin streaming markers. Without this staging
		// step the marker query would block waiting for the still-open summary
		// cursor, leading to the deadlock observed at startup.
		for _, summary := range summaries {
			if err := appendTrack(buildCtx, tarw, db, dbType, summary); err != nil {
				cancel()
				tarw.Close()
				gz.Close()
				cleanup()
				return "", time.Time{}, err
			}
		}

		if len(summaries) < pageSize || lastID == "" {
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

// appendTrack exports a single track into a temporary .cim JSON file and then
// copies it into the tar writer. We stream markers so archives stay memory
// efficient even for very long tracks.
func appendTrack(ctx context.Context, tw *tar.Writer, db *database.Database, dbType string, summary database.TrackSummary) error {
	if strings.TrimSpace(summary.TrackID) == "" || summary.MarkerCount == 0 {
		return nil
	}

	trackIndex, err := db.CountTrackIDsUpTo(ctx, summary.TrackID, dbType)
	if err != nil {
		return fmt.Errorf("track %s index: %w", summary.TrackID, err)
	}

	tmp, err := os.CreateTemp("", "track-*.cim")
	if err != nil {
		return fmt.Errorf("tmp track %s: %w", summary.TrackID, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	writer := bufio.NewWriter(tmp)
	latest, err := writeTrackJSON(ctx, db, dbType, summary, trackIndex, writer)
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

// writeTrackJSON streams markers into the JSON payload and returns the latest
// timestamp so the tar entry can mirror the freshest measurement.
func writeTrackJSON(
	ctx context.Context,
	db *database.Database,
	dbType string,
	summary database.TrackSummary,
	trackIndex int64,
	w *bufio.Writer,
) (time.Time, error) {
	if _, err := fmt.Fprintf(w, "{\n"); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "  \"trackID\": %q,\n", summary.TrackID); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "  \"trackIndex\": %d,\n", trackIndex); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "  \"apiURL\": %q,\n", trackjson.TrackAPIPath(summary.TrackID)); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "  \"firstID\": %d,\n", summary.FirstID); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "  \"lastID\": %d,\n", summary.LastID); err != nil {
		return time.Time{}, err
	}
	if _, err := fmt.Fprintf(w, "  \"markerCount\": %d,\n", summary.MarkerCount); err != nil {
		return time.Time{}, err
	}

	if _, err := w.WriteString("  \"markers\": ["); err != nil {
		return time.Time{}, err
	}

	latest := time.Time{}
	first := true
	handler := func(marker database.Marker) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		payload, when := trackjson.MakeMarkerPayload(marker)
		if when.After(latest) {
			latest = when
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal marker %d: %w", marker.ID, err)
		}

		prefix := "\n    "
		if !first {
			prefix = ",\n    "
		}
		if _, err := w.WriteString(prefix); err != nil {
			return err
		}
		if _, err := w.Write(encoded); err != nil {
			return err
		}

		first = false
		return nil
	}

	if err := streamTrackMarkers(ctx, db, dbType, summary, handler); err != nil {
		return time.Time{}, err
	}

	if first {
		if _, err := w.WriteString("],\n"); err != nil {
			return time.Time{}, err
		}
	} else {
		if _, err := w.WriteString("\n  ],\n"); err != nil {
			return time.Time{}, err
		}
	}

	disclaimers, err := json.Marshal(trackjson.Disclaimers)
	if err != nil {
		return time.Time{}, fmt.Errorf("marshal disclaimers: %w", err)
	}
	if _, err := fmt.Fprintf(w, "  \"disclaimers\": %s\n}", disclaimers); err != nil {
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
	base := trackjson.SafeCIMFilename(summary.TrackID)
	name := strings.TrimSuffix(base, ".cim")
	if name == "" {
		name = "track"
	}
	return fmt.Sprintf("%s-%d.cim", name, summary.FirstID)
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
