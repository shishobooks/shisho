package sidecar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

const SidecarSuffix = ".metadata.json"

// BookSidecarPath returns the sidecar file path for a book.
// For directory-based books: {bookdir}/{dirname}.metadata.json.
// For root-level books: {dir}/{filename_without_ext}.metadata.json.
func BookSidecarPath(bookPath string) string {
	// Clean the path to normalize trailing slashes
	cleanPath := filepath.Clean(bookPath)

	info, err := os.Stat(cleanPath)
	if err != nil {
		// Path doesn't exist yet - determine intent from path structure
		ext := filepath.Ext(cleanPath)
		if ext == "" {
			// No extension = intended to be a directory
			// Return sidecar path INSIDE the future directory
			dirName := filepath.Base(cleanPath)
			return filepath.Join(cleanPath, dirName+SidecarSuffix)
		}
		// Has extension = intended to be a file
		dir := filepath.Dir(cleanPath)
		base := strings.TrimSuffix(filepath.Base(cleanPath), ext)
		return filepath.Join(dir, base+SidecarSuffix)
	}

	if info.IsDir() {
		// Directory-based book: use directory name
		dirName := filepath.Base(cleanPath)
		return filepath.Join(cleanPath, dirName+SidecarSuffix)
	}

	// Root-level book (single file): use filename without extension
	dir := filepath.Dir(cleanPath)
	base := strings.TrimSuffix(filepath.Base(cleanPath), filepath.Ext(cleanPath))
	return filepath.Join(dir, base+SidecarSuffix)
}

// FileSidecarPath returns the sidecar file path for a media file.
// Returns {filepath}.metadata.json.
func FileSidecarPath(filePath string) string {
	return filePath + SidecarSuffix
}

// BookSidecarExists checks if a book sidecar file exists.
func BookSidecarExists(bookPath string) bool {
	_, err := os.Stat(BookSidecarPath(bookPath))
	return err == nil
}

// FileSidecarExists checks if a file sidecar exists.
func FileSidecarExists(filePath string) bool {
	_, err := os.Stat(FileSidecarPath(filePath))
	return err == nil
}

// ReadBookSidecar reads and parses a book sidecar file.
// Returns nil, nil if the sidecar doesn't exist.
func ReadBookSidecar(bookPath string) (*BookSidecar, error) {
	sidecarPath := BookSidecarPath(bookPath)

	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}

	var s BookSidecar
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, errors.WithStack(err)
	}

	return &s, nil
}

// ReadFileSidecar reads and parses a file sidecar.
// Returns nil, nil if the sidecar doesn't exist.
func ReadFileSidecar(filePath string) (*FileSidecar, error) {
	sidecarPath := FileSidecarPath(filePath)

	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}

	var s FileSidecar
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, errors.WithStack(err)
	}

	return &s, nil
}

