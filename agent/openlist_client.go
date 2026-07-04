package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenListClient is a small typed HTTP client over the OpenList
// REST API. The agent uses it both for "generic" tool calls
// (list_files, read_file, etc.) and as a thin shim around the
// alistencrypt plugin's /api/ext/* endpoints.
//
// All methods honour the agent's 30-second default timeout and
// surface a typed error (`*OpenListError`) so the agent's
// tool_result JSON can be machine-readable.
//
// Auth: every request includes the
// `Authorization: <token>` header (the OpenList convention is
// the raw token, NOT a "Bearer " prefix). The constant
// `OpenListAuthHeader` is the canonical header name.
const (
	OpenListAuthHeader = "Authorization"
)

// OpenListError is the typed error returned by every method on
// OpenListClient. It encodes both the HTTP status and the
// OpenList error body (if any) so the agent's tool_result
// payload can preserve them.
type OpenListError struct {
	Status int
	Method string
	Path   string
	Body   string
}

// Error implements the error interface.
func (e *OpenListError) Error() string {
	return fmt.Sprintf("openlist %s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// OpenListClient is a stateless HTTP wrapper. Each method
// creates its own request and returns the decoded response.
// Timeouts default to 30 seconds; pass a context with a shorter
// deadline to override.
type OpenListClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewOpenListClient builds a client with the supplied base URL
// (e.g. "http://127.0.0.1:5244") and token. The http.Client is
// the supplied one (defaults to http.DefaultClient with a 30s
// timeout if nil).
func NewOpenListClient(baseURL, token string, hc *http.Client) *OpenListClient {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &OpenListClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    hc,
	}
}

// ---- request envelope ----

// listFilesResponse mirrors OpenList's /api/admin/fs/list
// response. We only decode the fields the agent actually uses.
type listFilesResponse struct {
	Code    int  `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		Content []struct {
			Name     string `json:"name"`
			Size     int64  `json:"size"`
			IsDir    bool   `json:"is_dir"`
			Modified string `json:"modified"`
		} `json:"content"`
		Total int `json:"total"`
	} `json:"data"`
}

// fsItem mirrors one entry in the list_files response. The
// `is_dir` JSON tag has a hyphen, so the field must be a string
// map for unknown fields to round-trip; the typed version
// captures only the four fields above.
type fileItem struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"is_dir"`
	Modified string `json:"modified"`
}

// ListFilesResult is the agent-friendly result of ListFiles.
type ListFilesResult struct {
	Items []fileItem `json:"items"`
	Total int        `json:"total"`
}

// ListFiles calls OpenList's /api/admin/fs/list. The
// `path` argument is the absolute path on the OpenList
// instance.
func (c *OpenListClient) ListFiles(ctx context.Context, path string) (*ListFilesResult, error) {
	body, _ := json.Marshal(map[string]any{
		"path":     path,
		"page":     1,
		"per_page": 1000,
	})
	var resp listFilesResponse
	if err := c.doJSON(ctx, "POST", "/api/admin/fs/list", body, &resp); err != nil {
		return nil, err
	}
	out := &ListFilesResult{Total: resp.Data.Total}
	if resp.Data != nil {
		for _, it := range resp.Data.Content {
			out.Items = append(out.Items, fileItem{
				Name:     it.Name,
				Size:     it.Size,
				IsDir:    it.IsDir,
				Modified: it.Modified,
			})
		}
	}
	return out, nil
}

// ReadFile reads a file's contents. The return value is the
// raw bytes; callers that need text should string-ify.
func (c *OpenListClient) ReadFile(ctx context.Context, path string) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{"path": path})
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := c.doJSON(ctx, "POST", "/api/admin/fs/get", body, &resp); err != nil {
		return nil, err
	}
	return []byte(resp.Data), nil
}

// WriteFile uploads the supplied content to the given path.
// Existing files are overwritten. The boolean response is true
// when the write succeeded.
func (c *OpenListClient) WriteFile(ctx context.Context, path string, content []byte) (bool, error) {
	body, _ := json.Marshal(map[string]any{
		"path":    path,
		"data":    string(content),
		"flag":    "overwrite",
	})
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := c.doJSON(ctx, "POST", "/api/admin/fs/put", body, &resp); err != nil {
		return false, err
	}
	return resp.Code == 200, nil
}

// DeleteFile removes a path (file or empty directory). The
// boolean response is true on success.
func (c *OpenListClient) DeleteFile(ctx context.Context, path string) (bool, error) {
	body, _ := json.Marshal(map[string]any{"path": path})
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := c.doJSON(ctx, "POST", "/api/fs/remove", body, &resp); err != nil {
		return false, err
	}
	return resp.Code == 200, nil
}

