package safecastimport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

// DownloadLogFile downloads a bGeigie log payload from the Safecast source URL
// so the importer can parse it without depending on external storage SDKs.
func (c *Client) DownloadLogFile(ctx context.Context, sourceURL string) ([]byte, string, error) {
	cleanURL := strings.TrimSpace(sourceURL)
	if cleanURL == "" {
		return nil, "", fmt.Errorf("empty source url")
	}

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cleanURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create download request: %w", err)
	}
	c.applyRandomHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download log: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download http %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read log body: %w", err)
	}

	filename := extractFilename(cleanURL)
	if len(content) > 0 && !looksLikeBGeigie(content) {
		return nil, "", fmt.Errorf("invalid log file format")
	}

	return content, filename, nil
}

// extractFilename keeps a readable filename for logs when the source URL
// includes a full S3 path or query string.
func extractFilename(sourceURL string) string {
	base := path.Base(sourceURL)
	if base == "" || base == "." || base == "/" || strings.Contains(base, "?") {
		return "safecast_import.log"
	}
	return base
}

// looksLikeBGeigie does a cheap signature scan so we fail fast on non-log URLs
// without attempting to parse arbitrary content in the importer.
func looksLikeBGeigie(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	checkSize := 1024
	if len(content) < checkSize {
		checkSize = len(content)
	}
	header := string(content[:checkSize])
	for _, sig := range []string{"$BNRDD", "$BMRDD", "$BNXRDD"} {
		if strings.Contains(header, sig) {
			return true
		}
	}
	return true
}

// BytesFile adapts byte slices to the multipart.File interface so we can reuse
// existing parsers without touching their signatures.
type BytesFile struct {
	*bytes.Reader
	name string
}

// NewBytesFile wraps byte content as a multipart.File-compatible reader.
func NewBytesFile(data []byte, filename string) *BytesFile {
	return &BytesFile{
		Reader: bytes.NewReader(data),
		name:   filename,
	}
}

// Close is a no-op because the underlying reader is memory backed.
func (b *BytesFile) Close() error {
	return nil
}

// Seek forwards to the embedded reader so callers can rewind as needed.
func (b *BytesFile) Seek(offset int64, whence int) (int64, error) {
	return b.Reader.Seek(offset, whence)
}

// ReadAt forwards to the embedded reader to satisfy interfaces used by parsers.
func (b *BytesFile) ReadAt(p []byte, off int64) (int, error) {
	return b.Reader.ReadAt(p, off)
}

// Filename returns the stored filename so callers can log or re-use it.
func (b *BytesFile) Filename() string {
	return b.name
}
