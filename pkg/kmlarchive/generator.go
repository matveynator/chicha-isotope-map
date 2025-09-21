package kmlarchive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ========================
// Archive generation logic
// ========================

// Info describes the current archive snapshot. We share the path instead of
// bytes so HTTP handlers can stream directly from disk without loading the
// entire tarball into memory.
type Info struct {
	Path    string
	ModTime time.Time
}

// Generator continuously maintains a tar.gz bundle of KML files.
// Synchronisation happens via channels so we respect the requirement of
// communicating by passing messages instead of locking.
type Generator struct {
	requests chan chan result
	done     chan struct{}
}

type result struct {
	info Info
	err  error
}

// Start launches the background worker.
// The worker scans the source directory, packages every .kml file into a
// tar.gz archive, and refreshes it at the requested interval. When the
// context is cancelled we close the done channel so callers know the
// generator has stopped.
func Start(
	ctx context.Context,
	sourceDir string,
	refreshInterval time.Duration,
	logf func(string, ...any),
) *Generator {
	requests := make(chan chan result)
	done := make(chan struct{})
	buildRequests := make(chan struct{}, 1)
	buildResults := make(chan result, 1)

	triggerBuild := func() {
		select {
		case buildRequests <- struct{}{}:
		default:
		}
	}

	// Builder goroutine keeps disk IO away from the main loop.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-buildRequests:
				res := runBuild(ctx, sourceDir)
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

	// Coordinator goroutine multiplexes ticker events and HTTP requests.
	go func() {
		defer close(done)

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		var current result
		triggerBuild()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				triggerBuild()
			case res := <-buildResults:
				current = res
			case ch := <-requests:
				if current.info.Path == "" && current.err == nil {
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

// runBuild wraps buildArchive so the builder goroutine stays small.
func runBuild(ctx context.Context, sourceDir string) result {
	path, modTime, err := buildArchive(ctx, sourceDir)
	if err != nil {
		return result{err: err}
	}
	return result{info: Info{Path: path, ModTime: modTime}}
}

// buildArchive walks the source directory, finds .kml files and writes them
// into a tar.gz bundle. The function streams file contents directly into the
// archive writer to keep memory usage minimal.
func buildArchive(ctx context.Context, sourceDir string) (string, time.Time, error) {
	tmpFile, err := os.CreateTemp("", "kml-*.tar.gz")
	if err != nil {
		return "", time.Time{}, fmt.Errorf("tmp archive: %w", err)
	}

	// We clean up the file on failure so stale blobs do not accumulate.
	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}

	gz := gzip.NewWriter(tmpFile)
	tarw := tar.NewWriter(gz)

	fileCh := make(chan string)
	walkErr := make(chan error, 1)

	if _, statErr := os.Stat(sourceDir); statErr != nil {
		if !os.IsNotExist(statErr) {
			cleanup()
			return "", time.Time{}, fmt.Errorf("scan source: %w", statErr)
		}
		// No KML directory yet: close the channel immediately so the
		// archive becomes an empty placeholder built on demand.
		close(fileCh)
		walkErr <- nil
	} else {
		go func() {
			defer close(fileCh)
			walkErr <- filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				if !strings.HasSuffix(strings.ToLower(d.Name()), ".kml") {
					return nil
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case fileCh <- path:
					return nil
				}
			})
		}()
	}

	for {
		select {
		case <-ctx.Done():
			cleanup()
			return "", time.Time{}, ctx.Err()
		case path, ok := <-fileCh:
			if !ok {
				goto done
			}
			if err := addFileToArchive(tarw, path, sourceDir); err != nil {
				cleanup()
				return "", time.Time{}, err
			}
		}
	}

done:
	if err := <-walkErr; err != nil {
		cleanup()
		return "", time.Time{}, err
	}

	if err := tarw.Close(); err != nil {
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

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		cleanup()
		return "", time.Time{}, fmt.Errorf("stat archive: %w", err)
	}

	return tmpFile.Name(), info.ModTime(), nil
}

// addFileToArchive writes a single file into the tar stream.
// We convert the filename into a relative path so archive consumers do not
// leak local directory structure.
func addFileToArchive(
	tw *tar.Writer,
	path string,
	root string,
) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("rel path: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("header %s: %w", path, err)
	}
	header.Name = rel

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header %s: %w", path, err)
	}

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("copy %s: %w", path, err)
	}

	return nil
}
