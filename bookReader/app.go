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
	ctx      context.Context
	store    *progressStore
	books    map[string]*Book
	booksMu  sync.RWMutex
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{
		books: make(map[string]*Book),
	}
}

// startup is called when the app starts. The context is saved so we can call
// the runtime methods, and the progress store is initialised.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	store, err := newProgressStore()
	if err != nil {
		log.Printf("failed to init progress store: %v", err)
	}
	a.store = store
}

// shim to return an error result for the frontend when the store is unavailable.
func (a *App) storeReady() error {
	if a.store == nil {
		return fmt.Errorf("阅读记录存储未初始化")
	}
	return nil
}

// OpenFile shows a native file picker for TXT files, parses the selected book,
// and returns its metadata together with any saved reading progress.
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
	prog := a.store.Get(bookID)
	if prog == nil {
		return nil, fmt.Errorf("未找到该书籍的阅读记录")
	}
	return a.loadBook(prog.FilePath)
}

// loadBook reads the file from disk, caches it, and returns metadata + progress.
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
		Chapters:     book.Chapters,
	}

	if a.store != nil {
		if p := a.store.Get(book.ID); p != nil {
			result.Progress = p
		}
	}
	return result, nil
}

// GetChapter returns the text content of a chapter.
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
	return book.ChapterText(chapterIndex), nil
}

// SaveProgress persists the current reading position.
func (a *App) SaveProgress(bookID string, chapterIndex int, scrollPercent float64) error {
	if err := a.storeReady(); err != nil {
		return err
	}
	a.booksMu.RLock()
	book, ok := a.books[bookID]
	a.booksMu.RUnlock()
	if !ok {
		return fmt.Errorf("书籍未加载")
	}
	return a.store.Save(bookID, book.Title, book.FilePath, chapterIndex, scrollPercent)
}

// GetProgress returns the saved reading progress for a book.
func (a *App) GetProgress(bookID string) (*ReadingProgress, error) {
	if err := a.storeReady(); err != nil {
		return nil, err
	}
	return a.store.Get(bookID), nil
}

// ListRecent returns all books that have a saved reading progress entry.
func (a *App) ListRecent() ([]*ReadingProgress, error) {
	if err := a.storeReady(); err != nil {
		return nil, err
	}
	return a.store.List(), nil
}

// RemoveRecent deletes the reading record (and thus the recent entry) for a book.
func (a *App) RemoveRecent(bookID string) error {
	if err := a.storeReady(); err != nil {
		return err
	}
	return a.store.Delete(bookID)
}

// OpenResult is the payload returned to the frontend after opening a book.
type OpenResult struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	FilePath     string            `json:"filePath"`
	ChapterCount int               `json:"chapterCount"`
	Chapters     []Chapter         `json:"chapters"`
	Progress     *ReadingProgress  `json:"progress"`
}
