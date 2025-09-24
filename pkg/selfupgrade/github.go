package selfupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHubReleaseFetcher pulls the most recent release metadata from GitHub's API.
// We only care about the first stable (non-prerelease) build to keep updates
// predictable for production environments.
type GitHubReleaseFetcher struct {
	Owner  string
	Repo   string
	Client *http.Client
}

// Latest fetches releases ordered by creation date and returns the first
// release that qualifies as stable. GitHub returns the newest release first,
// so the first non-draft, non-prerelease entry is what we want.
func (f *GitHubReleaseFetcher) Latest(ctx context.Context) (Release, error) {
	if strings.TrimSpace(f.Owner) == "" || strings.TrimSpace(f.Repo) == "" {
		return Release{}, errors.New("selfupgrade: github fetcher requires owner and repo")
	}

	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", f.Owner, f.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return Release{}, fmt.Errorf("selfupgrade: github API %s responded %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload []struct {
		TagName    string    `json:"tag_name"`
		Name       string    `json:"name"`
		Body       string    `json:"body"`
		Draft      bool      `json:"draft"`
		Prerelease bool      `json:"prerelease"`
		Published  time.Time `json:"published_at"`
		Assets     []struct {
			Name        string `json:"name"`
			BrowserURL  string `json:"browser_download_url"`
			ContentType string `json:"content_type"`
			Size        int64  `json:"size"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Release{}, err
	}

	for _, item := range payload {
		if item.Draft || item.Prerelease {
			continue
		}
		release := Release{
			Tag:        strings.TrimSpace(item.TagName),
			Name:       strings.TrimSpace(item.Name),
			Body:       item.Body,
			Published:  item.Published,
			Draft:      item.Draft,
			Prerelease: item.Prerelease,
		}
		for _, asset := range item.Assets {
			release.Assets = append(release.Assets, ReleaseAsset{
				Name:        strings.TrimSpace(asset.Name),
				DownloadURL: strings.TrimSpace(asset.BrowserURL),
				ContentType: strings.TrimSpace(asset.ContentType),
				Size:        asset.Size,
			})
		}
		if release.Tag != "" {
			return release, nil
		}
	}

	return Release{}, errors.New("selfupgrade: no stable releases available")
}
