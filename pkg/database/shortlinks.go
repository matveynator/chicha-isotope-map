package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// =========================
// Short link storage helpers
// =========================

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// SaveShortLink stores or retrieves a short code for the provided target URL.
// The design leans on channels instead of mutexes so we remain faithful to
// Go's proverb "Don't communicate by sharing memory; share memory by
// communicating." The generator goroutine already feeds idGenerator, therefore
// we multiplex that channel inside a select so context cancellation works.
func (db *Database) SaveShortLink(ctx context.Context, target string, now time.Time) (string, error) {
	if db == nil || db.DB == nil {
		return "", errors.New("database not initialized")
	}
	cleaned := strings.TrimSpace(target)
	if cleaned == "" {
		return "", errors.New("empty target")
	}
	if len(cleaned) > 4096 {
		return "", errors.New("target too long")
	}

	// Fast path: existing mapping
	if existing, err := db.lookupShortLinkByTarget(ctx, cleaned); err != nil {
		return "", err
	} else if existing != "" {
		return existing, nil
	}

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case id := <-db.idGenerator:
			if id <= 0 {
				continue
			}
			code := encodeBase62(id)
			if err := db.insertShortLink(ctx, code, cleaned, now); err != nil {
				if isUniqueConstraintError(err) {
					// Another request might have inserted the target concurrently.
					if existing, lookupErr := db.lookupShortLinkByTarget(ctx, cleaned); lookupErr == nil && existing != "" {
						return existing, nil
					}
					continue
				}
				return "", err
			}
			return code, nil
		}
	}
}

// ResolveShortLink expands a short code into the stored absolute URL.
func (db *Database) ResolveShortLink(ctx context.Context, code string) (string, error) {
	if db == nil || db.DB == nil {
		return "", errors.New("database not initialized")
	}
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return "", nil
	}

	query := ""
	var arg any
	switch strings.ToLower(db.Driver) {
	case "pgx", "duckdb":
		query = "SELECT target FROM short_links WHERE code = $1 LIMIT 1"
		arg = trimmed
	default:
		query = "SELECT target FROM short_links WHERE code = ? LIMIT 1"
		arg = trimmed
	}

	var target string
	err := db.DB.QueryRowContext(ctx, query, arg).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return target, nil
}

// lookupShortLinkByTarget checks whether a mapping already exists for target.
func (db *Database) lookupShortLinkByTarget(ctx context.Context, target string) (string, error) {
	query := ""
	arg := any(target)
	switch strings.ToLower(db.Driver) {
	case "pgx", "duckdb":
		query = "SELECT code FROM short_links WHERE target = $1 ORDER BY id LIMIT 1"
	default:
		query = "SELECT code FROM short_links WHERE target = ? ORDER BY id LIMIT 1"
	}
	var code string
	err := db.DB.QueryRowContext(ctx, query, arg).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return code, nil
}

// insertShortLink writes the mapping for code → target into the database.
func (db *Database) insertShortLink(ctx context.Context, code, target string, now time.Time) error {
	switch strings.ToLower(db.Driver) {
	case "pgx":
		_, err := db.DB.ExecContext(ctx,
			"INSERT INTO short_links (code,target,created_at) VALUES ($1,$2,$3)",
			code, target, now.UTC())
		return err
	case "duckdb":
		_, err := db.DB.ExecContext(ctx,
			"INSERT INTO short_links (code,target,created_at) VALUES ($1,$2,$3)",
			code, target, now.UTC())
		return err
	default:
		_, err := db.DB.ExecContext(ctx,
			"INSERT INTO short_links (code,target,created_at) VALUES (?,?,?)",
			code, target, now.Unix())
		return err
	}
}

// encodeBase62 renders an integer into a compact alphanumeric string.
func encodeBase62(n int64) string {
	if n <= 0 {
		return "0"
	}
	var buf [11]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = base62Alphabet[n%62]
		n /= 62
	}
	return string(buf[i:])
}

// isUniqueConstraintError normalizes driver-specific duplicate errors.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "constraint failed") ||
		strings.Contains(msg, "unique violation")
}
