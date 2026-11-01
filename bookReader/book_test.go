package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitChapters(t *testing.T) {
	content := "第一章 初入江湖\n这是第一章的内容。\n第二行内容。\n\n第二章 风云再起\n第二章的内容来了。\n第三章 大结局\n终章内容。\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	book, err := NewBook(path)
	if err != nil {
		t.Fatal(err)
	}

	if book.ChapterCount != 3 {
		t.Fatalf("expected 3 chapters, got %d", book.ChapterCount)
	}

	titles := []string{"第一章 初入江湖", "第二章 风云再起", "第三章 大结局"}
	for i, c := range book.Chapters {
		if c.Title != titles[i] {
			t.Errorf("chapter %d title = %q, want %q", i, c.Title, titles[i])
		}
	}

	if text := book.ChapterText(0); text != "这是第一章的内容。\n第二行内容。" {
		t.Errorf("chapter 0 text = %q", text)
	}
}

func TestSplitByPagesFallback(t *testing.T) {
	// Content with no chapter titles -> should fall back to page splitting.
	content := "这是一段没有章节标题的内容。\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "test2.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	book, err := NewBook(path)
	if err != nil {
		t.Fatal(err)
	}

	if book.ChapterCount == 0 {
		t.Fatal("expected at least one page chapter in fallback mode")
	}
}

func TestChapterPatterns(t *testing.T) {
	cases := []string{
		"第一章 初入江湖",
		"第三回 某某",
		"第十二卷 序章",
		"Chapter 12 The Beginning",
		"楔子",
		"番外一",
	}
	for _, c := range cases {
		if !isChapterTitle(c) {
			t.Errorf("expected %q to be a chapter title", c)
		}
	}

	negatives := []string{
		"这是正文内容，不是标题。",
		"他走进了第一章描述的那个房间。",
	}
	for _, c := range negatives {
		if isChapterTitle(c) {
			t.Errorf("expected %q NOT to be a chapter title", c)
		}
	}
}
