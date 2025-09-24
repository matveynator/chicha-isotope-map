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

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	defer out.Close()

	hash := sha256.New()
	writer := io.MultiWriter(out, hash)
	if _, err := io.Copy(writer, withContextReader(ctx, resp.Body)); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
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
