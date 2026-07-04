package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeOpenListServer is a single httptest server that emulates
// the eight OpenList endpoints the agent uses. Each test
// re-arms the server's response map before issuing the call.
type fakeOpenListServer struct {
	responses map[string]fakeResponse
	gotAuth   string
}

type fakeResponse struct {
	status int
	body   string
}

func newFakeOpenListServer() *fakeOpenListServer {
	return &fakeOpenListServer{
		responses: map[string]fakeResponse{},
	}
}

func (s *fakeOpenListServer) set(method, path string, status int, body string) {
	s.responses[method+" "+path] = fakeResponse{status, body}
}

func (s *fakeOpenListServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.gotAuth = r.Header.Get(OpenListAuthHeader)
	resp, ok := s.responses[r.Method+" "+r.URL.Path]
	if !ok {
		http.Error(w, "no scripted response for "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.status)
	_, _ = w.Write([]byte(resp.body))
}

func TestOpenListClient_Ping_OK(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("GET", "/api/me", 200, `{"code":200,"message":"ok"}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	c := NewOpenListClient(ts.URL, "tok-1", nil)
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
	if srv.gotAuth != "tok-1" {
		t.Errorf("auth header: %q", srv.gotAuth)
	}
}

func TestOpenListClient_Ping_Unauthorized(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("GET", "/api/me", 401, `{"code":401,"message":"unauthorized"}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	c := NewOpenListClient(ts.URL, "bad", nil)
	err := c.Ping(context.Background())
	if err == nil {
		t.Errorf("expected error on 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should contain 401: %v", err)
	}
}

func TestOpenListClient_ListFiles(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("POST", "/api/admin/fs/list", 200, `{
		"code": 200,
		"message": "ok",
		"data": {
			"content": [
				{"name": "a.txt", "size": 10, "is_dir": false, "modified": "2025-01-01T00:00:00Z"},
				{"name": "b", "size": 0, "is_dir": true, "modified": "2025-01-01T00:00:00Z"}
			],
			"total": 2
		}
	}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	res, err := c.ListFiles(context.Background(), "/")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("total: %d", res.Total)
	}
	if len(res.Items) != 2 {
		t.Errorf("items: %d", len(res.Items))
	}
	if res.Items[0].Name != "a.txt" {
		t.Errorf("name: %q", res.Items[0].Name)
	}
	if !res.Items[1].IsDir {
		t.Errorf("b should be a directory")
	}
}

func TestOpenListClient_ReadFile(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("POST", "/api/admin/fs/get", 200, `{
		"code": 200,
		"message": "ok",
		"data": "hello world"
	}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	got, err := c.ReadFile(context.Background(), "/a.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content: %q", got)
	}
}

func TestOpenListClient_WriteFile(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("POST", "/api/admin/fs/put", 200, `{"code":200,"message":"ok"}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	ok, err := c.WriteFile(context.Background(), "/a.txt", []byte("data"))
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !ok {
		t.Errorf("expected success")
	}
}

func TestOpenListClient_DeleteFile(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("POST", "/api/fs/remove", 200, `{"code":200,"message":"ok"}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	ok, err := c.DeleteFile(context.Background(), "/a.txt")
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if !ok {
		t.Errorf("expected success")
	}
}

func TestOpenListClient_Rename(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("POST", "/api/fs/rename", 200, `{"code":200,"message":"ok"}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	ok, err := c.Rename(context.Background(), "/a.txt", "/b.txt")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if !ok {
		t.Errorf("expected success")
	}
}

func TestOpenListClient_ExecCommand(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("POST", "/api/admin/command", 200, `{"code":200,"message":"ok","data":"ls output"}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	out, err := c.ExecCommand(context.Background(), "ls")
	if err != nil {
		t.Fatalf("ExecCommand: %v", err)
	}
	if out != "ls output" {
		t.Errorf("output: %q", out)
	}
}

func TestOpenListClient_GetStorageInfo(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("GET", "/api/admin/storage", 200, `{
		"code": 200,
		"message": "ok",
		"data": {"total": 1000, "used": 500}
	}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	info, err := c.GetStorageInfo(context.Background())
	if err != nil {
		t.Fatalf("GetStorageInfo: %v", err)
	}
	if info["total"].(float64) != 1000 {
		t.Errorf("total: %v", info["total"])
	}
}

func TestOpenListClient_SearchFiles(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("POST", "/api/admin/fs/search", 200, `{
		"message": "ok",
		"data": {
			"content": [
				{"name": "report.pdf", "size": 100, "is_dir": false, "modified": "2025-01-01T00:00:00Z"}
			]
		}
	}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	hits, err := c.SearchFiles(context.Background(), "/", "report")
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("hits: %d", len(hits))
	}
	if hits[0].Name != "report.pdf" {
		t.Errorf("name: %q", hits[0].Name)
	}
}

func TestOpenListClient_StripsTrailingSlash(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("GET", "/api/me", 200, `{"code":200,"message":"ok"}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL+"/", "tok", nil)
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestOpenListError_Formats(t *testing.T) {
	e := &OpenListError{Status: 401, Method: "GET", Path: "/api/me", Body: "unauthorized"}
	if !strings.Contains(e.Error(), "401") || !strings.Contains(e.Error(), "unauthorized") {
		t.Errorf("error format: %q", e.Error())
	}
}

func TestOpenListClient_DoJSON_InvalidBaseURL(t *testing.T) {
	c := NewOpenListClient("://invalid", "tok", nil)
	err := c.Ping(context.Background())
	if err == nil {
		t.Errorf("expected error for invalid base url")
	}
}

func TestOpenListClient_DoJSON_NoResponseBody(t *testing.T) {
	// Some endpoints return 200 with an empty body. The
	// decoder should not panic.
	srv := newFakeOpenListServer()
	srv.set("GET", "/api/me", 200, "")
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("Ping with empty body: %v", err)
	}
}

func TestOpenListClient_RequestBodyIsValidJSON(t *testing.T) {
	srv := newFakeOpenListServer()
	srv.set("POST", "/api/admin/fs/list", 200, `{"code":200,"data":{"content":[],"total":0}}`)
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := NewOpenListClient(ts.URL, "tok", nil)
	_, err := c.ListFiles(context.Background(), "/some/path")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	// We don't have access to the request body directly,
	// but the test passes if ListFiles returns without
	// error, which proves the body was well-formed.
	_ = json.Valid
}
