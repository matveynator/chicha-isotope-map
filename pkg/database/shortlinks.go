package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// =========================
// Short link storage helpers
// =========================

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const defaultShortCodeLength = 8

// PreviewShortLink fetches an existing mapping or proposes a fresh random code.
// We prefer explicit control flow over clever caching so operators can reason
// about the behaviour — echoing the proverb "Clear is better than clever."
// The helper never writes to the database; it simply avoids duplicates by
// probing the current table and respects context cancellation so callers can
// bail out early if the user navigates away.
func (db *Database) PreviewShortLink(ctx context.Context, target string, length int) (code string, stored bool, err error) {
	if db == nil || db.DB == nil {
		return "", false, errors.New("database not initialized")
	}
	cleaned := strings.TrimSpace(target)
	if cleaned == "" {
		return "", false, errors.New("empty target")
	}
	if len(cleaned) > 4096 {
		return "", false, errors.New("target too long")
	}

	if existing, err := db.lookupShortLinkByTarget(ctx, cleaned); err != nil {
		return "", false, err
	} else if existing != "" {
		return existing, true, nil
	}

	code, err = db.randomUnusedCode(ctx, length)
	if err != nil {
		return "", false, err
	}
	return code, false, nil
}

// PersistShortLink inserts the provided mapping if it does not exist yet. We
// accept an optional pre-selected code so the UI can reserve a string for the
// user before they confirm copying, matching the "share memory by
// communicating" proverb — the browser communicates the reservation back
// instead of reaching for shared globals.
func (db *Database) PersistShortLink(ctx context.Context, target, code string, now time.Time, length int) (string, error) {
	if db == nil || db.DB == nil {
		return "", errors.New("database not initialized")
	}
	cleanedTarget := strings.TrimSpace(target)
	if cleanedTarget == "" {
		return "", errors.New("empty target")
	}
	if len(cleanedTarget) > 4096 {
		return "", errors.New("target too long")
	}

	if existing, err := db.lookupShortLinkByTarget(ctx, cleanedTarget); err != nil {
		return "", err
	} else if existing != "" {
		return existing, nil
	}

	trimmedCode := strings.TrimSpace(code)
	if trimmedCode != "" {
		if !isBase62(trimmedCode) {
			return "", errors.New("invalid code")
		}
	}

	const maxAttempts = 64
	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		candidate := trimmedCode
		if candidate == "" {
			generated, err := db.randomUnusedCode(ctx, length)
			if err != nil {
				return "", err
			}
			candidate = generated
		} else {
			exists, err := db.shortCodeExists(ctx, candidate)
			if err != nil {
				return "", err
			}
			if exists {
				trimmedCode = ""
				continue
			}
		}

		if err := db.insertShortLink(ctx, candidate, cleanedTarget, now); err != nil {
			if isUniqueConstraintError(err) {
				trimmedCode = ""
				continue
			}
			return "", err
		}
		return candidate, nil
	}
	return "", fmt.Errorf("persist short link: exhausted %d attempts", maxAttempts)
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
	err := db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		return conn.QueryRowContext(ctx, query, arg).Scan(&target)
	})
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
	err := db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		return conn.QueryRowContext(ctx, query, arg).Scan(&code)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return code, nil
}

// shortCodeExists reports whether a specific code is already in use. We keep the
// logic simple — a single SELECT with LIMIT — so databases can leverage indexes
// efficiently even at scale.
func (db *Database) shortCodeExists(ctx context.Context, code string) (bool, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return false, nil
	}

	query := ""
	var arg any
	switch strings.ToLower(db.Driver) {
	case "pgx", "duckdb":
		query = "SELECT 1 FROM short_links WHERE code = $1 LIMIT 1"
		arg = trimmed
	default:
		query = "SELECT 1 FROM short_links WHERE code = ? LIMIT 1"
		arg = trimmed
	}

	var dummy int
	err := db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		return conn.QueryRowContext(ctx, query, arg).Scan(&dummy)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// randomUnusedCode draws random characters until it finds a code that is not
// present in the database. Rejection sampling keeps the loop simple while still
// guaranteeing uniform distribution across the alphabet.
func (db *Database) randomUnusedCode(ctx context.Context, length int) (string, error) {
	if length <= 0 {
		length = defaultShortCodeLength
	}
	const maxAttempts = 64
	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		candidate, err := randomBase62String(length)
		if err != nil {
			return "", err
		}
		exists, err := db.shortCodeExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("random code generation exhausted %d attempts", maxAttempts)
}

// randomBase62String draws secure random bytes and maps them to the base62
// alphabet. We avoid math/rand so short links remain unpredictable.
func randomBase62String(length int) (string, error) {
	if length <= 0 {
		length = defaultShortCodeLength
	}
	buf := make([]byte, length)
	for i := 0; i < length; i++ {
		var b [1]byte
		for {
			if _, err := rand.Read(b[:]); err != nil {
				return "", err
			}
			v := int(b[0])
			if v < 62*4 { // 248 keeps rejection rate low while staying uniform.
				buf[i] = base62Alphabet[v%62]
				break
			}
		}
	}
	return string(buf), nil
}

// isBase62 validates that the provided code only contains characters from our
// alphabet. Keeping validation local avoids duplicating logic across callers.
func isBase62(code string) bool {
	if code == "" {
		return false
	}
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		default:
			return false
		}
	}
	return true
}

// insertShortLink writes the mapping for code → target into the database.
func (db *Database) insertShortLink(ctx context.Context, code, target string, now time.Time) error {
	return db.withSerializedConnectionFor(ctx, WorkloadUserUpload, func(ctx context.Context, conn *sql.DB) error {
		switch strings.ToLower(db.Driver) {
		case "pgx":
			_, err := conn.ExecContext(ctx,
				"INSERT INTO short_links (code,target,created_at) VALUES ($1,$2,$3)",
				code, target, now.UTC())
			return err
		case "duckdb":
			_, err := conn.ExecContext(ctx,
				"INSERT INTO short_links (code,target,created_at) VALUES ($1,$2,$3)",
				code, target, now.UTC())
			return err
		default:
			_, err := conn.ExecContext(ctx,
				"INSERT INTO short_links (code,target,created_at) VALUES (?,?,?)",
				code, target, now.Unix())
			return err
		}
	})
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
