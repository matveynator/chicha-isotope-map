package selfupgrade

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileDatabaseController works for file-backed engines (SQLite, DuckDB, Chai).
// We duplicate the database file for backups and clones because these engines
// represent the entire dataset inside a single file.
type FileDatabaseController struct {
	Driver       string
	OriginalPath string
	BackupsDir   string
	Logf         func(string, ...any)
}

// Backup copies the live database into the backups directory. The resulting
// filename embeds the version and timestamp so operators can correlate artifacts
// with releases.
func (c FileDatabaseController) Backup(ctx context.Context, version string) (string, error) {
	if strings.TrimSpace(c.OriginalPath) == "" {
		return "", fmt.Errorf("selfupgrade: original database path is empty")
	}
	if err := ensureDir(c.BackupsDir); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s.bak", time.Now().UTC().Format("20060102-150405"), sanitize(version))
	dest := filepath.Join(c.BackupsDir, name)
	checksum, err := copyFileWithHash(ctx, c.OriginalPath, dest)
	if err != nil {
		return "", err
	}
	if c.Logf != nil {
		c.Logf("[selfupgrade] DB backup created: %s (sha256=%s)", dest, checksum)
	}
	return dest, nil
}

// Clone duplicates the production database into a temporary file so canary
// instances can mutate schema/data without touching production.
func (c FileDatabaseController) Clone(ctx context.Context, version string) (CloneInfo, error) {
	if strings.TrimSpace(c.OriginalPath) == "" {
		return CloneInfo{}, fmt.Errorf("selfupgrade: original database path is empty")
	}
	if err := ensureDir(c.BackupsDir); err != nil {
		return CloneInfo{}, err
	}
	name := fmt.Sprintf("clone-%s-%s.db", sanitize(version), time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(c.BackupsDir, name)
	if _, err := copyFileWithHash(ctx, c.OriginalPath, path); err != nil {
		return CloneInfo{}, err
	}

	if err := c.ping(ctx, path); err != nil {
		return CloneInfo{}, err
	}
	if c.Logf != nil {
		c.Logf("[selfupgrade] DB clone ready: %s", path)
	}
	return CloneInfo{Name: name, DSN: path, Path: path}, nil
}

// DropClone removes the temporary database file once the rollout concludes.
func (c FileDatabaseController) DropClone(_ context.Context, clone CloneInfo) error {
	if strings.TrimSpace(clone.Path) == "" {
		return nil
	}
	if err := os.Remove(clone.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if c.Logf != nil {
		c.Logf("[selfupgrade] DB clone removed: %s", clone.Path)
	}
	return nil
}

// RestoreBackup replaces the live database with the provided backup.
func (c FileDatabaseController) RestoreBackup(ctx context.Context, backupPath string) error {
	if strings.TrimSpace(backupPath) == "" {
		return fmt.Errorf("selfupgrade: backup path is empty")
	}
	temp := c.OriginalPath + ".rollback"
	if _, err := copyFileWithHash(ctx, backupPath, temp); err != nil {
		return err
	}
	if err := os.Rename(temp, c.OriginalPath); err != nil {
		return err
	}
	if c.Logf != nil {
		c.Logf("[selfupgrade] DB restored from %s", backupPath)
	}
	return nil
}

func (c FileDatabaseController) ping(ctx context.Context, dsn string) error {
	if strings.TrimSpace(c.Driver) == "" {
		return fmt.Errorf("selfupgrade: database driver is empty")
	}
	db, err := sql.Open(c.Driver, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func sanitize(input string) string {
	cleaned := strings.TrimSpace(input)
	if cleaned == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return replacer.Replace(cleaned)
}
