package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// =====================
// Track source mappings
// =====================

// ResolveTrackSource returns the internal track ID mapped to an upstream source
// track. We keep the lookup tiny so external loaders can skip re-downloading
// data they already merged into a different track ID.
func (db *Database) ResolveTrackSource(ctx context.Context, source, sourceTrackID, dbType string) (string, error) {
	source = strings.TrimSpace(source)
	sourceTrackID = strings.TrimSpace(sourceTrackID)
	if source == "" || sourceTrackID == "" {
		return "", nil
	}
	phSource := placeholder(dbType, 1)
	phSourceID := placeholder(dbType, 2)
	query := fmt.Sprintf(`SELECT track_id FROM track_sources WHERE source = %s AND source_track_id = %s LIMIT 1`, phSource, phSourceID)
	var trackID string
	if err := db.DB.QueryRowContext(ctx, query, source, sourceTrackID).Scan(&trackID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("resolve track source: %w", err)
	}
	return trackID, nil
}

// EnsureTrackSource records an upstream-to-internal track mapping so loaders
// can skip duplicate fetches after de-duplication rewrites a TrackID.
func (db *Database) EnsureTrackSource(ctx context.Context, source, sourceTrackID, trackID, dbType string) error {
	source = strings.TrimSpace(source)
	sourceTrackID = strings.TrimSpace(sourceTrackID)
	trackID = strings.TrimSpace(trackID)
	if source == "" || sourceTrackID == "" || trackID == "" {
		return nil
	}
	if strings.ToLower(dbType) == "clickhouse" {
		if existing, err := db.ResolveTrackSource(ctx, source, sourceTrackID, dbType); err != nil {
			return err
		} else if existing != "" {
			return nil
		}
		stmt := "INSERT INTO track_sources (source, source_track_id, track_id) VALUES (?, ?, ?)"
		if _, err := db.DB.ExecContext(ctx, stmt, source, sourceTrackID, trackID); err != nil {
			return fmt.Errorf("ensure track source: %w", err)
		}
		return nil
	}

	insertSource := placeholder(dbType, 1)
	insertSourceID := placeholder(dbType, 2)
	insertTrackID := placeholder(dbType, 3)
	existsSource := placeholder(dbType, 4)
	existsSourceID := placeholder(dbType, 5)
	stmt := fmt.Sprintf(`INSERT INTO track_sources (source, source_track_id, track_id)
SELECT %s, %s, %s
WHERE NOT EXISTS (SELECT 1 FROM track_sources WHERE source = %s AND source_track_id = %s);`, insertSource, insertSourceID, insertTrackID, existsSource, existsSourceID)
	if _, err := db.DB.ExecContext(ctx, stmt, source, sourceTrackID, trackID, source, sourceTrackID); err != nil {
		return fmt.Errorf("ensure track source: %w", err)
	}
	return nil
}
