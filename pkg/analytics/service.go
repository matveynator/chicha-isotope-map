package analytics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// Service wires session tracking, event ingestion, and hourly summaries together
// using channels so request handlers never block on IO-heavy operations.
type Service struct {
	db             *database.Database
	dbType         string
	logf           func(string, ...any)
	messages       chan message
	numbers        chan numberRequest
	clock          func() time.Time
	visitorCounter int
	counterLoaded  bool
}

// Session keeps the resolved visitor identity handy for handlers that want to
// attach richer details to activity events.
type Session struct {
	ID   string
	Name string
}

type messageKind int

const (
	messageSession messageKind = iota
	messageEvent
)

type message struct {
	kind    messageKind
	session database.AnalyticsSession
	event   database.AnalyticsEvent
}

// numberRequest serializes visitor numbering so handler goroutines avoid races
// without sharing mutable counters directly.
type numberRequest struct {
	ctx  context.Context
	resp chan numberResponse
}

type numberResponse struct {
	number int
	err    error
}

const (
	sessionCookieName = "cim_session"
	nameCookieName    = "cim_session_name"
	cookieMaxAge      = 365 * 24 * 60 * 60
)

var sessionContextKey = &struct{}{}

// NewService prepares the analytics pipeline without starting it so callers can
// decide when to spawn goroutines.
func NewService(db *database.Database, dbType string, logf func(string, ...any)) *Service {
	if logf == nil {
		logf = log.Printf
	}
	return &Service{
		db:       db,
		dbType:   strings.ToLower(strings.TrimSpace(dbType)),
		logf:     logf,
		messages: make(chan message, 256),
		numbers:  make(chan numberRequest, 32),
		clock:    time.Now,
	}
}

// Start launches the background worker that flushes events and emits summaries.
func (s *Service) Start(ctx context.Context) {
	if s == nil {
		return
	}
	go s.run(ctx)
}

// Middleware assigns session cookies and logs high-level HTTP activity without
// blocking request handlers.
func (s *Service) Middleware(next http.Handler) http.Handler {
	if s == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.shouldTrackRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		session := s.ensureSession(w, r)
		ctx := context.WithValue(r.Context(), sessionContextKey, session)
		r = r.WithContext(ctx)

		if kind, detail := s.classifyRequest(r); kind != "" {
			event := database.AnalyticsEvent{
				SessionID:   session.ID,
				DisplayName: session.Name,
				OccurredAt:  s.clock().UTC().Unix(),
				Kind:        kind,
				Path:        r.URL.Path,
				IP:          clientIP(r),
				Referer:     strings.TrimSpace(r.Referer()),
				UserAgent:   strings.TrimSpace(r.UserAgent()),
				Detail:      encodeDetail(detail),
			}
			s.RecordEvent(event)
		}

		next.ServeHTTP(w, r)
	})
}

// SessionFromContext exposes the resolved visitor identity to request handlers.
func SessionFromContext(ctx context.Context) (Session, bool) {
	if ctx == nil {
		return Session{}, false
	}
	value := ctx.Value(sessionContextKey)
	if value == nil {
		return Session{}, false
	}
	session, ok := value.(Session)
	return session, ok
}

// RecordEvent enqueues a single analytics event, dropping it if the queue is
// saturated to keep the UI responsive.
func (s *Service) RecordEvent(event database.AnalyticsEvent) {
	if s == nil {
		return
	}
	select {
	case s.messages <- message{kind: messageEvent, event: event}:
	default:
		s.logf("activity: event queue full, dropping kind=%s session=%s", event.Kind, event.SessionID)
	}
}

// ApproximateRegion converts coordinates into a coarse, human-readable
// descriptor so summaries stay meaningful without external GeoIP data.
func ApproximateRegion(lat, lon float64) string {
	if math.IsNaN(lat) || math.IsNaN(lon) {
		return "unknown"
	}
	latBand := band(lat, 30)
	lonBand := band(lon, 30)
	latDir := hemisphere(lat, "N", "S")
	lonDir := hemisphere(lon, "E", "W")
	return fmt.Sprintf("%s %s°, %s %s°", latBand, latDir, lonBand, lonDir)
}

// ClassifyDose summarizes dose-rate observations so logs read like quick health
// checks instead of raw numbers.
func ClassifyDose(markers []database.Marker) (string, float64) {
	if len(markers) == 0 {
		return "unknown", 0
	}
	maxDose := 0.0
	for _, marker := range markers {
		if marker.DoseRate > maxDose {
			maxDose = marker.DoseRate
		}
	}
	switch {
	case maxDose >= 1.0:
		return "high", maxDose
	case maxDose >= 0.3:
		return "medium", maxDose
	case maxDose > 0:
		return "low", maxDose
	default:
		return "unknown", maxDose
	}
}

// -------------------------
// Internal helpers
// -------------------------

