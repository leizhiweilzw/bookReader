package main

import (
	"path/filepath"
	"testing"
)

// openTestDB creates a dbStore backed by a temp file for testing.
func openTestDB(t *testing.T) *dbStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := openDB(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestDBSaveAndGetBook(t *testing.T) {
	s := openTestDB(t)

	book, err := NewBook("sample-novel.txt")
	if err != nil {
		t.Fatalf("NewBook: %v", err)
	}

	if err := s.SaveBook(book); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	// Progress should default to chapter 0, 0%.
	p := s.GetProgress(book.ID)
	if p == nil {
		t.Fatal("progress not found after save")
	}
	if p.ChapterIndex != 0 || p.ScrollPercent != 0 {
		t.Errorf("default progress = ch %d / %.1f, want 0 / 0", p.ChapterIndex, p.ScrollPercent)
	}

	// Update progress.
	if err := s.SaveProgress(book.ID, 3, 42.5); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	p = s.GetProgress(book.ID)
	if p.ChapterIndex != 3 || p.ScrollPercent != 42.5 {
		t.Errorf("progress = ch %d / %.1f, want 3 / 42.5", p.ChapterIndex, p.ScrollPercent)
	}
}

func TestDBChapterReadStatus(t *testing.T) {
	s := openTestDB(t)

	book, err := NewBook("sample-novel.txt")
	if err != nil {
		t.Fatalf("NewBook: %v", err)
	}
	if err := s.SaveBook(book); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	// All chapters should be unread initially.
	recs, err := s.GetChapters(book.ID)
	if err != nil {
		t.Fatalf("GetChapters: %v", err)
	}
	for _, r := range recs {
		if r.IsRead {
			t.Errorf("chapter %d should be unread", r.ChapterIndex)
		}
	}

	// Mark chapter 5 as read.
	if err := s.MarkChapterRead(book.ID, 5); err != nil {
		t.Fatalf("MarkChapterRead: %v", err)
	}

	recs, err = s.GetChapters(book.ID)
	if err != nil {
		t.Fatalf("GetChapters: %v", err)
	}
	readCount := 0
	for _, r := range recs {
		if r.IsRead {
			readCount++
			if r.ChapterIndex != 5 {
				t.Errorf("unexpected read chapter %d", r.ChapterIndex)
			}
		}
	}
	if readCount != 1 {
		t.Errorf("expected 1 read chapter, got %d", readCount)
	}
}

func TestDBListAndRemoveBook(t *testing.T) {
	s := openTestDB(t)

	book, err := NewBook("sample-novel.txt")
	if err != nil {
		t.Fatalf("NewBook: %v", err)
	}
	if err := s.SaveBook(book); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	books, err := s.ListBooks()
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}

	if err := s.RemoveBook(book.ID); err != nil {
		t.Fatalf("RemoveBook: %v", err)
	}

	books, err = s.ListBooks()
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("expected 0 books after remove, got %d", len(books))
	}

	// Chapters should also be gone.
	recs, err := s.GetChapters(book.ID)
	if err != nil {
		t.Fatalf("GetChapters: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected 0 chapters after remove, got %d", len(recs))
	}
}

func TestDBSaveBookPreservesReadStatus(t *testing.T) {
	s := openTestDB(t)

	book, err := NewBook("sample-novel.txt")
	if err != nil {
		t.Fatalf("NewBook: %v", err)
	}
	if err := s.SaveBook(book); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	if err := s.MarkChapterRead(book.ID, 2); err != nil {
		t.Fatalf("MarkChapterRead: %v", err)
	}

	// Re-save the book (simulating reopening) - read status must be preserved.
	if err := s.SaveBook(book); err != nil {
		t.Fatalf("SaveBook again: %v", err)
	}

	recs, err := s.GetChapters(book.ID)
	if err != nil {
		t.Fatalf("GetChapters: %v", err)
	}
	for _, r := range recs {
		if r.ChapterIndex == 2 && !r.IsRead {
			t.Error("chapter 2 read status was not preserved after re-save")
		}
	}
}
