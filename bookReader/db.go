package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
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

// BookRecord is a stored book entry with its last reading position.
type BookRecord struct {
	BookID            string  `json:"bookId"`
	Title             string  `json:"title"`
	FilePath          string  `json:"filePath"`
	FileSize          int64   `json:"fileSize"`
	LastChapterIndex  int     `json:"lastChapterIndex"`
	LastScrollPercent float64 `json:"lastScrollPercent"`
	UpdatedAt         int64   `json:"updatedAt"`
}

// ChapterRecord is a stored chapter with its read status.
type ChapterRecord struct {
	BookID       string `json:"bookId"`
	ChapterIndex int    `json:"chapterIndex"`
	Title        string `json:"title"`
	IsRead       bool   `json:"isRead"`
	ReadAt       int64  `json:"readAt"`
}

// dbStore persists books, chapters, and reading progress in a SQLite database.
type dbStore struct {
	mu sync.Mutex
	db *sql.DB
}

func newDBStore() (*dbStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(dir, "bookReader")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return nil, err
	}
	return openDB(filepath.Join(appDir, "library.db"))
}

// openDB opens (or creates) a SQLite database at the given path.
func openDB(dbPath string) (*dbStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, err
	}

	store := &dbStore{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *dbStore) initSchema() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS books (
		book_id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		file_path TEXT NOT NULL,
		file_size INTEGER NOT NULL DEFAULT 0,
		last_chapter_index INTEGER NOT NULL DEFAULT 0,
		last_scroll_percent REAL NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS chapters (
		book_id TEXT NOT NULL,
		chapter_index INTEGER NOT NULL,
		title TEXT NOT NULL,
		is_read INTEGER NOT NULL DEFAULT 0,
		read_at INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (book_id, chapter_index),
		FOREIGN KEY (book_id) REFERENCES books(book_id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_chapters_book ON chapters(book_id);
	`)
	return err
}

// SaveBook upserts a book and all its chapters. Existing chapter read status
// is preserved.
func (s *dbStore) SaveBook(book *Book) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert book. If it already exists, preserve last reading position.
	_, err = tx.Exec(`
	INSERT INTO books (book_id, title, file_path, file_size, last_chapter_index, last_scroll_percent, created_at, updated_at)
	VALUES (?, ?, ?, ?, 0, 0, ?, ?)
	ON CONFLICT(book_id) DO UPDATE SET
		title = excluded.title,
		file_path = excluded.file_path,
		file_size = excluded.file_size,
		updated_at = excluded.updated_at
	`, book.ID, book.Title, book.FilePath, book.Size, now, now)
	if err != nil {
		return fmt.Errorf("upsert book: %w", err)
	}

	// Upsert chapters. Preserve is_read/read_at for already-known chapters.
	for _, c := range book.Chapters {
		_, err = tx.Exec(`
		INSERT INTO chapters (book_id, chapter_index, title, is_read, read_at)
		VALUES (?, ?, ?, 0, 0)
		ON CONFLICT(book_id, chapter_index) DO UPDATE SET title = excluded.title
		`, book.ID, c.Index, c.Title)
		if err != nil {
			return fmt.Errorf("upsert chapter %d: %w", c.Index, err)
		}
	}

	return tx.Commit()
}

// SaveProgress updates the last reading position for a book.
func (s *dbStore) SaveProgress(bookID string, chapterIndex int, scrollPercent float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	res, err := s.db.Exec(`
	UPDATE books SET last_chapter_index = ?, last_scroll_percent = ?, updated_at = ?
	WHERE book_id = ?
	`, chapterIndex, scrollPercent, now, bookID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("book %s not found", bookID)
	}
	return nil
}

// GetProgress returns the last reading position for a book, or nil.
func (s *dbStore) GetProgress(bookID string) *ReadingProgress {
	s.mu.Lock()
	defer s.mu.Unlock()

	var p ReadingProgress
	var title, filePath string
	var updatedAt int64
	err := s.db.QueryRow(`
	SELECT book_id, title, file_path, last_chapter_index, last_scroll_percent, updated_at
	FROM books WHERE book_id = ?
	`, bookID).Scan(&p.BookID, &title, &filePath, &p.ChapterIndex, &p.ScrollPercent, &updatedAt)
	if err != nil {
		return nil
	}
	p.BookTitle = title
	p.FilePath = filePath
	p.UpdatedAt = updatedAt
	return &p
}

// MarkChapterRead marks a chapter as read and records the timestamp.
func (s *dbStore) MarkChapterRead(bookID string, chapterIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(`
	UPDATE chapters SET is_read = 1, read_at = ?
	WHERE book_id = ? AND chapter_index = ? AND is_read = 0
	`, now, bookID, chapterIndex)
	return err
}

// GetChapters returns all chapters of a book with their read status.
func (s *dbStore) GetChapters(bookID string) ([]ChapterRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
	SELECT chapter_index, title, is_read, read_at
	FROM chapters WHERE book_id = ? ORDER BY chapter_index ASC
	`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChapterRecord
	for rows.Next() {
		var c ChapterRecord
		var isRead int
		if err := rows.Scan(&c.ChapterIndex, &c.Title, &isRead, &c.ReadAt); err != nil {
			return nil, err
		}
		c.BookID = bookID
		c.IsRead = isRead == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListBooks returns all stored books ordered by most recently read first.
func (s *dbStore) ListBooks() ([]BookRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
	SELECT book_id, title, file_path, file_size, last_chapter_index, last_scroll_percent, updated_at
	FROM books ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookRecord
	for rows.Next() {
		var b BookRecord
		if err := rows.Scan(&b.BookID, &b.Title, &b.FilePath, &b.FileSize, &b.LastChapterIndex, &b.LastScrollPercent, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// RemoveBook deletes a book and all its chapters.
func (s *dbStore) RemoveBook(bookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM chapters WHERE book_id = ?", bookID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM books WHERE book_id = ?", bookID); err != nil {
		return err
	}
	return tx.Commit()
}

// Close closes the database.
func (s *dbStore) Close() error {
	return s.db.Close()
}
