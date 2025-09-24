package autodeploy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// =====================
// Filesystem primitives
// =====================
// fileChecksum streams the file so large binaries do not require buffering the
// entire blob in memory. We return a hex-encoded string to keep logging human
// readable and to align with the GitHub checksum format.
func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("autodeploy: open %s: %w", path, err)
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", fmt.Errorf("autodeploy: checksum copy: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// copyFile keeps the implementation streaming friendly so slow disks do not
// block the autodeploy loop for long. We ensure the destination is durable by
// syncing before returning.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("autodeploy: copy open src: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("autodeploy: copy mkdir: %w", err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("autodeploy: copy create dst: %w", err)
	}
	defer func() {
		out.Close()
		if err != nil {
			os.Remove(dst)
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("autodeploy: copy stream: %w", err)
	}
	if err = out.Sync(); err != nil {
		return fmt.Errorf("autodeploy: copy sync: %w", err)
	}
	return nil
}

// sanitize removes characters that make filesystem names awkward. We keep only
// ASCII alphanumerics plus dash, underscore, and dot so timestamped filenames
// remain predictable.
func sanitize(in string) string {
	var b strings.Builder
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "artifact"
	}
	return b.String()
}

// ensureDir creates the directory if missing while normalising the path for
// future joins. We fail fast so operators spot permission issues early.
func ensureDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("autodeploy: empty directory path")
	}
	cleaned := filepath.Clean(path)
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return "", fmt.Errorf("autodeploy: mkdir %s: %w", cleaned, err)
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("autodeploy: abs %s: %w", cleaned, err)
	}
	return abs, nil
}

// stateData persists across restarts so we do not redownload the same release.
type stateData struct {
	LastETag     string    `json:"last_etag"`
	LastChecksum string    `json:"last_checksum"`
	LastApplied  time.Time `json:"last_applied"`
}

func loadState(path string) (stateData, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return stateData{}, nil
	}
	if err != nil {
		return stateData{}, fmt.Errorf("autodeploy: state read: %w", err)
	}
	var st stateData
	if err := json.Unmarshal(data, &st); err != nil {
		return stateData{}, fmt.Errorf("autodeploy: state decode: %w", err)
	}
	return st, nil
}

func saveState(path string, st stateData) error {
	tmp := path + ".tmp"
	payload, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("autodeploy: state encode: %w", err)
	}
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return fmt.Errorf("autodeploy: state write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("autodeploy: state rename: %w", err)
	}
	return nil
}

// pendingRecord keeps the minimal data we need to resume pending sessions on
// restart.
type pendingRecord struct {
	Token        string    `json:"token"`
	Version      string    `json:"version"`
	Checksum     string    `json:"checksum"`
	ETag         string    `json:"etag"`
	BinaryPath   string    `json:"binary_path"`
	ClonePath    string    `json:"clone_path"`
	BackupBinary string    `json:"backup_binary"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

func pendingFile(dir, token string) string {
	return filepath.Join(dir, sanitize(token)+".json")
}

func savePending(dir string, rec pendingRecord) error {
	if strings.TrimSpace(rec.Token) == "" {
		return errors.New("autodeploy: pending token required")
	}
	tmp := pendingFile(dir, rec.Token) + ".tmp"
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("autodeploy: pending encode: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("autodeploy: pending write tmp: %w", err)
	}
	if err := os.Rename(tmp, pendingFile(dir, rec.Token)); err != nil {
		return fmt.Errorf("autodeploy: pending rename: %w", err)
	}
	return nil
}

func loadPending(dir string) ([]pendingRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("autodeploy: pending list: %w", err)
	}
	records := make([]pendingRecord, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("autodeploy: pending read %s: %w", e.Name(), err)
		}
		var rec pendingRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("autodeploy: pending decode %s: %w", e.Name(), err)
		}
		if strings.TrimSpace(rec.Token) == "" {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

func deletePending(dir, token string) error {
	path := pendingFile(dir, token)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("autodeploy: pending delete: %w", err)
	}
	return nil
}

// randomToken builds a base62 token to embed in links. We use crypto/rand so
// guessed URLs remain impractical to brute force.
func randomToken(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if n <= 0 {
		n = 24
	}
	max := big.NewInt(int64(len(alphabet)))
	var b strings.Builder
	for i := 0; i < n; i++ {
		val, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("autodeploy: token rand: %w", err)
		}
		b.WriteByte(alphabet[val.Int64()])
	}
	return b.String(), nil
}
