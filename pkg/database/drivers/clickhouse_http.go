package drivers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// clickhouseDriver wires the built-in HTTP interface of ClickHouse into database/sql.
// Implementing our own driver keeps dependencies minimal while following the Go
// proverb "A little copying is better than a little dependency".
type clickhouseDriver struct{}

// clickHouseConfig stores parsed DSN components so each request can reuse them.
type clickHouseConfig struct {
	scheme   string
	host     string
	database string
	username string
	password string
	params   url.Values
}

// clickhouseConn implements driver.Conn together with the optional query/exec interfaces.
type clickhouseConn struct {
	cfg    clickHouseConfig
	client *http.Client
}

// clickhouseStmt keeps a prepared statement string around. We expand placeholders
// manually because the HTTP endpoint does not support native bound parameters.
type clickhouseStmt struct {
	conn  *clickhouseConn
	query string
	num   int
}

// clickhouseRows materialises JSON responses so the sql package can iterate row by row.
type clickhouseRows struct {
	columns []string
	data    [][]driver.Value
	index   int
}

// init registers the driver with database/sql the moment the package is imported.
func init() {
	sql.Register("clickhouse", &clickhouseDriver{})
}

// Open parses the DSN and initialises an HTTP client.
func (d *clickhouseDriver) Open(name string) (driver.Conn, error) {
	cfg, err := parseClickHouseDSN(name)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	return &clickhouseConn{cfg: cfg, client: client}, nil
}

