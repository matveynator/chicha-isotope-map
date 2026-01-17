package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AnalyticsSession stores durable, human-readable visitor identity metadata so
// logs can tell real people apart across requests.
type AnalyticsSession struct {
	SessionID     string
	DisplayName   string
	VisitorNumber int
	Fingerprint   string
	IP            string
	UserAgent     string
	Referer       string
	CreatedAt     int64
	LastSeenAt    int64
	VisitCount    int
}

// AnalyticsEvent captures a single user action or request with lightweight
// metadata for later summarization.
type AnalyticsEvent struct {
	SessionID   string
	DisplayName string
	OccurredAt  int64
	Kind        string
	Path        string
	IP          string
	Referer     string
	UserAgent   string
	Region      string
	Theme       string
	Layer       string
	Speed       string
	MapZoom     int
	CenterLat   float64
	CenterLon   float64
	DoseClass   string
	TrackKind   string
	TrackID     string
	Detector    string
	Detail      string
}

// AnalyticsSummaryItem holds a label/count pair for hourly rollups.
type AnalyticsSummaryItem struct {
	Label string
	Count int
}

// AnalyticsSummary aggregates the headline hourly metrics into a single struct
// so the logger can emit concise, human-readable lines.
type AnalyticsSummary struct {
	TotalEvents    int
	UniqueSessions int
	TopUsers       []AnalyticsSummaryItem
	TopKinds       []AnalyticsSummaryItem
	TopRegions     []AnalyticsSummaryItem
	TopDoseClasses []AnalyticsSummaryItem
	TopTrackKinds  []AnalyticsSummaryItem
	TopReferrers   []AnalyticsSummaryItem
}

// DeviceSummary holds track-level device hints for activity logging.
type DeviceSummary struct {
	Detector   string
	DeviceName string
	Tube       string
	Transport  string
}

