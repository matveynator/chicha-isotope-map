package autodeploy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// fileChecksum streams the file to avoid loading large binaries into memory.
// Returning a hex string keeps logging simple and matches the manifest format.
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

// copyFile performs a streaming copy so slow disks do not block other goroutines
// for too long. We rely on io.Copy to keep the implementation straightforward.
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

// sanitize removes characters that do not play well with filesystem naming. We
// keep only alphanumerics, dash, underscore, and dot so timestamps and semantic
// versions survive unchanged.
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