// Prepare builds a lightweight statement wrapper.
func (c *clickhouseConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

// PrepareContext counts placeholders so database/sql can report NumInput correctly.
func (c *clickhouseConn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	return &clickhouseStmt{conn: c, query: query, num: strings.Count(query, "?")}, nil
}

// Close releases no resources but fulfils the interface contract.
func (c *clickhouseConn) Close() error { return nil }

// Begin is unsupported because ClickHouse autocommits each statement.
func (c *clickhouseConn) Begin() (driver.Tx, error) {
	return nil, errors.New("clickhouse: transactions are not supported")
}

// BeginTx reports the same limitation through the context-aware interface.
func (c *clickhouseConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return nil, errors.New("clickhouse: transactions are not supported")
}

// Exec executes a statement outside prepared contexts.
func (c *clickhouseConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	return c.ExecContext(context.Background(), query, namedValuesFromValues(args))
}

// Query executes a query outside prepared contexts.
func (c *clickhouseConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return c.QueryContext(context.Background(), query, namedValuesFromValues(args))
}

// Ping verifies the HTTP endpoint is reachable.
func (c *clickhouseConn) Ping(ctx context.Context) error {
	req, err := c.newRequest(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("clickhouse: ping failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// CheckNamedValue coerces numeric types into the forms accepted by our encoder.
func (c *clickhouseConn) CheckNamedValue(nv *driver.NamedValue) error {
	switch v := nv.Value.(type) {
	case nil, int64, float64, bool, string, []byte, time.Time:
		return nil
	case int:
		nv.Value = int64(v)
		return nil
	case int32:
		nv.Value = int64(v)
		return nil
	case int16:
		nv.Value = int64(v)
		return nil
	case int8:
		nv.Value = int64(v)
		return nil
	case uint:
		nv.Value = int64(v)
		return nil
	case uint64:
		if v > math.MaxInt64 {
			return fmt.Errorf("clickhouse: uint64 %d overflows int64", v)
		}
		nv.Value = int64(v)
		return nil
	case uint32:
		nv.Value = int64(v)
		return nil
	case uint16:
		nv.Value = int64(v)
		return nil
	case uint8:
		nv.Value = int64(v)
		return nil
	case float32:
		nv.Value = float64(v)
		return nil
	default:
		return fmt.Errorf("clickhouse: unsupported value type %T", nv.Value)
	}
}

// ExecContext runs INSERT/DDL statements.
func (c *clickhouseConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	rendered, err := renderQuery(query, args)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, rendered)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("clickhouse: exec failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	io.Copy(io.Discard, resp.Body)
	return driver.RowsAffected(0), nil
}

// QueryContext runs SELECT statements and decodes the JSON response.
func (c *clickhouseConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rendered, err := renderQuery(query, args)
	if err != nil {
		return nil, err
	}
	formatted := ensureJSONFormat(rendered)
	req, err := c.newRequest(ctx, formatted)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("clickhouse: query failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	payload, err := decodeJSONResult(resp.Body)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// newRequest builds the POST request with headers and query parameters.
func (c *clickhouseConn) newRequest(ctx context.Context, query string) (*http.Request, error) {
	params := url.Values{}
	for k, vals := range c.cfg.params {
		for _, v := range vals {
			params.Add(k, v)
		}
	}
	if c.cfg.database != "" {
		params.Set("database", c.cfg.database)
	}
	endpoint := url.URL{Scheme: c.cfg.scheme, Host: c.cfg.host, Path: "/", RawQuery: params.Encode()}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if c.cfg.username != "" {
		req.SetBasicAuth(c.cfg.username, c.cfg.password)
	}
	return req, nil
}

// NumInput reports how many placeholders a prepared statement expects.
func (s *clickhouseStmt) NumInput() int { return s.num }

// Close releases no resources.
func (s *clickhouseStmt) Close() error { return nil }

// Exec executes the statement with the supplied args.
func (s *clickhouseStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.ExecContext(context.Background(), s.query, namedValuesFromValues(args))
}

// Query runs the statement with the supplied args.
func (s *clickhouseStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.QueryContext(context.Background(), s.query, namedValuesFromValues(args))
}

// ExecContext enables context-aware execution for prepared statements.
func (s *clickhouseStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

// QueryContext enables context-aware querying for prepared statements.
func (s *clickhouseStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

// Columns returns a copy to keep the sql package from mutating internal state.
func (r *clickhouseRows) Columns() []string {
	out := make([]string, len(r.columns))
	copy(out, r.columns)
	return out
}

// Close releases buffers early so garbage collection can reclaim memory promptly.
func (r *clickhouseRows) Close() error {
	r.data = nil
	return nil
}

// Next populates the destination slice with the next row of data.
func (r *clickhouseRows) Next(dest []driver.Value) error {
	if r.index >= len(r.data) {
		return io.EOF
	}
	row := r.data[r.index]
	if len(dest) < len(row) {
		return fmt.Errorf("clickhouse: destination has %d slots, need %d", len(dest), len(row))
	}
	for i := range row {
		dest[i] = row[i]
	}
	r.index++
	return nil
}

// namedValuesFromValues adapts plain driver.Values into NamedValue form.
func namedValuesFromValues(values []driver.Value) []driver.NamedValue {
	named := make([]driver.NamedValue, len(values))
	for i, v := range values {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return named
}

// renderQuery substitutes positional placeholders with quoted literals.
func renderQuery(query string, args []driver.NamedValue) (string, error) {
	if len(args) == 0 {
		return query, nil
	}
	var sb strings.Builder
	argIndex := 0
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '?' {
			if argIndex >= len(args) {
				return "", fmt.Errorf("clickhouse: not enough arguments for placeholders")
			}
			literal, err := encodeLiteral(args[argIndex].Value)
			if err != nil {
				return "", err
			}
			sb.WriteString(literal)
			argIndex++
			continue
		}
		sb.WriteByte(ch)
	}
	if argIndex != len(args) {
		return "", fmt.Errorf("clickhouse: too many arguments (expected %d, got %d)", argIndex, len(args))
	}
	return sb.String(), nil
}

// encodeLiteral converts Go values into ClickHouse-friendly SQL literals.
func encodeLiteral(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "NULL", nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), nil
	case bool:
		if val {
			return "1", nil
		}
		return "0", nil
	case string:
		escaped := strings.ReplaceAll(val, "'", "''")
		return "'" + escaped + "'", nil
	case []byte:
		if len(val) == 0 {
			return "''", nil
		}
		return "unhex('" + fmt.Sprintf("%x", val) + "')", nil
	case time.Time:
		ts := val.UTC().Format("2006-01-02 15:04:05")
		return "'" + ts + "'", nil
	default:
		return "", fmt.Errorf("clickhouse: unsupported literal type %T", v)
	}
}

// ensureJSONFormat appends a FORMAT clause so ClickHouse returns JSON metadata.
func ensureJSONFormat(query string) string {
	trimmed := strings.TrimSpace(query)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") && !strings.HasPrefix(lower, "show") {
		return query
	}
	if strings.Contains(lower, " format ") {
		return query
	}
	if strings.HasSuffix(trimmed, ";") {
		trimmed = strings.TrimSuffix(trimmed, ";")
		return trimmed + " FORMAT JSONCompactEachRowWithNamesAndTypes;"
	}
	return query + " FORMAT JSONCompactEachRowWithNamesAndTypes"
}

// parseClickHouseDSN handles clickhouse:// and http(s):// DSNs uniformly.
func parseClickHouseDSN(dsn string) (clickHouseConfig, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return clickHouseConfig{}, fmt.Errorf("clickhouse: invalid DSN: %w", err)
	}

	cfg := clickHouseConfig{params: url.Values{}}
	switch strings.ToLower(u.Scheme) {
	case "clickhouse", "http":
		cfg.scheme = "http"
	case "https":
		cfg.scheme = "https"
	case "":
		cfg.scheme = "http"
	default:
		return clickHouseConfig{}, fmt.Errorf("clickhouse: unsupported scheme %q", u.Scheme)
	}

	if host := strings.TrimSpace(u.Host); host != "" {
		cfg.host = host
	} else {
		cfg.host = "127.0.0.1:9000"
	}

	if u.User != nil {
		cfg.username = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			cfg.password = pw
		}
	}

	db := strings.TrimPrefix(u.Path, "/")
	cfg.database = db

	q := u.Query()
	if strings.EqualFold(q.Get("secure"), "true") {
		cfg.scheme = "https"
		q.Del("secure")
	}
	cfg.params = q

	return cfg, nil
}

// decodeJSONResult converts the ClickHouse JSON payload into clickhouseRows.
func decodeJSONResult(r io.Reader) (*clickhouseRows, error) {
	var payload struct {
		Meta []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"meta"`
		Data [][]any `json:"data"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, fmt.Errorf("clickhouse: decode json: %w", err)
	}
	cols := make([]string, len(payload.Meta))
	types := make([]string, len(payload.Meta))
	for i, meta := range payload.Meta {
		cols[i] = meta.Name
		types[i] = meta.Type
	}
	rows := make([][]driver.Value, 0, len(payload.Data))
	for _, raw := range payload.Data {
		if len(raw) != len(types) {
			return nil, fmt.Errorf("clickhouse: column mismatch: got %d values, expected %d", len(raw), len(types))
		}
		converted := make([]driver.Value, len(raw))
		for i, val := range raw {
			v, err := convertJSONValue(types[i], val)
			if err != nil {
				return nil, err
			}
			converted[i] = v
		}
		rows = append(rows, converted)
	}
	return &clickhouseRows{columns: cols, data: rows}, nil
}

// convertJSONValue maps ClickHouse JSON types onto database/sql driver values.
func convertJSONValue(meta string, raw any) (driver.Value, error) {
	if raw == nil {
		return nil, nil
	}
	lower := strings.ToLower(meta)
	switch {
	case strings.Contains(lower, "float"):
		switch v := raw.(type) {
		case float64:
			return v, nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, err
			}
			return f, nil
		}
	case strings.Contains(lower, "uint") || strings.Contains(lower, "int"):
		switch v := raw.(type) {
		case float64:
			return int64(v), nil
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, err
			}
			return i, nil
		}
	case strings.Contains(lower, "bool"):
		switch v := raw.(type) {
		case bool:
			if v {
				return int64(1), nil
			}
			return int64(0), nil
		case float64:
			return int64(v), nil
		}
	case strings.Contains(lower, "date"):
		if s, ok := raw.(string); ok {
			layouts := []string{"2006-01-02 15:04:05", "2006-01-02"}
			for _, layout := range layouts {
				if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
					return t, nil
				}
			}
			return s, nil
		}
	case strings.Contains(lower, "string") || strings.Contains(lower, "uuid") || strings.Contains(lower, "enum"):
		switch v := raw.(type) {
		case string:
			return v, nil
		default:
			return fmt.Sprintf("%v", v), nil
		}
	}
	switch v := raw.(type) {
	case string:
		return v, nil
	case float64:
		return v, nil
	case bool:
		if v {
			return int64(1), nil
		}
		return int64(0), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
