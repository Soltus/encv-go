package server

import (
	"strings"
	"sync"
)

type TagStore struct {
	mu       sync.RWMutex
	fileTags map[string][]string
	tagFiles map[string][]string
}

var GlobalTagStore = &TagStore{
	fileTags: make(map[string][]string),
	tagFiles: make(map[string][]string),
}

func (t *TagStore) AddTag(path, tag string) {
	tag = strings.ToLower(tag)
	t.mu.Lock()
	defer t.mu.Unlock()

	tags := t.fileTags[path]
	for _, existing := range tags {
		if existing == tag {
			return
		}
	}
	t.fileTags[path] = append(tags, tag)

	files := t.tagFiles[tag]
	for _, existing := range files {
		if existing == path {
			return
		}
	}
	t.tagFiles[tag] = append(files, path)
}

func (t *TagStore) RemoveTag(path, tag string) {
	tag = strings.ToLower(tag)
	t.mu.Lock()
	defer t.mu.Unlock()

	tags := t.fileTags[path]
	filtered := make([]string, 0, len(tags))
	for _, existing := range tags {
		if existing != tag {
			filtered = append(filtered, existing)
		}
	}
	if len(filtered) == 0 {
		delete(t.fileTags, path)
	} else {
		t.fileTags[path] = filtered
	}

	files := t.tagFiles[tag]
	fileFiltered := make([]string, 0, len(files))
	for _, existing := range files {
		if existing != path {
			fileFiltered = append(fileFiltered, existing)
		}
	}
	if len(fileFiltered) == 0 {
		delete(t.tagFiles, tag)
	} else {
		t.tagFiles[tag] = fileFiltered
	}
}

func (t *TagStore) GetFileTags(path string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	tags := t.fileTags[path]
	result := make([]string, len(tags))
	copy(result, tags)
	return result
}

func (t *TagStore) GetAllTags() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]int, len(t.tagFiles))
	for tag, files := range t.tagFiles {
		result[tag] = len(files)
	}
	return result
}

func (t *TagStore) GetFilesByTag(tag string) []string {
	tag = strings.ToLower(tag)
	t.mu.RLock()
	defer t.mu.RUnlock()
	files := t.tagFiles[tag]
	result := make([]string, len(files))
	copy(result, files)
	return result
}
