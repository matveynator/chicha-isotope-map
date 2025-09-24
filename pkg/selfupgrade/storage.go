package selfupgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ensureDir creates the directory tree when missing. We treat existing
// directories as success so repeated calls remain idempotent.
func ensureDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("selfupgrade: directory path is empty")
	}
	return os.MkdirAll(path, 0o755)
}

// copyFileWithHash streams src into dst while computing the SHA-256 checksum.
// Callers can reuse the checksum for manifest files without rescanning the
// filesystem.
func copyFileWithHash(ctx context.Context, src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	if err := ensureDir(filepath.Dir(dst)); err != nil {
		return "", err
	}

	info, err := in.Stat()
	if err != nil {
		return "", err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return "", err
	}
	defer out.Close()

	hash := sha256.New()
	writer := io.MultiWriter(out, hash)
	if _, err := io.Copy(writer, withContextReader(ctx, in)); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// downloadToFile streams the HTTP response body into dest while hashing it.
func downloadToFile(ctx context.Context, client *http.Client, url, dest string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return "", fmt.Errorf("selfupgrade: download %s failed with %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := ensureDir(filepath.Dir(dest)); err != nil {
		return "", err
	}

	// We stream into a temporary file so interrupted downloads never corrupt
	// the last good artefact. The suffix mixes in time to keep retries unique.
	tmp := fmt.Sprintf("%s.partial-%d", dest, time.Now().UnixNano())
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	writer := io.MultiWriter(out, hash)
	if _, err := io.Copy(writer, withContextReader(ctx, resp.Body)); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}

	checksum := hex.EncodeToString(hash.Sum(nil))
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}

	return checksum, nil
}

// hashFile returns the SHA-256 of the provided file so callers can reuse cached
// artefacts without downloading them again.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// loadCachedChecksum inspects the neighbouring checksum file and confirms the
// binary content still matches. We silently ignore missing artefacts so callers
// can fall back to a fresh download.
func loadCachedChecksum(path string) (string, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}

	data, err := os.ReadFile(path + ".sha256")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	checksum := strings.TrimSpace(string(data))
	if checksum == "" {
		return "", false, nil
	}
	actual, err := hashFile(path)
	if err != nil {
		return "", false, err
	}
	if actual != checksum {
		return "", false, nil
	}
	return checksum, true, nil
}

// writeChecksumFile places the checksum next to a binary so humans can verify
// integrity before promoting it.
func writeChecksumFile(path, checksum string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("selfupgrade: checksum path is empty")
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(checksum)+"\n"), 0o644)
}

// withContextReader aborts long copies when the context is cancelled.
func withContextReader(ctx context.Context, r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		buf := make([]byte, 32<<10)
		for {
			select {
			case <-ctx.Done():
				pw.CloseWithError(ctx.Err())
				return
			default:
			}
			n, err := r.Read(buf)
			if n > 0 {
				if _, werr := pw.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					pw.CloseWithError(err)
				}
				return
			}
		}
	}()
	return pr
}
