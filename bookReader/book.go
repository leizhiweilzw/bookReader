package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// Chapter represents a single chapter in a book.
type Chapter struct {
	Index  int    `json:"index"`
	Title  string `json:"title"`
	Start  int    `json:"start"`  // start line index in raw lines
	Length int    `json:"length"` // number of lines
	IsRead bool   `json:"isRead"` // whether the chapter has been read (from DB)
	ReadAt int64  `json:"readAt"` // unix timestamp when marked read
}

// Book holds the parsed contents of a TXT novel.
type Book struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	FilePath     string    `json:"filePath"`
	Lines        []string  `json:"-"`
	Chapters     []Chapter `json:"chapters"`
	ChapterCount int       `json:"chapterCount"`
	Size         int64     `json:"size"`
}

// Chapter patterns: common novel chapter title conventions.
var (
	chapterPatterns = []*regexp.Regexp{
		// 第X章/节/回/卷/篇/集/部/幕 (e.g. 第一章, 第三回, 第十二卷)
		regexp.MustCompile(`^[\s　]*第[一二三四五六七八九十百千零〇两\d]+[章节回卷篇集部幕]`),
		// English: Chapter 1 / CHAPTER 12
		regexp.MustCompile(`^[\s　]*[Cc][Hh][Aa][Pp][Tt][Ee][Rr]\s+\d+`),
		// Prologue / epilogue style standalone lines (allow trailing subtitle)
		regexp.MustCompile(`^[\s　]*(序章|楔子|引子|尾声|后记|前言|引言|番外|终章|大结局)`),
	}
	// sentenceEnders marks lines that are almost certainly body sentences, not titles.
	sentenceEnders = []string{"。", "！", "？", "!", "?"}
)

// NewBook reads a TXT file from disk and splits it into chapters.
func NewBook(filePath string) (*Book, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	text := decodeText(raw)

	// Normalise line endings and split.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	book := &Book{
		ID:       bookID(filePath),
		Title:    titleFromPath(filePath),
		FilePath: filePath,
		Lines:    lines,
		Size:     int64(len(raw)),
	}

	book.Chapters = splitChapters(lines)
	book.ChapterCount = len(book.Chapters)
	return book, nil
}

// decodeText tries UTF-8 first, then falls back to GBK/GB18030 for Chinese novels.
func decodeText(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	// Try GB18030 (superset of GBK) for Chinese text.
	decoder := simplifiedchinese.GB18030.NewDecoder()
	decoded, _, err := transform.Bytes(decoder, raw)
	if err == nil {
		return string(decoded)
	}
	return string(raw)
}

// splitChapters scans the lines and groups them into chapters based on title
// patterns. If no titles are found, it falls back to fixed-size page chunks.
func splitChapters(lines []string) []Chapter {
	var chapters []Chapter
	var current *Chapter

	flush := func(endIdx int) {
		if current != nil {
			current.Length = endIdx - current.Start
			chapters = append(chapters, *current)
			current = nil
		}
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Chapter title lines must be reasonably short to avoid false positives.
		if len(trimmed) <= 60 && isChapterTitle(trimmed) {
			flush(i)
			current = &Chapter{
				Index: len(chapters),
				Title: trimmed,
				Start: i,
			}
		}
	}
	flush(len(lines))

	// Fallback: if no chapter titles were detected, split into ~page-sized chunks.
	if len(chapters) == 0 {
		return splitByPages(lines)
	}
	return chapters
}

func isChapterTitle(s string) bool {
	// Chapter titles are never full sentences: reject lines ending with
	// sentence-ending punctuation.
	for _, e := range sentenceEnders {
		if strings.Contains(s, e) {
			return false
		}
	}
	for _, p := range chapterPatterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}

// splitByPages divides content into fixed-size chunks when no chapter titles
// are present. Each chunk is ~3000 characters (a comfortable reading page).
func splitByPages(lines []string) []Chapter {
	const pageSize = 3000
	var chapters []Chapter
	var buf strings.Builder
	var start = 0
	var charCount = 0
	idx := 0

	for i, line := range lines {
		buf.WriteString(line)
		buf.WriteString("\n")
		charCount += len(line) + 1
		if charCount >= pageSize {
			chapters = append(chapters, Chapter{
				Index:  idx,
				Title:  fmt.Sprintf("第 %d 页", idx+1),
				Start:  start,
				Length: i + 1 - start,
			})
			idx++
			start = i + 1
			charCount = 0
			buf.Reset()
		}
	}
	if start < len(lines) {
		chapters = append(chapters, Chapter{
			Index:  idx,
			Title:  fmt.Sprintf("第 %d 页", idx+1),
			Start:  start,
			Length: len(lines) - start,
		})
	}
	return chapters
}

// ChapterText returns the full text of a chapter (excluding the title line).
func (b *Book) ChapterText(index int) string {
	if index < 0 || index >= len(b.Chapters) {
		return ""
	}
	c := b.Chapters[index]
	end := c.Start + c.Length
	if end > len(b.Lines) {
		end = len(b.Lines)
	}
	// Skip the title line itself in the body.
	bodyStart := c.Start + 1
	if bodyStart > end {
		bodyStart = end
	}
	lines := b.Lines[bodyStart:end]
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// titleFromPath derives a book title from the file name (without extension).
func titleFromPath(path string) string {
	name := path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		name = path[i+1:]
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	return name
}

// bookID produces a stable identifier for a file path.
func bookID(path string) string {
	h := sha1.Sum([]byte(strings.ToLower(path)))
	return hex.EncodeToString(h[:])
}
