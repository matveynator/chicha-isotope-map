package safecastimport

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config collects Safecast API options so callers can stay explicit about
// endpoints and timeouts without scattering defaults throughout the code.
type Config struct {
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
}

// Client wraps the Safecast API endpoints with a lean HTTP client so the
// fetcher can keep its dependency surface tiny and predictable.
type Client struct {
	baseURL    string
	userAgent  string
	httpClient *http.Client
	rng        *rand.Rand
	userAgents []string
	platforms  []string
	languages  []string
}

// NewClient builds a Safecast API client while normalizing defaults so every
// caller gets consistent behavior without extra setup.
func NewClient(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "http://safecastapi-prd-010.baebmmfncu.us-west-2.elasticbeanstalk.com"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	agent := strings.TrimSpace(cfg.UserAgent)
	if agent == "" {
		agent = "Mozilla/5.0 (compatible; ChichaIsotopeMap/1.0; +https://github.com/matveynator/chicha-isotope-map)"
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &Client{
		baseURL:   base,
		userAgent: agent,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		rng:        rng,
		userAgents: defaultUserAgents(),
		platforms:  defaultPlatforms(),
		languages:  defaultLanguages(),
	}
}

// Import captures the minimal Safecast API fields we need to download and
// attribute a bGeigie log payload.
type Import struct {
	ID        int64
	SourceURL string
	UserID    int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Status    string
}

type importRaw struct {
	ID     int64 `json:"id"`
	Source struct {
		URL string `json:"url"`
	} `json:"source"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    string    `json:"status"`
}

// FetchApprovedImports pulls the latest approved imports in descending order
// so loaders can walk from newest to oldest without extra sorting work.
func (c *Client) FetchApprovedImports(ctx context.Context, page int) ([]Import, error) {
	if page <= 0 {
		page = 1
	}
	endpoint, err := url.Parse(c.baseURL + "/en-US/bgeigie_imports")
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	query := endpoint.Query()
	query.Set("by_status", "done")
	query.Set("status", "approved")
	query.Set("order", "created_at desc")
	query.Set("page", strconv.Itoa(page))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	c.applyRandomHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request imports: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imports http %d", resp.StatusCode)
	}

	var raw []importRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode imports: %w", err)
	}

	imports := make([]Import, 0, len(raw))
	for _, item := range raw {
		imports = append(imports, Import{
			ID:        item.ID,
			SourceURL: strings.TrimSpace(item.Source.URL),
			UserID:    item.UserID,
			Name:      strings.TrimSpace(item.Name),
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
			Status:    strings.TrimSpace(item.Status),
		})
	}
	return imports, nil
}

// User records the Safecast username so we can attach a friendly label to
// imported tracks for future account binding.
type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// FetchUser retrieves the Safecast user profile so we can persist the name
// alongside the internal user identifier for future registration flows.
func (c *Client) FetchUser(ctx context.Context, userID int64) (*User, error) {
	if userID <= 0 {
		return nil, nil
	}
	endpoint := fmt.Sprintf("%s/users/%d.json", c.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create user request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	c.applyRandomHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user http %d", resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}
	user.Name = strings.TrimSpace(user.Name)
	return &user, nil
}

// applyRandomHeaders rotates browser-like headers so Safecast requests avoid a
// static fingerprint while staying readable to upstream services.
func (c *Client) applyRandomHeaders(req *http.Request) {
	if req == nil {
		return
	}
	userAgent := c.userAgent
	if len(c.userAgents) > 0 {
		userAgent = c.userAgents[c.rng.Intn(len(c.userAgents))]
	}
	platform := ""
	if len(c.platforms) > 0 {
		platform = c.platforms[c.rng.Intn(len(c.platforms))]
	}
	lang := ""
	if len(c.languages) > 0 {
		lang = c.languages[c.rng.Intn(len(c.languages))]
	}

	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if platform != "" {
		req.Header.Set("Sec-CH-UA-Platform", fmt.Sprintf("%q", platform))
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
	}
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("X-Request-Id", randomToken(c.rng, 12))
}

// randomToken returns a short random string so each request is unique even when
// the header pool repeats, keeping request fingerprints diverse.
func randomToken(rng *rand.Rand, length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	if length <= 0 {
		length = 8
	}
	out := make([]byte, length)
	for i := range out {
		out[i] = letters[rng.Intn(len(letters))]
	}
	return string(out)
}

// defaultUserAgents keeps a small pool of real-world browsers so the API stays
// compatible with Safecast's edge filters without adding new dependencies.
func defaultUserAgents() []string {
	return []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_6) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
	}
}

// defaultPlatforms keeps OS hints aligned with the UA pool so headers remain consistent.
func defaultPlatforms() []string {
	return []string{
		"Windows",
		"Windows",
		"macOS",
		"Linux",
		"Windows",
	}
}

// defaultLanguages rotates common Accept-Language headers without forcing
// locale-specific content from the API.
func defaultLanguages() []string {
	return []string{
		"en-US,en;q=0.9",
		"en-GB,en;q=0.9",
		"ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7",
		"en-US,en;q=0.8,ru;q=0.6",
		"en-US,en;q=0.8,de;q=0.5",
	}
}
