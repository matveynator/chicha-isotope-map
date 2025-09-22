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

// progressUpdate summarises archive build status for the logging goroutine.
// Using a struct keeps the channel communication explicit and avoids mutexes,
// echoing the proverb "Don't communicate by sharing memory; share memory by
// communicating".
type progressUpdate struct {
	totalTracks     int64
	processedTracks int64
	bytesWritten    int64
	currentTrack    string
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
	normalisedPath, err := normaliseArchiveDestination(destPath)
	if err != nil && logf != nil {
		logf("json archive destination normalisation warning for %q: %v", destPath, err)
	}
	if strings.TrimSpace(normalisedPath) != "" {
		destPath = normalisedPath
	}
	archiveDir := filepath.Dir(destPath)
	if err := purgeStaleArchiveTemps(archiveDir, destPath, logf); err != nil && logf != nil {
		logf("json archive stale temp cleanup warning: %v", err)
	}
	if logf != nil {
		logf("json archive writer targeting %s (work dir %s)", destPath, archiveDir)
	}

	// If a previous run already produced an archive we can reuse it immediately
	// so HTTP handlers keep responding without waiting for the fresh build to
	// finish. This keeps startup "Clear is better than clever" by serving the
	// existing snapshot while the background worker prepares the next revision.
	// Capture any existing archive so Fetch callers can immediately serve it
	// while the background build runs. This keeps the system responsive even
	// before the first refresh completes.
	var initialResult result
	haveInitial := false
	if info, err := os.Stat(destPath); err == nil && info.Mode().IsRegular() {
		initialResult = result{info: Info{Path: destPath, ModTime: info.ModTime()}}
		haveInitial = true
		if logf != nil {
			logf("json archive previous snapshot found: %s", destPath)
		}
	}

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
				res := runBuild(ctx, db, dbType, destPath, logf)
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

		current := initialResult
		haveResult := haveInitial

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
func runBuild(ctx context.Context, db *database.Database, dbType, destPath string, logf func(string, ...any)) result {
	path, modTime, err := buildArchive(ctx, db, dbType, destPath, logf)
	if err != nil {
		return result{err: err}
	}
	return result{info: Info{Path: path, ModTime: modTime}}
}

// buildArchive streams track summaries from the database, exports each track to
// a temporary JSON file, and writes them into a tar.gz bundle. We only replace
// the destination after the build succeeds so clients never observe a partial
// archive.
func buildArchive(ctx context.Context, db *database.Database, dbType, destPath string, logf func(string, ...any)) (string, time.Time, error) {
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", time.Time{}, fmt.Errorf("create archive directory: %w", err)
	}

	tmpTrackDir := filepath.Join(destDir, ".json-archive-tracks")
	if err := os.MkdirAll(tmpTrackDir, 0o755); err != nil {
		return "", time.Time{}, fmt.Errorf("create track temp dir: %w", err)
	}
	if logf != nil {
		logf("json archive build starting: target=%s temp=%s", destPath, tmpTrackDir)
	}
	if err := purgeStaleTrackFiles(tmpTrackDir, logf); err != nil {
		return "", time.Time{}, err
	}

	tmpFile, err := os.CreateTemp(destDir, "json-*.tar.gz")
	if err != nil {
		return "", time.Time{}, fmt.Errorf("tmp archive: %w", err)
	}

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}

	counter := &countingWriter{Writer: tmpFile}
	gz := gzip.NewWriter(counter)
	tarw := tar.NewWriter(gz)

	totalTracks, err := db.CountTracks(ctx)
	if err != nil {
		tarw.Close()
		gz.Close()
		cleanup()
		return "", time.Time{}, fmt.Errorf("count tracks: %w", err)
	}

	updates := make(chan progressUpdate, 64)
	progressWait := func() {}
	if logf != nil {
		progressWait = startProgressLogger(ctx, logf, destPath, updates)
	}
	defer func() {
		close(updates)
		progressWait()
	}()

	sendProgress := func(processed int64, current string) {
		select {
		case updates <- progressUpdate{totalTracks: totalTracks, processedTracks: processed, bytesWritten: counter.Bytes(), currentTrack: current}:
		default:
		}
	}
	sendProgress(0, "")

	pageSize := 256
	startAfter := ""
	processed := int64(0)
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
			if err := appendTrack(buildCtx, tarw, db, dbType, summary, tmpTrackDir); err != nil {
				cancel()
				tarw.Close()
				gz.Close()
				cleanup()
				return "", time.Time{}, err
			}
			processed++
			sendProgress(processed, summary.TrackID)
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

	sendProgress(processed, "finalising")

	if err := replaceFile(tmpFile.Name(), destPath); err != nil {
		cleanup()
		return "", time.Time{}, err
	}

	info, err := os.Stat(destPath)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("stat archive: %w", err)
	}

	sendProgress(processed, "complete")

	return destPath, info.ModTime(), nil
}

