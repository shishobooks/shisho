package cbz

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/shishobooks/shisho/pkg/mediafile"
)

// DetectChapters detects chapters from a list of image file paths.
// Uses folder-based detection first, falls back to filename patterns.
func DetectChapters(files []string) []mediafile.ParsedChapter {
	// Phase 1: Try folder-based detection
	chapters := detectChaptersFromFolders(files)
	if len(chapters) > 0 {
		return chapters
	}

	// Phase 2: Fall back to filename pattern detection
	return detectChaptersFromFilenames(files)
}

// detectChaptersFromFolders detects chapters based on immediate parent directories.
// Returns empty slice if all files share the same parent or only one unique parent exists.
func detectChaptersFromFolders(files []string) []mediafile.ParsedChapter {
	if len(files) == 0 {
		return nil
	}

	// Group files by their immediate parent directory
	type chapterInfo struct {
		name      string
		firstPage int
		pageCount int
	}
	chapterMap := make(map[string]*chapterInfo)
	var chapterOrder []string

	for i, file := range files {
		parent := filepath.Dir(file)
		chapterName := filepath.Base(parent)

		// Root level files (parent = ".") have no chapter
		if parent == "." {
			chapterName = ""
		}

		if chapterName == "" {
			continue
		}

		if _, exists := chapterMap[parent]; !exists {
			chapterMap[parent] = &chapterInfo{
				name:      chapterName,
				firstPage: i,
			}
			chapterOrder = append(chapterOrder, parent)
		}
		chapterMap[parent].pageCount++
	}

	// If all files are in root or only one chapter folder, no chapters
	if len(chapterMap) <= 1 {
		return nil
	}

	// Build chapter list sorted by first page
	chapters := make([]mediafile.ParsedChapter, 0, len(chapterOrder))
	for _, parent := range chapterOrder {
		info := chapterMap[parent]
		startPage := info.firstPage
		chapters = append(chapters, mediafile.ParsedChapter{
			Title:     info.name,
			StartPage: &startPage,
		})
	}

	// Sort by start page
	sort.Slice(chapters, func(i, j int) bool {
		return *chapters[i].StartPage < *chapters[j].StartPage
	})

	return chapters
}

// chapterPattern matches "ch" or "chapter" followed by digits, case-insensitive.
var chapterPattern = regexp.MustCompile(`(?i)ch(?:apter)?[\s_-]*(\d+)`)

// detectChaptersFromFilenames detects chapters from filename patterns.
// Only used when all files share the same parent directory.
func detectChaptersFromFilenames(files []string) []mediafile.ParsedChapter {
	if len(files) == 0 {
		return nil
	}

	// Track chapter numbers and their first occurrence
	type chapterInfo struct {
		number    int
		firstPage int
	}
	chapterMap := make(map[int]*chapterInfo)

	for i, file := range files {
		filename := filepath.Base(file)
		matches := chapterPattern.FindStringSubmatch(filename)
		if matches == nil {
			continue
		}

		chNum, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		if _, exists := chapterMap[chNum]; !exists {
			chapterMap[chNum] = &chapterInfo{
				number:    chNum,
				firstPage: i,
			}
		}
	}

	// Need at least 2 chapters to be meaningful
	if len(chapterMap) < 2 {
		return nil
	}

	// Convert to sorted slice
	chapters := make([]mediafile.ParsedChapter, 0, len(chapterMap))
	for _, info := range chapterMap {
		startPage := info.firstPage
		chapters = append(chapters, mediafile.ParsedChapter{
			Title:     fmt.Sprintf("Chapter %d", info.number),
			StartPage: &startPage,
		})
	}

	// Sort by start page
	sort.Slice(chapters, func(i, j int) bool {
		return *chapters[i].StartPage < *chapters[j].StartPage
	})

	return chapters
}