// nextVisitorNumber asks the background worker for a sequential visitor counter
// so cookies remain stable without racing on shared state.
func (s *Service) nextVisitorNumber(ctx context.Context) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("analytics service unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req := numberRequest{
		ctx:  ctx,
		resp: make(chan numberResponse, 1),
	}

	select {
	case s.numbers <- req:
	case <-ctx.Done():
		return 0, ctx.Err()
	}

	select {
	case res := <-req.resp:
		return res.number, res.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// handleNumberRequest keeps the counter update on the run goroutine so only
// channels coordinate access, following "Don't communicate by sharing memory."
func (s *Service) handleNumberRequest(ctx context.Context, req numberRequest) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !s.counterLoaded {
		seed, err := s.loadVisitorSeed(ctx)
		if err != nil {
			s.logf("activity: visitor seed load failed: %v", err)
		}
		s.visitorCounter = seed
		s.counterLoaded = true
	}
	s.visitorCounter++
	response := numberResponse{number: s.visitorCounter}
	select {
	case req.resp <- response:
	case <-req.ctx.Done():
	}
}

// loadVisitorSeed reads the current maximum visitor number so counters continue
// across restarts without reusing IDs.
func (s *Service) loadVisitorSeed(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, nil
	}
	seed, err := s.db.AnalyticsVisitorSeed(ctx, s.dbType)
	if err != nil {
		return 0, err
	}
	return seed, nil
}

func (s *Service) run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logf("activity: shutdown")
			return
		case msg := <-s.messages:
			s.handleMessage(ctx, msg)
		case req := <-s.numbers:
			s.handleNumberRequest(ctx, req)
		case tick := <-ticker.C:
			s.emitSummary(ctx, tick)
		}
	}
}

func (s *Service) handleMessage(ctx context.Context, msg message) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	switch msg.kind {
	case messageSession:
		if s.db == nil {
			return
		}
		if err := s.db.UpsertAnalyticsSession(ctx, msg.session, s.dbType); err != nil {
			s.logf("activity: session upsert failed: %v", err)
			return
		}
		if msg.session.CreatedAt != 0 {
			s.logf("activity: new visitor %s (%s) ip=%s", msg.session.DisplayName, msg.session.SessionID, msg.session.IP)
		}
	case messageEvent:
		if s.db == nil {
			return
		}
		if err := s.db.InsertAnalyticsEvent(ctx, msg.event, s.dbType); err != nil {
			s.logf("activity: event insert failed: %v", err)
			return
		}
		s.logf("activity: %s by %s (%s) path=%s", msg.event.Kind, msg.event.DisplayName, msg.event.SessionID, msg.event.Path)
	}
}

func (s *Service) ensureSession(w http.ResponseWriter, r *http.Request) Session {
	sessionID := readCookieValue(r, sessionCookieName)
	name := readCookieValue(r, nameCookieName)
	visitorNumber := parseVisitorNumber(name)
	newSession := false

	if sessionID == "" {
		sessionID = newSessionID()
		newSession = true
	}
	if name == "" {
		number, err := s.nextVisitorNumber(r.Context())
		if err != nil {
			s.logf("activity: visitor number fallback: %v", err)
			number = pickNumber(100, 999)
		}
		visitorNumber = number
		name = newSessionName(number)
		newSession = true
	}

	if newSession {
		writeCookie(w, r, sessionCookieName, sessionID)
		writeCookie(w, r, nameCookieName, url.QueryEscape(name))
	}

	session := Session{ID: sessionID, Name: name}

	s.enqueueSession(database.AnalyticsSession{
		SessionID:     sessionID,
		DisplayName:   name,
		VisitorNumber: visitorNumber,
		IP:            clientIP(r),
		UserAgent:     strings.TrimSpace(r.UserAgent()),
		Referer:       strings.TrimSpace(r.Referer()),
		CreatedAt:     createdAtValue(newSession, s.clock().UTC()),
		LastSeenAt:    s.clock().UTC().Unix(),
	})

	return session
}

func (s *Service) enqueueSession(session database.AnalyticsSession) {
	select {
	case s.messages <- message{kind: messageSession, session: session}:
	default:
		s.logf("activity: session queue full, dropping %s", session.SessionID)
	}
}