// UpsertAnalyticsSession records the latest activity for a session while keeping
// the visit count aligned to unique days.
func (db *Database) UpsertAnalyticsSession(ctx context.Context, session AnalyticsSession, dbType string) error {
	if db == nil {
		return fmt.Errorf("nil database")
	}
	return db.withSerializedConnectionFor(ctx, WorkloadGeneral, func(ctx context.Context, conn *sql.DB) error {
		query := fmt.Sprintf(`SELECT display_name, last_seen_at, visit_count, visitor_number FROM analytics_sessions WHERE session_id = %s ORDER BY last_seen_at DESC LIMIT 1`, placeholder(dbType, 1))
		var (
			existingName  string
			lastSeen      sql.NullInt64
			visitCount    sql.NullInt64
			visitorNumber sql.NullInt64
		)
		err := conn.QueryRowContext(ctx, query, session.SessionID).Scan(&existingName, &lastSeen, &visitCount, &visitorNumber)
		if err != nil && err != sql.ErrNoRows {
			return err
		}

		now := session.LastSeenAt
		if now == 0 {
			now = time.Now().UTC().Unix()
		}

		name := strings.TrimSpace(session.DisplayName)
		if name == "" {
			name = existingName
		}
		number := session.VisitorNumber
		if number == 0 && visitorNumber.Valid {
			number = int(visitorNumber.Int64)
		}

		visits := int(visitCount.Int64)
		if visits == 0 {
			visits = 1
		}
		if lastSeen.Valid && !sameUTCDate(lastSeen.Int64, now) {
			visits++
		}
		if err == sql.ErrNoRows && session.CreatedAt == 0 {
			session.CreatedAt = now
		}

		if strings.EqualFold(dbType, "clickhouse") {
			insert := fmt.Sprintf(`INSERT INTO analytics_sessions (session_id, display_name, visitor_number, fingerprint, created_at, last_seen_at, visit_count, ip, user_agent, referer)
VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
				placeholder(dbType, 1),
				placeholder(dbType, 2),
				placeholder(dbType, 3),
				placeholder(dbType, 4),
				placeholder(dbType, 5),
				placeholder(dbType, 6),
				placeholder(dbType, 7),
				placeholder(dbType, 8),
				placeholder(dbType, 9),
				placeholder(dbType, 10))
			_, execErr := conn.ExecContext(ctx, insert,
				session.SessionID,
				name,
				number,
				session.Fingerprint,
				session.CreatedAt,
				now,
				visits,
				session.IP,
				session.UserAgent,
				session.Referer,
			)
			return execErr
		}

		if err == sql.ErrNoRows {
			insert := fmt.Sprintf(`INSERT INTO analytics_sessions (session_id, display_name, visitor_number, fingerprint, created_at, last_seen_at, visit_count, ip, user_agent, referer)
VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
				placeholder(dbType, 1),
				placeholder(dbType, 2),
				placeholder(dbType, 3),
				placeholder(dbType, 4),
				placeholder(dbType, 5),
				placeholder(dbType, 6),
				placeholder(dbType, 7),
				placeholder(dbType, 8),
				placeholder(dbType, 9),
				placeholder(dbType, 10))
			_, execErr := conn.ExecContext(ctx, insert,
				session.SessionID,
				name,
				number,
				session.Fingerprint,
				session.CreatedAt,
				now,
				visits,
				session.IP,
				session.UserAgent,
				session.Referer,
			)
			return execErr
		}

		update := fmt.Sprintf(`UPDATE analytics_sessions SET display_name = %s, visitor_number = CASE WHEN %s > 0 THEN %s ELSE visitor_number END, fingerprint = CASE WHEN %s != '' THEN %s ELSE fingerprint END, last_seen_at = %s, visit_count = %s, ip = %s, user_agent = %s, referer = %s WHERE session_id = %s`,
			placeholder(dbType, 1),
			placeholder(dbType, 2),
			placeholder(dbType, 3),
			placeholder(dbType, 4),
			placeholder(dbType, 5),
			placeholder(dbType, 6),
			placeholder(dbType, 7),
			placeholder(dbType, 8),
			placeholder(dbType, 9),
			placeholder(dbType, 10),
			placeholder(dbType, 11))
		_, execErr := conn.ExecContext(ctx, update,
			name,
			number,
			number,
			session.Fingerprint,
			session.Fingerprint,
			now,
			visits,
			session.IP,
			session.UserAgent,
			session.Referer,
			session.SessionID,
		)
		return execErr
	})
}

// AnalyticsSessionByFingerprint resolves a session using the stable fingerprint
// so repeat visitors can be recognized even when their IP changes.
func (db *Database) AnalyticsSessionByFingerprint(ctx context.Context, fingerprint string, dbType string) (AnalyticsSession, bool, error) {
	if db == nil {
		return AnalyticsSession{}, false, fmt.Errorf("nil database")
	}
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return AnalyticsSession{}, false, nil
	}
	var result AnalyticsSession
	err := db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		query := fmt.Sprintf(`SELECT session_id, display_name, visitor_number FROM analytics_sessions WHERE fingerprint = %s ORDER BY last_seen_at DESC LIMIT 1`, placeholder(dbType, 1))
		var number sql.NullInt64
		err := conn.QueryRowContext(ctx, query, fingerprint).Scan(&result.SessionID, &result.DisplayName, &number)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		if number.Valid {
			result.VisitorNumber = int(number.Int64)
		}
		return nil
	})
	if err != nil {
		return AnalyticsSession{}, false, err
	}
	if result.SessionID == "" {
		return AnalyticsSession{}, false, nil
	}
	return result, true, nil
}

// AnalyticsVisitorSeed returns the highest known visitor index so new sessions
// can continue the sequential counter without reusing numbers after restarts.
func (db *Database) AnalyticsVisitorSeed(ctx context.Context, dbType string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("nil database")
	}
	var seed int
	err := db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		query := `SELECT COALESCE(MAX(visitor_number), 0), COUNT(*) FROM analytics_sessions`
		var maxNumber sql.NullInt64
		var count sql.NullInt64
		if err := conn.QueryRowContext(ctx, query).Scan(&maxNumber, &count); err != nil {
			return err
		}
		maxValue := maxNumber.Int64
		countValue := count.Int64
		if countValue > maxValue {
			maxValue = countValue
		}
		seed = int(maxValue)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return seed, nil
}

