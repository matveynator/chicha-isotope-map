package selfupgrade

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// StaticFetcher talks to a fixed HTTP endpoint that serves the linux/amd64 binary.
// We keep the metadata lean: custom headers announce the version and whether a
// database backup is required. This keeps the updater decoupled from GitHub's
// release API and follows "The bigger the interface, the weaker the abstraction".
type StaticFetcher struct {
	URL    string
	Client *http.Client
}

// Latest performs a HEAD request so we avoid downloading the binary unless a new
// version appears. Operators can expose custom headers:
//
//	X-Chicha-Version: semantic version for display and caching.
//	X-Chicha-DB-Migration: "true" when the release requires a database backup.
func (f StaticFetcher) Latest(ctx context.Context) (Release, error) {
	if strings.TrimSpace(f.URL) == "" {
		return Release{}, errors.New("selfupgrade: download URL is empty")
	}

	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, f.URL, nil)
	if err != nil {
		return Release{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Release{}, fmt.Errorf("selfupgrade: HEAD %s failed with %d", f.URL, resp.StatusCode)
	}

	version := strings.TrimSpace(resp.Header.Get("X-Chicha-Version"))
	if version == "" {
		version = strings.Trim(resp.Header.Get("ETag"), "\"")
	}
	if version == "" {
		version = strings.TrimSpace(resp.Header.Get("Last-Modified"))
	}
	if version == "" {
		version = fmt.Sprintf("build-%d", time.Now().Unix())
	}

	needsBackup := false
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("X-Chicha-DB-Migration")), "true") {
		needsBackup = true
	}

	assetName := filepath.Base(f.URL)
	if strings.TrimSpace(assetName) == "" || assetName == "." || assetName == string(filepath.Separator) {
		assetName = "chicha-isotope-map_linux_amd64"
	}

	release := Release{
		Tag:              version,
		Name:             version,
		Published:        time.Now(),
		RequiresDBBackup: needsBackup,
	}
	release.Assets = append(release.Assets, ReleaseAsset{
		Name:        assetName,
		DownloadURL: f.URL,
		ContentType: resp.Header.Get("Content-Type"),
		Size:        resp.ContentLength,
	})
	return release, nil
}
