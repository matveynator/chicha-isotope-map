package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// =====================
// User identities
// =====================

// ResolveUserBySource returns the internal user ID associated with an external
// provider identifier so imports can connect tracks to future accounts.
func (db *Database) ResolveUserBySource(ctx context.Context, source, sourceUserID, dbType string) (string, string, error) {
	source = strings.TrimSpace(source)
	sourceUserID = strings.TrimSpace(sourceUserID)
	if source == "" || sourceUserID == "" {
		return "", "", nil
	}
	phSource := placeholder(dbType, 1)
	phSourceID := placeholder(dbType, 2)
	query := fmt.Sprintf(`SELECT user_id, name FROM users WHERE source = %s AND source_user_id = %s LIMIT 1`, phSource, phSourceID)
	var (
		userID string
		name   sql.NullString
	)
	if err := db.DB.QueryRowContext(ctx, query, source, sourceUserID).Scan(&userID, &name); err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil
		}
		return "", "", fmt.Errorf("resolve user: %w", err)
	}
	return userID, strings.TrimSpace(name.String), nil
}

// EnsureUserBySource returns a stable internal user ID and stores the display
// name when it is available, keeping the mapping ready for future account links.
func (db *Database) EnsureUserBySource(ctx context.Context, source, sourceUserID, name, dbType string) (string, error) {
	source = strings.TrimSpace(source)
	sourceUserID = strings.TrimSpace(sourceUserID)
	name = strings.TrimSpace(name)
	if source == "" || sourceUserID == "" {
		return "", nil
	}

	if existingID, _, err := db.ResolveUserBySource(ctx, source, sourceUserID, dbType); err != nil {
		return "", err
	} else if existingID != "" {
		if name != "" {
			if err := db.UpdateUserNameIfEmpty(ctx, existingID, name, dbType); err != nil {
				return "", err
			}
		}
		return existingID, nil
	}

	newID, err := newUserID()
	if err != nil {
		return "", err
	}
	createdAt := time.Now().Unix()

	if strings.ToLower(dbType) == "clickhouse" {
		stmt := "INSERT INTO users (user_id, source, source_user_id, name, created_at) VALUES (?, ?, ?, ?, ?)"
		if _, err := db.DB.ExecContext(ctx, stmt, newID, source, sourceUserID, name, createdAt); err != nil {
			return "", fmt.Errorf("insert user: %w", err)
		}
		return newID, nil
	}

	insertID := placeholder(dbType, 1)
	insertSource := placeholder(dbType, 2)
	insertSourceID := placeholder(dbType, 3)
	insertName := placeholder(dbType, 4)
	insertCreated := placeholder(dbType, 5)
	existsSource := placeholder(dbType, 6)
	existsSourceID := placeholder(dbType, 7)
	stmt := fmt.Sprintf(`INSERT INTO users (user_id, source, source_user_id, name, created_at)
SELECT %s, %s, %s, %s, %s
WHERE NOT EXISTS (SELECT 1 FROM users WHERE source = %s AND source_user_id = %s);`, insertID, insertSource, insertSourceID, insertName, insertCreated, existsSource, existsSourceID)
	if _, err := db.DB.ExecContext(ctx, stmt, newID, source, sourceUserID, name, createdAt, source, sourceUserID); err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}

	if userID, _, err := db.ResolveUserBySource(ctx, source, sourceUserID, dbType); err != nil {
		return "", err
	} else if userID != "" {
		return userID, nil
	}
	return newID, nil
}

// UpdateUserNameIfEmpty populates missing display names so later UI work can
// show a friendly label without rewriting non-empty data.
func (db *Database) UpdateUserNameIfEmpty(ctx context.Context, userID, name, dbType string) error {
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	if userID == "" || name == "" {
		return nil
	}
	if strings.ToLower(dbType) == "clickhouse" {
		return nil
	}
	phName := placeholder(dbType, 1)
	phUser := placeholder(dbType, 2)
	stmt := fmt.Sprintf(`UPDATE users SET name = %s WHERE user_id = %s AND (name IS NULL OR name = '')`, phName, phUser)
	if _, err := db.DB.ExecContext(ctx, stmt, name, userID); err != nil {
		return fmt.Errorf("update user name: %w", err)
	}
	return nil
}

// EnsureTrackUser records that a track belongs to a user so future account
// pages can filter by ownership without rescanning markers.
func (db *Database) EnsureTrackUser(ctx context.Context, trackID, userID, source, dbType string) error {
	trackID = strings.TrimSpace(trackID)
	userID = strings.TrimSpace(userID)
	source = strings.TrimSpace(source)
	if trackID == "" || userID == "" {
		return nil
	}
	if source == "" {
		source = "external"
	}

	if strings.ToLower(dbType) == "clickhouse" {
		existsStmt := "SELECT 1 FROM track_users WHERE track_id = ? AND user_id = ? LIMIT 1"
		var exists int
		if err := db.DB.QueryRowContext(ctx, existsStmt, trackID, userID).Scan(&exists); err == nil {
			return nil
		}
		insertStmt := "INSERT INTO track_users (track_id, user_id, source) VALUES (?, ?, ?)"
		if _, err := db.DB.ExecContext(ctx, insertStmt, trackID, userID, source); err != nil {
			return fmt.Errorf("insert track user: %w", err)
		}
		return nil
	}

	insertTrack := placeholder(dbType, 1)
	insertUser := placeholder(dbType, 2)
	insertSource := placeholder(dbType, 3)
	existsTrack := placeholder(dbType, 4)
	existsUser := placeholder(dbType, 5)
	stmt := fmt.Sprintf(`INSERT INTO track_users (track_id, user_id, source)
SELECT %s, %s, %s
WHERE NOT EXISTS (SELECT 1 FROM track_users WHERE track_id = %s AND user_id = %s);`, insertTrack, insertUser, insertSource, existsTrack, existsUser)
	if _, err := db.DB.ExecContext(ctx, stmt, trackID, userID, source, trackID, userID); err != nil {
		return fmt.Errorf("insert track user: %w", err)
	}
	return nil
}

// newUserID generates a random identifier so we can reference users even before
// they register with an email-backed account.
func newUserID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random user id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