// InsertAnalyticsEvent records an activity event without blocking the caller.
func (db *Database) InsertAnalyticsEvent(ctx context.Context, event AnalyticsEvent, dbType string) error {
	if db == nil {
		return fmt.Errorf("nil database")
	}
	return db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		insert := fmt.Sprintf(`INSERT INTO analytics_events (
  session_id, display_name, occurred_at, kind, path, ip, referer, user_agent,
  region, map_theme, map_layer, map_zoom, map_speed, map_center_lat, map_center_lon,
  dose_class, track_kind, track_id, detector, detail
) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
			placeholder(dbType, 1),
			placeholder(dbType, 2),
			placeholder(dbType, 3),
			placeholder(dbType, 4),
			placeholder(dbType, 5),
			placeholder(dbType, 6),
			placeholder(dbType, 7),
			placeholder(dbType, 8),
			placeholder(dbType, 9),
			placeholder(dbType, 10),
			placeholder(dbType, 11),
			placeholder(dbType, 12),
			placeholder(dbType, 13),
			placeholder(dbType, 14),
			placeholder(dbType, 15),
			placeholder(dbType, 16),
			placeholder(dbType, 17),
			placeholder(dbType, 18),
			placeholder(dbType, 19),
			placeholder(dbType, 20),
		)
		_, err := conn.ExecContext(ctx, insert,
			event.SessionID,
			event.DisplayName,
			event.OccurredAt,
			event.Kind,
			event.Path,
			event.IP,
			event.Referer,
			event.UserAgent,
			event.Region,
			event.Theme,
			event.Layer,
			event.MapZoom,
			event.Speed,
			event.CenterLat,
			event.CenterLon,
			event.DoseClass,
			event.TrackKind,
			event.TrackID,
			event.Detector,
			event.Detail,
		)
		return err
	})
}

// QueryAnalyticsSummary returns an hourly rollup used for log output.
func (db *Database) QueryAnalyticsSummary(ctx context.Context, start, end int64, limit int, dbType string) (AnalyticsSummary, error) {
	if db == nil {
		return AnalyticsSummary{}, fmt.Errorf("nil database")
	}
	summary := AnalyticsSummary{}
	err := db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		total, err := queryCount(ctx, conn, dbType, `SELECT COUNT(*) FROM analytics_events WHERE occurred_at >= %s AND occurred_at < %s`, start, end)
		if err != nil {
			return err
		}
		unique, err := queryCount(ctx, conn, dbType, `SELECT COUNT(DISTINCT session_id) FROM analytics_events WHERE occurred_at >= %s AND occurred_at < %s`, start, end)
		if err != nil {
			return err
		}
		summary.TotalEvents = total
		summary.UniqueSessions = unique
		summary.TopUsers, err = queryTopUsers(ctx, conn, dbType, start, end, limit)
		if err != nil {
			return err
		}
		summary.TopKinds, err = queryTopList(ctx, conn, dbType, `SELECT kind, COUNT(*) FROM analytics_events WHERE occurred_at >= %s AND occurred_at < %s GROUP BY kind ORDER BY COUNT(*) DESC LIMIT %s`, start, end, limit)
		if err != nil {
			return err
		}
		summary.TopRegions, err = queryTopList(ctx, conn, dbType, `SELECT region, COUNT(*) FROM analytics_events WHERE occurred_at >= %s AND occurred_at < %s AND region != '' GROUP BY region ORDER BY COUNT(*) DESC LIMIT %s`, start, end, limit)
		if err != nil {
			return err
		}
		summary.TopDoseClasses, err = queryTopList(ctx, conn, dbType, `SELECT dose_class, COUNT(*) FROM analytics_events WHERE occurred_at >= %s AND occurred_at < %s AND dose_class != '' GROUP BY dose_class ORDER BY COUNT(*) DESC LIMIT %s`, start, end, limit)
		if err != nil {
			return err
		}
		summary.TopTrackKinds, err = queryTopList(ctx, conn, dbType, `SELECT track_kind, COUNT(*) FROM analytics_events WHERE occurred_at >= %s AND occurred_at < %s AND track_kind != '' GROUP BY track_kind ORDER BY COUNT(*) DESC LIMIT %s`, start, end, limit)
		if err != nil {
			return err
		}
		summary.TopReferrers, err = queryTopList(ctx, conn, dbType, `SELECT referer, COUNT(*) FROM analytics_events WHERE occurred_at >= %s AND occurred_at < %s AND referer != '' GROUP BY referer ORDER BY COUNT(*) DESC LIMIT %s`, start, end, limit)
		if err != nil {
			return err
		}
		return nil
	})
	return summary, err
}

func queryCount(ctx context.Context, conn *sql.DB, dbType string, template string, start, end int64) (int, error) {
	query := fmt.Sprintf(template, placeholder(dbType, 1), placeholder(dbType, 2))
	var count int
	if err := conn.QueryRowContext(ctx, query, start, end).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func queryTopUsers(ctx context.Context, conn *sql.DB, dbType string, start, end int64, limit int) ([]AnalyticsSummaryItem, error) {
	query := fmt.Sprintf(`SELECT display_name, session_id, COUNT(*) FROM analytics_events
WHERE occurred_at >= %s AND occurred_at < %s
GROUP BY display_name, session_id
ORDER BY COUNT(*) DESC
LIMIT %s`, placeholder(dbType, 1), placeholder(dbType, 2), placeholder(dbType, 3))
	rows, err := conn.QueryContext(ctx, query, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AnalyticsSummaryItem
	for rows.Next() {
		var name, sessionID string
		var count int
		if err := rows.Scan(&name, &sessionID, &count); err != nil {
			return nil, err
		}
		label := strings.TrimSpace(name)
		if label == "" {
			label = sessionID
		}
		items = append(items, AnalyticsSummaryItem{Label: label, Count: count})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func queryTopList(ctx context.Context, conn *sql.DB, dbType string, template string, start, end int64, limit int) ([]AnalyticsSummaryItem, error) {
	query := fmt.Sprintf(template, placeholder(dbType, 1), placeholder(dbType, 2), placeholder(dbType, 3))
	rows, err := conn.QueryContext(ctx, query, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AnalyticsSummaryItem
	for rows.Next() {
		var label sql.NullString
		var count int
		if err := rows.Scan(&label, &count); err != nil {
			return nil, err
		}
		value := strings.TrimSpace(label.String)
		if value == "" {
			value = "unknown"
		}
		items = append(items, AnalyticsSummaryItem{Label: value, Count: count})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func sameUTCDate(a, b int64) bool {
	tA := time.Unix(a, 0).UTC()
	tB := time.Unix(b, 0).UTC()
	return tA.Year() == tB.Year() && tA.YearDay() == tB.YearDay()
}

// GetTrackDeviceSummary fetches a single marker for the track so uploads can
// be tagged with detector metadata without scanning the full dataset.
func (db *Database) GetTrackDeviceSummary(ctx context.Context, trackID, dbType string) (DeviceSummary, error) {
	if db == nil {
		return DeviceSummary{}, fmt.Errorf("nil database")
	}
	var summary DeviceSummary
	err := db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		query := fmt.Sprintf(`SELECT detector, device_name, tube, transport FROM markers WHERE trackID = %s LIMIT 1`, placeholder(dbType, 1))
		row := conn.QueryRowContext(ctx, query, trackID)
		return row.Scan(&summary.Detector, &summary.DeviceName, &summary.Tube, &summary.Transport)
	})
	if err == sql.ErrNoRows {
		return summary, nil
	}
	return summary, err
}
