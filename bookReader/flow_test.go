package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadingFlow simulates the full reading lifecycle through the backend:
// load book -> read chapters -> save progress -> reopen -> restore position.
func TestReadingFlow(t *testing.T) {
	// 1. Parse the sample novel.
	book, err := NewBook("sample-novel.txt")
	if err != nil {
		t.Fatalf("NewBook failed: %v", err)
	}
	if book.ChapterCount != 10 {
		t.Fatalf("expected 10 chapters, got %d", book.ChapterCount)
	}

	// 2. Read a couple of chapters and verify content is non-empty.
	for _, idx := range []int{0, 5, 9} {
		text := book.ChapterText(idx)
		if text == "" {
			t.Errorf("chapter %d text is empty", idx)
		}
	}
	// First chapter should NOT include its own title line.
	firstBody := book.ChapterText(0)
	if contains(firstBody, "第一章 初入江湖") {
		t.Error("chapter 0 body should not contain the title line")
	}

	// 3. Set up a temporary progress store.
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "progress.json")
	ps := &progressStore{
		filePath: storePath,
		items:    make(map[string]*ReadingProgress),
	}

	// 4. Save progress for chapter 3 at 42.5% scroll.
	if err := ps.Save(book.ID, book.Title, book.FilePath, 3, 42.5); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 5. Verify it's retrievable.
	got := ps.Get(book.ID)
	if got == nil {
		t.Fatal("progress not found after save")
	}
	if got.ChapterIndex != 3 {
		t.Errorf("chapter index = %d, want 3", got.ChapterIndex)
	}
	if got.ScrollPercent != 42.5 {
		t.Errorf("scroll percent = %f, want 42.5", got.ScrollPercent)
	}

	// 6. Reopen store from disk to confirm persistence.
	ps2, err := newProgressStore()
	if err != nil {
		t.Fatalf("reopen store failed: %v", err)
	}
	// Override its path to our temp one.
	ps2.filePath = storePath
	if err := ps2.load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	got2 := ps2.Get(book.ID)
	if got2 == nil {
		t.Fatal("progress not persisted to disk")
	}
	if got2.ChapterIndex != 3 || got2.ScrollPercent != 42.5 {
		t.Errorf("persisted progress = %+v, want chapter 3 / 42.5%%", got2)
	}

	// 7. List recent books.
	recents := ps2.List()
	if len(recents) != 1 {
		t.Errorf("expected 1 recent book, got %d", len(recents))
	}

	// 8. Cleanup: delete progress.
	if err := ps2.Delete(book.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if ps2.Get(book.ID) != nil {
		t.Error("progress still exists after delete")
	}
}

// TestEncodingDetection verifies GBK-encoded content is decoded correctly.
func TestEncodingDetection(t *testing.T) {
	// "你好世界" in GBK bytes.
	gbkBytes := []byte{0xC4, 0xE3, 0xBA, 0xC3, 0xCA, 0xC0, 0xBD, 0xE7}
	decoded := decodeText(gbkBytes)
	if decoded != "你好世界" {
		t.Errorf("GBK decode = %q, want %q", decoded, "你好世界")
	}

	// UTF-8 should pass through unchanged.
	utf8Bytes := []byte("Hello 世界")
	if decodeText(utf8Bytes) != "Hello 世界" {
		t.Error("UTF-8 passthrough failed")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Ensure the progress store uses a temp location during tests by stubbing the
// config dir via an env-free approach: we just test the store directly.
func TestProgressStoreTempDir(t *testing.T) {
	tmpDir := t.TempDir()
	ps := &progressStore{
		filePath: filepath.Join(tmpDir, "p.json"),
		items:    make(map[string]*ReadingProgress),
	}
	if err := ps.Save("id1", "title1", "path1", 1, 50.0); err != nil {
		t.Fatal(err)
	}
	// Re-read from disk.
	data, err := os.ReadFile(ps.filePath)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if len(data) == 0 {
		t.Error("progress file is empty")
	}
}