// appendTrack exports a single track into a temporary .cim JSON file and then
// copies it into the tar writer. We stream markers so archives stay memory
// efficient even for very long tracks.
func appendTrack(ctx context.Context, tw *tar.Writer, db *database.Database, dbType string, summary database.TrackSummary, tmpDir string) error {
	if strings.TrimSpace(summary.TrackID) == "" || summary.MarkerCount == 0 {
		return nil
	}

	trackIndex := summary.Index
	if trackIndex <= 0 {
		// Older databases may not supply the index yet. We keep the fallback
		// to avoid breaking incremental migrations but expect the new streaming
		// path to populate it so PostgreSQL is not hammered during archive builds.
		var err error
		trackIndex, err = db.CountTrackIDsUpTo(ctx, summary.TrackID, dbType)
		if err != nil {
			return fmt.Errorf("track %s index: %w", summary.TrackID, err)
		}
	}

	tmp, err := os.CreateTemp(tmpDir, "track-*.cim")
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

// purgeStaleArchiveTemps removes leftover json-*.tar.gz files that remain when
// the process crashes mid-build. Cleaning these early keeps restarts tidy and
// follows "Simplicity is complicated" by preferring a deterministic reset over
// attempting to resume partially written tarballs.
func purgeStaleArchiveTemps(dir, destPath string, logf func(string, ...any)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("scan archive dir: %w", err)
	}

	destBase := filepath.Base(destPath)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == destBase {
			continue
		}
		if !strings.HasPrefix(name, "json-") || !strings.HasSuffix(name, ".tar.gz") {
			continue
		}
		full := filepath.Join(dir, name)
		if removeErr := os.Remove(full); removeErr != nil {
			if os.IsNotExist(removeErr) {
				continue
			}
			if logf != nil {
				logf("json archive temp removal warning: %s: %v", full, removeErr)
			}
		} else if logf != nil {
			logf("json archive removed stale temp: %s", full)
		}
	}
	return nil
}

// normaliseArchiveDestination ensures we have a concrete file path even when the
// operator supplied a directory. This avoids surprising os.Rename failures and
// keeps configuration simple and explicit.
func normaliseArchiveDestination(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return cleaned, nil
	}

	info, err := os.Stat(cleaned)
	switch {
	case err == nil:
		if info.IsDir() {
			return filepath.Join(cleaned, "daily-json.tar.gz"), nil
		}
		return cleaned, nil
	case os.IsNotExist(err):
		if strings.HasSuffix(path, string(os.PathSeparator)) {
			dir := filepath.Clean(path)
			return filepath.Join(dir, "daily-json.tar.gz"), nil
		}
		return cleaned, nil
	default:
		return cleaned, err
	}
}

// purgeStaleTrackFiles removes leftover per-track temp files so fresh builds do
// not slowly consume disk space if the process crashed mid-run previously. The
// helper leans on logging instead of failing hard, following "A little copying is
// better than a little dependency" by avoiding extra cleanup packages.
func purgeStaleTrackFiles(dir string, logf func(string, ...any)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("scan track temp dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "track-") || !strings.HasSuffix(name, ".cim") {
			continue
		}
		full := filepath.Join(dir, name)
		if removeErr := os.Remove(full); removeErr != nil && logf != nil {
			logf("json archive temp removal warning: %s: %v", full, removeErr)
		}
	}
	return nil
}

// startProgressLogger spins a goroutine that periodically prints build
// milestones. Returning a wait function lets the caller coordinate shutdown via
// channels without resorting to mutexes.
func startProgressLogger(ctx context.Context, logf func(string, ...any), destPath string, updates <-chan progressUpdate) func() {
	if logf == nil {
		return func() {}
	}
	done := make(chan struct{})
	go logProgress(ctx, logf, destPath, updates, done)
	return func() { <-done }
}

// logProgress aggregates updates and emits human friendly progress messages.
// A ticker throttles output so huge archives do not flood logs while still
// offering operators visibility into long-running builds.
func logProgress(
	ctx context.Context,
	logf func(string, ...any),
	destPath string,
	updates <-chan progressUpdate,
	done chan<- struct{},
) {
	defer close(done)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var last progressUpdate
	haveUpdate := false

	emit := func(prefix string) {
		if !haveUpdate {
			return
		}
		percent := computePercent(last.processedTracks, last.totalTracks)
		current := last.currentTrack
		if strings.TrimSpace(current) == "" {
			current = "startup"
		}
		logf("json archive %s: %.1f%% (%d/%d tracks) %s written to %s (current=%s)",
			prefix,
			percent,
			last.processedTracks,
			last.totalTracks,
			formatBytes(last.bytesWritten),
			destPath,
			current,
		)
	}

	for {
		select {
		case <-ctx.Done():
			emit("cancelled")
			return
		case update, ok := <-updates:
			if !ok {
				emit("complete")
				return
			}
			last = update
			haveUpdate = true
		case <-ticker.C:
			emit("progress")
		}
	}
}

// computePercent returns a percentage suitable for logging, guarding against
// division by zero so empty databases still report useful output.
func computePercent(done, total int64) float64 {
	if total <= 0 {
		if done <= 0 {
			return 100.0
		}
		return 100.0
	}
	return float64(done) / float64(total) * 100.0
}

// countingWriter wraps another writer and tracks how many bytes went through.
// This keeps progress reporting cheap and avoids querying the filesystem on each
// update.
type countingWriter struct {
	io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.Writer.Write(p)
	cw.n += int64(n)
	return n, err
}

// Bytes reports how many bytes were written so far.
func (cw *countingWriter) Bytes() int64 {
	return cw.n
}

// formatBytes pretty-prints byte counts using binary units, keeping log output
// digestible even for 300+ GiB archives.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(n)
	for _, suffix := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1fEiB", value/unit)
}
