package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// =====================
// Import history
// =====================

// ImportHistory describes a single import attempt stored for de-duplication.
// The table is intentionally non-authoritative so operators can wipe it and
// force a full re-import without touching the real track data.
type ImportHistory struct {
	Source   string
	SourceID string
	TrackID  string
	Status   string
	Imported int64
	Message  string
}

// FindImportHistory returns an existing import history record for the given
// source ID so importers can skip already-processed payloads.
func (db *Database) FindImportHistory(ctx context.Context, source, sourceID, dbType string) (ImportHistory, bool, error) {
	source = strings.TrimSpace(source)
	sourceID = strings.TrimSpace(sourceID)
	if source == "" || sourceID == "" {
		return ImportHistory{}, false, nil
	}
	phSource := placeholder(dbType, 1)
	phSourceID := placeholder(dbType, 2)
	query := fmt.Sprintf(`SELECT source, source_id, track_id, status, imported_at, message FROM import_history WHERE source = %s AND source_id = %s LIMIT 1`, phSource, phSourceID)
	var record ImportHistory
	var (
		trackID sql.NullString
		status  sql.NullString
		message sql.NullString
	)
	if err := db.DB.QueryRowContext(ctx, query, source, sourceID).Scan(&record.Source, &record.SourceID, &trackID, &status, &record.Imported, &message); err != nil {
		if err == sql.ErrNoRows {
			return ImportHistory{}, false, nil
		}
		return ImportHistory{}, false, fmt.Errorf("find import history: %w", err)
	}
	record.TrackID = strings.TrimSpace(trackID.String)
	record.Status = strings.TrimSpace(status.String)
	record.Message = strings.TrimSpace(message.String)
	return record, true, nil
}

// CountImportHistory reports how many records exist for a source so loaders can
// decide whether to run a full backfill or a delta refresh.
func (db *Database) CountImportHistory(ctx context.Context, source, dbType string) (int64, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return 0, nil
	}
	phSource := placeholder(dbType, 1)
	query := fmt.Sprintf(`SELECT COUNT(*) FROM import_history WHERE source = %s`, phSource)
	var count int64
	if err := db.DB.QueryRowContext(ctx, query, source).Scan(&count); err != nil {
		return 0, fmt.Errorf("count import history: %w", err)
	}
	return count, nil
}

// EnsureImportHistory records a completed import so future runs can skip
// already-processed sources. The caller controls the status value.
func (db *Database) EnsureImportHistory(ctx context.Context, source, sourceID, trackID, status, message, dbType string) error {
	source = strings.TrimSpace(source)
	sourceID = strings.TrimSpace(sourceID)
	trackID = strings.TrimSpace(trackID)
	status = strings.TrimSpace(status)
	message = strings.TrimSpace(message)
	if source == "" || sourceID == "" {
		return nil
	}
	if status == "" {
		status = "imported"
	}
	importedAt := time.Now().Unix()

	if strings.ToLower(dbType) == "clickhouse" {
		existsStmt := "SELECT 1 FROM import_history WHERE source = ? AND source_id = ? LIMIT 1"
		var exists int
		if err := db.DB.QueryRowContext(ctx, existsStmt, source, sourceID).Scan(&exists); err == nil {
			return nil
		}
		stmt := "INSERT INTO import_history (source, source_id, track_id, status, imported_at, message) VALUES (?, ?, ?, ?, ?, ?)"
		if _, err := db.DB.ExecContext(ctx, stmt, source, sourceID, trackID, status, importedAt, message); err != nil {
			return fmt.Errorf("insert import history: %w", err)
		}
		return nil
	}

	insertSource := placeholder(dbType, 1)
	insertSourceID := placeholder(dbType, 2)
	insertTrackID := placeholder(dbType, 3)
	insertStatus := placeholder(dbType, 4)
	insertImported := placeholder(dbType, 5)
	insertMessage := placeholder(dbType, 6)
	existsSource := placeholder(dbType, 7)
	existsSourceID := placeholder(dbType, 8)
	stmt := fmt.Sprintf(`INSERT INTO import_history (source, source_id, track_id, status, imported_at, message)
SELECT %s, %s, %s, %s, %s, %s
WHERE NOT EXISTS (SELECT 1 FROM import_history WHERE source = %s AND source_id = %s);`, insertSource, insertSourceID, insertTrackID, insertStatus, insertImported, insertMessage, existsSource, existsSourceID)
	if _, err := db.DB.ExecContext(ctx, stmt, source, sourceID, trackID, status, importedAt, message, source, sourceID); err != nil {
		return fmt.Errorf("insert import history: %w", err)
	}
	return nil
}