// Rename moves/renames a file. The agent uses the
// OpenList-native naming where `src` is the source path and
// `dst` is the destination path.
func (c *OpenListClient) Rename(ctx context.Context, src, dst string) (bool, error) {
	body, _ := json.Marshal(map[string]any{
		"src": src,
		"dst": dst,
	})
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := c.doJSON(ctx, "POST", "/api/fs/rename", body, &resp); err != nil {
		return false, err
	}
	return resp.Code == 200, nil
}

// ExecCommand runs a server-side command via the OpenList
// "command runner" endpoint. The semantics depend on the
// server build (admin-only on most installs). The string
// response is the captured stdout.
func (c *OpenListClient) ExecCommand(ctx context.Context, command string) (string, error) {
	body, _ := json.Marshal(map[string]any{"command": command})
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := c.doJSON(ctx, "POST", "/api/admin/command", body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// GetStorageInfo returns the total / used / free bytes of the
// OpenList storage. OpenList's response shape varies across
// versions; we accept any of the common keys.
func (c *OpenListClient) GetStorageInfo(ctx context.Context) (map[string]any, error) {
	var resp struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := c.doJSON(ctx, "GET", "/api/admin/storage", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// storageListResponse mirrors OpenList's /api/admin/storage/list
// response. The `content` array carries one entry per mounted
// storage backend (e.g. Local / S3 / GoogleDrive / Onedrive).
type storageListResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		Content []storageMount `json:"content"`
		Total   int           `json:"total"`
	} `json:"data"`
}

// storageMount is the agent-friendly projection of an OpenList
// storage entry. Only the fields the agent actually uses are
// captured here:
//
//   - ID        OpenList storage ID (used by admin endpoints)
//   - MountPath absolute path on the OpenList instance where
//     this storage is mounted (e.g. "/", "/local", "/s3")
//   - Driver    storage driver name (e.g. "Local", "S3",
//     "GoogleDrive", "Onedrive", "AlistV3"...)
//   - Order     load order (smaller = higher priority on
//     overlapping mount paths)
//   - Enabled   whether the storage is enabled
//   - Status    runtime status (OpenList reports "work" when
//     healthy, other strings when broken)
//
// The agent surfaces this list verbatim to the LLM so it can
// "perceive" the file-system layout: before calling
// list_files("/foo") the LLM should first call ListStorages
// to discover whether /foo is a valid mount path.
type storageMount struct {
	ID        int    `json:"id"`
	MountPath string `json:"mount_path"`
	Driver    string `json:"driver"`
	Order     int    `json:"order"`
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
}

// ListStoragesResult is the agent-friendly result of
// ListStorages.
type ListStoragesResult struct {
	Items []storageMount `json:"items"`
	Total int            `json:"total"`
}

// ListStorages calls OpenList's /api/admin/storage/list and
// returns the list of mounted storage backends. The agent
// uses this to "perceive" what file systems are actually
// mounted on the remote instance — without it the LLM has to
// guess mount paths when calling list_files, which silently
// returns an empty list for nonexistent paths.
//
// The endpoint is admin-only; pass a non-admin token and you
// get a 401.
func (c *OpenListClient) ListStorages(ctx context.Context) (*ListStoragesResult, error) {
	var resp storageListResponse
	if err := c.doJSON(ctx, "GET", "/api/admin/storage/list", nil, &resp); err != nil {
		return nil, err
	}
	out := &ListStoragesResult{Total: 0}
	if resp.Data != nil {
		out.Total = resp.Data.Total
		for _, it := range resp.Data.Content {
			out.Items = append(out.Items, it)
		}
	}
	return out, nil
}

// SearchFiles searches by keyword. OpenList's /api/admin/fs/search
// returns a list of file items; we return the typed slice.
func (c *OpenListClient) SearchFiles(ctx context.Context, parent, keyword string) ([]fileItem, error) {
	body, _ := json.Marshal(map[string]any{
		"parent":  parent,
		"keywords": keyword,
	})
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    *struct {
			Content []fileItem `json:"content"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, "POST", "/api/admin/fs/search", body, &resp); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		return nil, nil
	}
	return resp.Data.Content, nil
}

// doJSON is the shared helper for all JSON request / response
// methods. It signs the request with the token, decodes the
// response into `out` (if non-nil), and surfaces a typed
// *OpenListError on any non-2xx response.
func (c *OpenListClient) doJSON(
	ctx context.Context,
	method, path string,
	body []byte,
	out any,
) error {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("openlist: invalid base url: %w", err)
	}
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return fmt.Errorf("openlist: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(OpenListAuthHeader, c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("openlist: do request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &OpenListError{
			Status: resp.StatusCode,
			Method: method,
			Path:   path,
			Body:   string(raw),
		}
	}
	if out == nil {
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("openlist: decode response: %w (body: %s)", err, string(raw))
	}
	return nil
}

// Ping issues a GET /api/me. It is used by the agent's
// health-check path to confirm the token is valid.
func (c *OpenListClient) Ping(ctx context.Context) error {
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	return c.doJSON(ctx, "GET", "/api/me", nil, &resp)
}
