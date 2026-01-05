package atomfast

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

// TrackRef describes a remote AtomFast track resource.
type TrackRef struct {
	ID  string
	URL string
}

// Config captures the options for AtomFast discovery so callers can tune timing.
type Config struct {
	ListURL     string
	Interval    time.Duration
	MaxPages    int
	MaxBodySize int64
	UserAgent   string
	Client      *http.Client
}

const defaultBodyLimit = int64(6 << 20)

// StartDiscovery launches a background loop that emits discovered tracks.
// The returned channel closes when the context is cancelled.
func StartDiscovery(ctx context.Context, cfg Config, logf func(string, ...any)) <-chan TrackRef {
	out := make(chan TrackRef)

	go func() {
		defer close(out)

		interval := cfg.Interval
		if interval <= 0 {
			interval = 30 * time.Minute
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			refs, err := DiscoverTracks(ctx, cfg)
			if err != nil && logf != nil {
				logf("atomfast discovery error: %v", err)
			}
			for _, ref := range refs {
				select {
				case <-ctx.Done():
					return
				case out <- ref:
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return out
}

// DiscoverTracks walks paginated AtomFast pages and extracts track links.
func DiscoverTracks(ctx context.Context, cfg Config) ([]TrackRef, error) {
	if strings.TrimSpace(cfg.ListURL) == "" {
		return nil, fmt.Errorf("list url missing")
	}

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	maxPages := cfg.MaxPages
	if maxPages <= 0 {
		maxPages = 50
	}

	bodyLimit := cfg.MaxBodySize
	if bodyLimit <= 0 {
		bodyLimit = defaultBodyLimit
	}

	refs := make([]TrackRef, 0, 32)
	seen := make(map[string]struct{})
	emptyPages := 0

	for page := 1; page <= maxPages; page++ {
		pageURL, err := withPageParam(cfg.ListURL, page)
		if err != nil {
			return refs, err
		}

		body, err := fetchURL(ctx, client, cfg.UserAgent, pageURL, bodyLimit)
		if err != nil {
			return refs, err
		}

		links := extractTrackLinks(body, pageURL)
		if len(links) == 0 {
			emptyPages++
			if emptyPages >= 2 && page > 1 {
				break
			}
			continue
		}
		emptyPages = 0

		for _, ref := range links {
			if _, ok := seen[ref.URL]; ok {
				continue
			}
			seen[ref.URL] = struct{}{}
			refs = append(refs, ref)
		}
	}

	return refs, nil
}

// FetchTrack downloads the raw track payload for later parsing.
func FetchTrack(ctx context.Context, client *http.Client, userAgent string, ref TrackRef) ([]byte, error) {
	if strings.TrimSpace(ref.URL) == "" {
		return nil, fmt.Errorf("track url missing")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return fetchURL(ctx, client, userAgent, ref.URL, 20<<20)
}

// withPageParam updates the p query parameter while preserving other params.
func withPageParam(raw string, page int) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse list url: %w", err)
	}
	q := u.Query()
	q.Set("p", fmt.Sprintf("%d", page))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// fetchURL downloads a page with a conservative size cap to keep memory bounded.
func fetchURL(ctx context.Context, client *http.Client, userAgent, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "ChichaAtomFastSync/1.0"
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", rawURL, resp.StatusCode)
	}

	reader := io.LimitReader(resp.Body, limit)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rawURL, err)
	}
	return body, nil
}

var (
	linkPattern      = regexp.MustCompile(`(?i)(?:href|data-url|data-file)=["']([^"']+?\\.json[^"']*)["']`)
	absoluteJSONLink = regexp.MustCompile(`(?i)https?://[^\\s"'<>]+?\\.json[^\\s"'<>]*`)
)

// extractTrackLinks searches an AtomFast page for JSON track links.
func extractTrackLinks(body []byte, rawBase string) []TrackRef {
	baseURL, err := url.Parse(rawBase)
	if err != nil {
		return nil
	}

	candidates := make([]string, 0, 16)
	matches := linkPattern.FindAllSubmatch(body, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		candidates = append(candidates, string(m[1]))
	}
	plain := absoluteJSONLink.FindAllString(string(body), -1)
	candidates = append(candidates, plain...)

	refs := make([]TrackRef, 0, len(candidates))
	seen := make(map[string]struct{})
	for _, raw := range candidates {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if !u.IsAbs() {
			u = baseURL.ResolveReference(u)
		}
		refURL := u.String()
		if _, ok := seen[refURL]; ok {
			continue
		}
		seen[refURL] = struct{}{}
		refs = append(refs, TrackRef{
			ID:  normalizeTrackID(u),
			URL: refURL,
		})
	}
	return refs
}

// normalizeTrackID derives a stable identifier from the URL path.
func normalizeTrackID(u *url.URL) string {
	base := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	base = strings.TrimSpace(base)
	if base == "" {
		sum := sha1.Sum([]byte(u.String()))
		return "track-" + hex.EncodeToString(sum[:6])
	}
	var b strings.Builder
	for _, r := range base {
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
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		sum := sha1.Sum([]byte(u.String()))
		return "track-" + hex.EncodeToString(sum[:6])
	}
	return out
}
