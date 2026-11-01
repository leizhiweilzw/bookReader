package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ReadingProgress records where the reader left off in a book.
type ReadingProgress struct {
	BookID        string  `json:"bookId"`
	BookTitle     string  `json:"bookTitle"`
	FilePath      string  `json:"filePath"`
	ChapterIndex  int     `json:"chapterIndex"`
	ScrollPercent float64 `json:"scrollPercent"` // 0 - 100
	UpdatedAt     int64   `json:"updatedAt"`
}

// progressStore persists reading progress to a JSON file on disk.
type progressStore struct {
	mu       sync.Mutex
	filePath string
	items    map[string]*ReadingProgress
}

func newProgressStore() (*progressStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(dir, "bookReader")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return nil, err
	}
	ps := &progressStore{
		filePath: filepath.Join(appDir, "progress.json"),
		items:    make(map[string]*ReadingProgress),
	}
	if err := ps.load(); err != nil {
		return nil, err
	}
	return ps, nil
}

func (ps *progressStore) load() error {
	data, err := os.ReadFile(ps.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Reset the map so reloading replaces rather than merges entries.
	ps.items = make(map[string]*ReadingProgress)
	return json.Unmarshal(data, &ps.items)
}

func (ps *progressStore) save() error {
	data, err := json.MarshalIndent(ps.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ps.filePath, data, 0o644)
}

// Save stores the reading progress for a book.
func (ps *progressStore) Save(bookID, title, filePath string, chapterIndex int, scrollPercent float64) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.items[bookID] = &ReadingProgress{
		BookID:        bookID,
		BookTitle:     title,
		FilePath:      filePath,
		ChapterIndex:  chapterIndex,
		ScrollPercent: scrollPercent,
		UpdatedAt:     time.Now().Unix(),
	}
	return ps.save()
}

// Get returns the stored progress for a book, or nil if none exists.
func (ps *progressStore) Get(bookID string) *ReadingProgress {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.items[bookID]
}

// List returns all stored progress entries ordered by most recently read first.
func (ps *progressStore) List() []*ReadingProgress {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	out := make([]*ReadingProgress, 0, len(ps.items))
	for _, p := range ps.items {
		out = append(out, p)
	}
	// Sort by updated time descending.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].UpdatedAt > out[i].UpdatedAt {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// Delete removes the progress entry for a book.
func (ps *progressStore) Delete(bookID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.items, bookID)
	return ps.save()
}
