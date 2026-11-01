package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct exposed to the frontend.
type App struct {
	ctx     context.Context
	store   *dbStore
	books   map[string]*Book
	booksMu sync.RWMutex
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{
		books: make(map[string]*Book),
	}
}

// startup is called when the app starts. The context is saved so we can call
// the runtime methods, and the SQLite store is initialised.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	store, err := newDBStore()
	if err != nil {
		log.Printf("failed to init db store: %v", err)
	}
	a.store = store
}

func (a *App) storeReady() error {
	if a.store == nil {
		return fmt.Errorf("数据存储未初始化")
	}
	return nil
}

// OpenFile shows a native file picker for TXT files, parses the selected book,
// persists it to SQLite, and returns its metadata + saved reading progress.
func (a *App) OpenFile() (*OpenResult, error) {
	selection, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 TXT 小说文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "文本文件 (*.txt)", Pattern: "*.txt"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if selection == "" {
		return nil, nil // user cancelled
	}
	return a.loadBook(selection)
}

// OpenRecent reopens a previously read book by its stored file path.
func (a *App) OpenRecent(bookID string) (*OpenResult, error) {
	if err := a.storeReady(); err != nil {
		return nil, err
	}
	prog := a.store.GetProgress(bookID)
	if prog == nil {
		return nil, fmt.Errorf("未找到该书籍的阅读记录")
	}
	return a.loadBook(prog.FilePath)
}

// loadBook reads the file from disk, caches it, saves it to the DB, and
// returns metadata + progress + chapter read statuses.
func (a *App) loadBook(path string) (*OpenResult, error) {
	book, err := NewBook(path)
	if err != nil {
		return nil, err
	}
	a.booksMu.Lock()
	a.books[book.ID] = book
	a.booksMu.Unlock()

	result := &OpenResult{
		ID:           book.ID,
		Title:        book.Title,
		FilePath:     book.FilePath,
		ChapterCount: book.ChapterCount,
	}

	if a.store != nil {
		// Persist the book and its chapters (read status preserved).
		if err := a.store.SaveBook(book); err != nil {
			log.Printf("save book failed: %v", err)
		}
		// Restore last reading position.
		if p := a.store.GetProgress(book.ID); p != nil {
			result.Progress = p
		}
		// Populate chapter read status from DB.
		if recs, err := a.store.GetChapters(book.ID); err == nil {
			readMap := make(map[int]ChapterRecord, len(recs))
			for _, r := range recs {
				readMap[r.ChapterIndex] = r
			}
			for i := range book.Chapters {
				if r, ok := readMap[book.Chapters[i].Index]; ok {
					book.Chapters[i].IsRead = r.IsRead
					book.Chapters[i].ReadAt = r.ReadAt
				}
			}
		}
	}
	result.Chapters = book.Chapters
	return result, nil
}

// GetChapter returns the text content of a chapter and marks it as read.
func (a *App) GetChapter(bookID string, chapterIndex int) (string, error) {
	a.booksMu.RLock()
	book, ok := a.books[bookID]
	a.booksMu.RUnlock()
	if !ok {
		return "", fmt.Errorf("书籍未加载")
	}
	if chapterIndex < 0 || chapterIndex >= book.ChapterCount {
		return "", fmt.Errorf("章节索引越界")
	}

	// Mark the chapter as read (idempotent).
	if a.store != nil {
		if err := a.store.MarkChapterRead(bookID, chapterIndex); err != nil {
			log.Printf("mark chapter read failed: %v", err)
		}
		// Update in-memory status.
		if chapterIndex < len(book.Chapters) {
			book.Chapters[chapterIndex].IsRead = true
		}
	}

	return book.ChapterText(chapterIndex), nil
}

// SaveProgress persists the current reading position.
func (a *App) SaveProgress(bookID string, chapterIndex int, scrollPercent float64) error {
	if err := a.storeReady(); err != nil {
		return err
	}
	a.booksMu.RLock()
	_, ok := a.books[bookID]
	a.booksMu.RUnlock()
	if !ok {
		return fmt.Errorf("书籍未加载")
	}
	return a.store.SaveProgress(bookID, chapterIndex, scrollPercent)
}

// GetProgress returns the saved reading progress for a book.
func (a *App) GetProgress(bookID string) (*ReadingProgress, error) {
	if err := a.storeReady(); err != nil {
		return nil, err
	}
	return a.store.GetProgress(bookID), nil
}

// ListRecent returns all stored books ordered by most recently read first.
func (a *App) ListRecent() ([]*ReadingProgress, error) {
	if err := a.storeReady(); err != nil {
		return nil, err
	}
	books, err := a.store.ListBooks()
	if err != nil {
		return nil, err
	}
	out := make([]*ReadingProgress, 0, len(books))
	for _, b := range books {
		out = append(out, &ReadingProgress{
			BookID:        b.BookID,
			BookTitle:     b.Title,
			FilePath:      b.FilePath,
			ChapterIndex:  b.LastChapterIndex,
			ScrollPercent: b.LastScrollPercent,
			UpdatedAt:     b.UpdatedAt,
		})
	}
	return out, nil
}

// RemoveRecent deletes a book and all its chapter records.
func (a *App) RemoveRecent(bookID string) error {
	if err := a.storeReady(); err != nil {
		return err
	}
	return a.store.RemoveBook(bookID)
}

// OpenResult is the payload returned to the frontend after opening a book.
type OpenResult struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	FilePath     string           `json:"filePath"`
	ChapterCount int              `json:"chapterCount"`
	Chapters     []Chapter        `json:"chapters"`
	Progress     *ReadingProgress `json:"progress"`
}
