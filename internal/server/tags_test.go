package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagStore_AddTag(t *testing.T) {
	store := &TagStore{
		fileTags: make(map[string][]string),
		tagFiles: make(map[string][]string),
	}

	store.AddTag("/file1.txt", "important")

	tags := store.GetFileTags("/file1.txt")
	assert.Equal(t, []string{"important"}, tags)

	allTags := store.GetAllTags()
	assert.Equal(t, 1, allTags["important"])
}

func TestTagStore_AddDuplicateTag(t *testing.T) {
	store := &TagStore{
		fileTags: make(map[string][]string),
		tagFiles: make(map[string][]string),
	}

	store.AddTag("/file1.txt", "important")
	store.AddTag("/file1.txt", "important")

	allTags := store.GetAllTags()
	assert.Equal(t, 1, allTags["important"])

	tags := store.GetFileTags("/file1.txt")
	assert.Len(t, tags, 1)
}

func TestTagStore_RemoveTag(t *testing.T) {
	store := &TagStore{
		fileTags: make(map[string][]string),
		tagFiles: make(map[string][]string),
	}

	store.AddTag("/file1.txt", "important")
	store.RemoveTag("/file1.txt", "important")

	tags := store.GetFileTags("/file1.txt")
	assert.Empty(t, tags)

	allTags := store.GetAllTags()
	_, exists := allTags["important"]
	assert.False(t, exists)
}

func TestTagStore_RemoveNonExistent(t *testing.T) {
	store := &TagStore{
		fileTags: make(map[string][]string),
		tagFiles: make(map[string][]string),
	}

	assert.NotPanics(t, func() {
		store.RemoveTag("/nonexistent.txt", "missing")
	})

	tags := store.GetFileTags("/nonexistent.txt")
	assert.Empty(t, tags)
}

func TestTagStore_MultipleFilesSameTag(t *testing.T) {
	store := &TagStore{
		fileTags: make(map[string][]string),
		tagFiles: make(map[string][]string),
	}

	store.AddTag("/a.txt", "favorite")
	store.AddTag("/b.txt", "favorite")
	store.AddTag("/c.txt", "favorite")

	allTags := store.GetAllTags()
	assert.Equal(t, 3, allTags["favorite"])

	files := store.GetFilesByTag("favorite")
	assert.Len(t, files, 3)
	assert.Contains(t, files, "/a.txt")
	assert.Contains(t, files, "/b.txt")
	assert.Contains(t, files, "/c.txt")
}

func TestTagStore_TagNameLowercased(t *testing.T) {
	store := &TagStore{
		fileTags: make(map[string][]string),
		tagFiles: make(map[string][]string),
	}

	store.AddTag("/file1.txt", "IMPORTANT")

	tags := store.GetFileTags("/file1.txt")
	assert.Equal(t, []string{"important"}, tags)

	allTags := store.GetAllTags()
	_, exists := allTags["IMPORTANT"]
	assert.False(t, exists)
	assert.Equal(t, 1, allTags["important"])
}

func setupTagsRouter() (*gin.Engine, *Server) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{}
	r.GET("/api/files/tags", s.handleTagsListGin)
	r.POST("/api/files/tags", s.handleTagsMutateGin)
	return r, s
}

func resetGlobalTagStore() {
	GlobalTagStore.mu.Lock()
	defer GlobalTagStore.mu.Unlock()
	GlobalTagStore.fileTags = make(map[string][]string)
	GlobalTagStore.tagFiles = make(map[string][]string)
}

func TestHandleTagsListGin_Empty(t *testing.T) {
	resetGlobalTagStore()
	r, _ := setupTagsRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/files/tags", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	tags, ok := resp["tags"]
	require.True(t, ok)
	assert.Empty(t, tags)
}

func TestHandleTagsListGin_WithData(t *testing.T) {
	resetGlobalTagStore()
	GlobalTagStore.AddTag("/video.mp4", "movie")
	GlobalTagStore.AddTag("/doc.pdf", "movie")
	GlobalTagStore.AddTag("/video.mp4", "favorite")

	r, _ := setupTagsRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/files/tags", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]json.RawMessage
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	tagsRaw, ok := resp["tags"]
	require.True(t, ok)

	var tags []map[string]interface{}
	err = json.Unmarshal(tagsRaw, &tags)
	require.NoError(t, err)
	assert.Len(t, tags, 2)

	nameSet := make(map[string]int)
	countMap := make(map[string]int)
	for _, entry := range tags {
		name := entry["name"].(string)
		count := int(entry["count"].(float64))
		nameSet[name]++
		countMap[name] = count
	}
	assert.Contains(t, nameSet, "movie")
	assert.Contains(t, nameSet, "favorite")
	assert.Equal(t, 2, countMap["movie"])
	assert.Equal(t, 1, countMap["favorite"])
}

func TestHandleTagsMutateGin_Add(t *testing.T) {
	resetGlobalTagStore()
	r, _ := setupTagsRouter()

	body, _ := json.Marshal(map[string]string{
		"path":   "/test.mp4",
		"tag":    "important",
		"action": "add",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/files/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	tags := GlobalTagStore.GetFileTags("/test.mp4")
	assert.Contains(t, tags, "important")
}

func TestHandleTagsMutateGin_Remove(t *testing.T) {
	resetGlobalTagStore()
	GlobalTagStore.AddTag("/test.mp4", "old-tag")

	r, _ := setupTagsRouter()

	body, _ := json.Marshal(map[string]string{
		"path":   "/test.mp4",
		"tag":    "old-tag",
		"action": "remove",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/files/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	tags := GlobalTagStore.GetFileTags("/test.mp4")
	assert.NotContains(t, tags, "old-tag")
}

func TestHandleTagsMutateGin_InvalidAction(t *testing.T) {
	resetGlobalTagStore()
	r, _ := setupTagsRouter()

	body, _ := json.Marshal(map[string]string{
		"path":   "/test.mp4",
		"tag":    "some",
		"action": "update",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/files/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "action must be 'add' or 'remove'")
}