// WriteBookSidecar writes a book sidecar file.
// Note: The caller is responsible for ensuring the parent directory exists.
// For root-level files with OrganizeFileStructure enabled, the directory
// should be created before calling this function.
func WriteBookSidecar(bookPath string, s *BookSidecar) error {
	sidecarPath := BookSidecarPath(bookPath)

	// Ensure version is set
	if s.Version == 0 {
		s.Version = CurrentVersion
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	// Sidecar files should be readable by users and other applications
	return errors.WithStack(os.WriteFile(sidecarPath, data, 0644)) //nolint:gosec
}

// WriteFileSidecar writes a file sidecar.
func WriteFileSidecar(filePath string, s *FileSidecar) error {
	sidecarPath := FileSidecarPath(filePath)

	// Ensure version is set
	if s.Version == 0 {
		s.Version = CurrentVersion
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	// Sidecar files should be readable by users and other applications
	return errors.WithStack(os.WriteFile(sidecarPath, data, 0644)) //nolint:gosec
}

// BookSidecarFromModel creates a BookSidecar from a Book model.
func BookSidecarFromModel(book *models.Book) *BookSidecar {
	s := &BookSidecar{
		Version:     CurrentVersion,
		Title:       book.Title,
		SortTitle:   book.SortTitle,
		Subtitle:    book.Subtitle,
		Description: book.Description,
	}

	// Convert authors from Authors
	for _, author := range book.Authors {
		if author.Person != nil {
			s.Authors = append(s.Authors, AuthorMetadata{
				Name:      author.Person.Name,
				SortName:  author.Person.SortName,
				SortOrder: author.SortOrder,
				Role:      author.Role,
			})
		}
	}

	// Convert series from BookSeries
	for _, bs := range book.BookSeries {
		if bs.Series != nil {
			s.Series = append(s.Series, SeriesMetadata{
				Name:      bs.Series.Name,
				SortName:  bs.Series.SortName,
				Number:    bs.SeriesNumber,
				SortOrder: bs.SortOrder,
			})
		}
	}

	return s
}

// FileSidecarFromModel creates a FileSidecar from a File model.
func FileSidecarFromModel(file *models.File) *FileSidecar {
	s := &FileSidecar{
		Version:   CurrentVersion,
		URL:       file.URL,
		Publisher: nil,
		Imprint:   nil,
		Name:      file.Name,
		CoverPage: file.CoverPage,
	}

	// Set publisher name if available
	if file.Publisher != nil {
		s.Publisher = &file.Publisher.Name
	}

	// Set imprint name if available
	if file.Imprint != nil {
		s.Imprint = &file.Imprint.Name
	}

	// Format release date as ISO 8601 string (YYYY-MM-DD)
	if file.ReleaseDate != nil {
		dateStr := file.ReleaseDate.Format("2006-01-02")
		s.ReleaseDate = &dateStr
	}

	// Convert narrators from Narrators
	for _, narrator := range file.Narrators {
		if narrator.Person != nil {
			s.Narrators = append(s.Narrators, NarratorMetadata{
				Name:      narrator.Person.Name,
				SortName:  narrator.Person.SortName,
				SortOrder: narrator.SortOrder,
			})
		}
	}

	// Map identifiers
	if len(file.Identifiers) > 0 {
		s.Identifiers = make([]IdentifierMetadata, len(file.Identifiers))
		for i, id := range file.Identifiers {
			s.Identifiers[i] = IdentifierMetadata{
				Type:  id.Type,
				Value: id.Value,
			}
		}
	}

	// Convert chapters if loaded
	if len(file.Chapters) > 0 {
		s.Chapters = ChaptersFromModels(file.Chapters)
	}

	return s
}

// WriteBookSidecarFromModel writes a book sidecar from a Book model.
func WriteBookSidecarFromModel(book *models.Book) error {
	s := BookSidecarFromModel(book)
	return WriteBookSidecar(book.Filepath, s)
}

// WriteFileSidecarFromModel writes a file sidecar from a File model.
func WriteFileSidecarFromModel(file *models.File) error {
	s := FileSidecarFromModel(file)
	return WriteFileSidecar(file.Filepath, s)
}

// WriteFileSidecarWithChapters writes a file sidecar from a File model and chapters.
func WriteFileSidecarWithChapters(file *models.File, chapters []*models.Chapter) error {
	s := FileSidecarFromModel(file)
	s.Chapters = ChaptersFromModels(chapters)
	return WriteFileSidecar(file.Filepath, s)
}

// ChaptersFromModels converts model chapters to ChapterMetadata slice.
func ChaptersFromModels(chapters []*models.Chapter) []ChapterMetadata {
	if len(chapters) == 0 {
		return nil
	}

	result := make([]ChapterMetadata, len(chapters))
	for i, ch := range chapters {
		result[i] = ChapterMetadata{
			Title:            ch.Title,
			StartPage:        ch.StartPage,
			StartTimestampMs: ch.StartTimestampMs,
			Href:             ch.Href,
			Children:         ChaptersFromModels(ch.Children),
		}
	}
	return result
}

// ChaptersToModels converts ChapterMetadata slice to model chapters.
// Note: This creates chapter models without IDs - they should be inserted fresh.
func ChaptersToModels(chapters []ChapterMetadata) []*models.Chapter {
	if len(chapters) == 0 {
		return nil
	}

	result := make([]*models.Chapter, len(chapters))
	for i, ch := range chapters {
		result[i] = &models.Chapter{
			Title:            ch.Title,
			StartPage:        ch.StartPage,
			StartTimestampMs: ch.StartTimestampMs,
			Href:             ch.Href,
			Children:         ChaptersToModels(ch.Children),
		}
	}
	return result
}