func (s *Service) shouldTrackRequest(r *http.Request) bool {
	path := r.URL.Path
	if path == "" {
		return false
	}
	skipPrefixes := []string{"/static/", "/favicon", "/robots.txt", "/sitemap"}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

func (s *Service) classifyRequest(r *http.Request) (string, map[string]any) {
	method := strings.ToUpper(r.Method)
	if method != http.MethodGet && method != http.MethodHead {
		return "", nil
	}
	path := r.URL.Path

	if strings.HasPrefix(path, "/get_markers") || strings.HasPrefix(path, "/stream_") || strings.HasPrefix(path, "/upload") {
		return "", nil
	}
	if strings.HasPrefix(path, "/api/shorten") {
		return "", nil
	}

	detail := map[string]any{
		"method": method,
		"query":  r.URL.RawQuery,
	}

	switch {
	case path == "/" || path == "":
		return "page_view", detail
	case strings.HasPrefix(path, "/trackid/"):
		detail["track_id"] = strings.TrimPrefix(strings.TrimSuffix(path, "/"), "/trackid/")
		return "track_view", detail
	case strings.HasPrefix(path, "/api/docs"):
		return "docs_view", detail
	case strings.HasPrefix(path, "/api/"):
		return "api_request", detail
	case strings.HasPrefix(path, "/licenses/"):
		return "license_view", detail
	default:
		return "page_view", detail
	}
}

func (s *Service) emitSummary(ctx context.Context, now time.Time) {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	start := now.Add(-1 * time.Hour).UTC().Unix()
	end := now.UTC().Unix()

	summary, err := s.db.QueryAnalyticsSummary(ctx, start, end, 10, s.dbType)
	if err != nil {
		s.logf("activity: summary failed: %v", err)
		return
	}

	s.logf("activity summary (last hour): visitors=%d events=%d", summary.UniqueSessions, summary.TotalEvents)
	if len(summary.TopUsers) > 0 {
		s.logf("activity summary: top users: %s", formatSummaryList(summary.TopUsers))
	}
	if len(summary.TopKinds) > 0 {
		s.logf("activity summary: top actions: %s", formatSummaryList(summary.TopKinds))
	}
	if len(summary.TopRegions) > 0 {
		s.logf("activity summary: regions: %s", formatSummaryList(summary.TopRegions))
	}
	if len(summary.TopDoseClasses) > 0 {
		s.logf("activity summary: dose levels: %s", formatSummaryList(summary.TopDoseClasses))
	}
	if len(summary.TopTrackKinds) > 0 {
		s.logf("activity summary: track types: %s", formatSummaryList(summary.TopTrackKinds))
	}
	if len(summary.TopReferrers) > 0 {
		s.logf("activity summary: referrers: %s", formatSummaryList(summary.TopReferrers))
	}
}

func formatSummaryList(items []database.AnalyticsSummaryItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s (%d)", item.Label, item.Count))
	}
	return strings.Join(parts, ", ")
}

func encodeDetail(detail map[string]any) string {
	if detail == nil {
		return ""
	}
	payload, err := json.Marshal(detail)
	if err != nil {
		return ""
	}
	return string(payload)
}

func readCookieValue(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(cookie.Value)
	if name == nameCookieName {
		if decoded, err := url.QueryUnescape(value); err == nil {
			return decoded
		}
	}
	return value
}

func writeCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   cookieMaxAge,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	}
	if r.TLS != nil {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

func newSessionID() string {
	payload := make([]byte, 16)
	if _, err := rand.Read(payload); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(payload)
}

func newSessionName(number int) string {
	adjective := pickOne(sessionAdjectives)
	noun := pickOne(sessionNouns)
	if number <= 0 {
		number = pickNumber(100, 999)
	}
	return fmt.Sprintf("%s%s-%d", adjective, noun, number)
}

// parseVisitorNumber extracts the numeric suffix so existing cookies can
// populate the session record without extra database reads.
func parseVisitorNumber(name string) int {
	if name == "" {
		return 0
	}
	idx := strings.LastIndex(name, "-")
	if idx < 0 || idx+1 >= len(name) {
		return 0
	}
	value := strings.TrimSpace(name[idx+1:])
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func pickOne(values []string) string {
	if len(values) == 0 {
		return "Visitor"
	}
	idx := pickNumber(0, len(values)-1)
	return values[idx]
}

func pickNumber(min, max int) int {
	if max <= min {
		return min
	}
	span := max - min + 1
	value, err := rand.Int(rand.Reader, big.NewInt(int64(span)))
	if err != nil {
		return min
	}
	return int(value.Int64()) + min
}

func createdAtValue(isNew bool, now time.Time) int64 {
	if !isNew {
		return 0
	}
	return now.Unix()
}

func clientIP(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		candidate := strings.TrimSpace(parts[0])
		if candidate != "" {
			return candidate
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	if trimmed := strings.TrimSpace(r.RemoteAddr); trimmed != "" {
		return trimmed
	}
	return ""
}

func band(value float64, size float64) string {
	if size <= 0 {
		return "0"
	}
	base := math.Floor(math.Abs(value)/size) * size
	low := int(base)
	high := int(base + size)
	return fmt.Sprintf("%d-%d", low, high)
}

func hemisphere(value float64, positive, negative string) string {
	if value >= 0 {
		return positive
	}
	return negative
}

var sessionAdjectives = []string{
	"Bright",
	"Calm",
	"Curious",
	"Gentle",
	"Keen",
	"Quiet",
	"Swift",
	"Witty",
	"Bold",
	"Kind",
	"Mellow",
	"Nimble",
	"Sunny",
	"Amber",
	"Silver",
	"Vivid",
}

var sessionNouns = []string{
	"Fox",
	"Otter",
	"Hawk",
	"Pine",
	"River",
	"Atlas",
	"Comet",
	"Beacon",
	"Lumen",
	"Cedar",
	"Harbor",
	"Echo",
	"Orbit",
	"Summit",
	"Trail",
	"Nova",
}
