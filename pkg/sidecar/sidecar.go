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

// ErrEmptySidecarPath is returned by Write* helpers when the caller supplies
// an empty book or file path. An empty path would otherwise resolve to the
// current working directory via filepath.Clean("") → ".", which historically
// caused stray "..metadata.json" files to be written next to whatever process
// was running (including Go test binaries).
var ErrEmptySidecarPath = errors.New("sidecar: path is empty")

// BookSidecarPath returns the sidecar file path for a book.
// For directory-based books: {bookdir}/{dirname}.metadata.json.
// For root-level books: {dir}/{filename_without_ext}.metadata.json.
// Returns "" when bookPath is empty — an empty bookPath has no meaningful
// sidecar location and must not be coerced into a CWD-relative path.
func BookSidecarPath(bookPath string) string {
	if bookPath == "" {
		return ""
	}
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
// Returns {filepath}.metadata.json, or "" when filePath is empty.
func FileSidecarPath(filePath string) string {
	if filePath == "" {
		return ""
	}
	return filePath + SidecarSuffix
}

// BookSidecarExists checks if a book sidecar file exists.
func BookSidecarExists(bookPath string) bool {
	sidecarPath := BookSidecarPath(bookPath)
	if sidecarPath == "" {
		return false
	}
	_, err := os.Stat(sidecarPath)
	return err == nil
}

// FileSidecarExists checks if a file sidecar exists.
func FileSidecarExists(filePath string) bool {
	sidecarPath := FileSidecarPath(filePath)
	if sidecarPath == "" {
		return false
	}
	_, err := os.Stat(sidecarPath)
	return err == nil
}

// ReadBookSidecar reads and parses a book sidecar file.
// Returns nil, nil if the sidecar doesn't exist or bookPath is empty.
func ReadBookSidecar(bookPath string) (*BookSidecar, error) {
	sidecarPath := BookSidecarPath(bookPath)
	if sidecarPath == "" {
		return nil, nil
	}

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

// ReadBookSidecarFromModel reads the book sidecar for a Book, using the same
// anchor-resolution logic as WriteBookSidecarFromModel. For root-level books
// with a synthetic book.Filepath, the sidecar is read from next to a file in
// the book instead of from the non-existent synthetic directory. The optional
// fileHint is used as a fallback anchor when book.Files is not populated —
// pass the current file being scanned so resolution works even before the
// book has been reloaded with its files relation.
func ReadBookSidecarFromModel(book *models.Book, fileHint *models.File) (*BookSidecar, error) {
	anchor := resolveBookSidecarAnchor(book)
	// If the anchor still resolves to a non-existent directory (no files on
	// the book model yet), fall back to the hint's filepath.
	if anchor != "" {
		if info, err := os.Stat(anchor); err != nil || (!info.IsDir() && filepath.Ext(anchor) == "") {
			if fileHint != nil && fileHint.Filepath != "" {
				anchor = fileHint.Filepath
			}
		}
	} else if fileHint != nil && fileHint.Filepath != "" {
		anchor = fileHint.Filepath
	}
	return ReadBookSidecar(anchor)
}

// ReadFileSidecar reads and parses a file sidecar.
// Returns nil, nil if the sidecar doesn't exist or filePath is empty.
func ReadFileSidecar(filePath string) (*FileSidecar, error) {
	sidecarPath := FileSidecarPath(filePath)
	if sidecarPath == "" {
		return nil, nil
	}

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
// Returns ErrEmptySidecarPath if bookPath is empty.
func WriteBookSidecar(bookPath string, s *BookSidecar) error {
	sidecarPath := BookSidecarPath(bookPath)
	if sidecarPath == "" {
		return errors.WithStack(ErrEmptySidecarPath)
	}

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
// Returns ErrEmptySidecarPath if filePath is empty.
func WriteFileSidecar(filePath string, s *FileSidecar) error {
	sidecarPath := FileSidecarPath(filePath)
	if sidecarPath == "" {
		return errors.WithStack(ErrEmptySidecarPath)
	}

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
		Language:  file.Language,
		Abridged:  file.Abridged,
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
//
// For root-level books in libraries with OrganizeFileStructure disabled,
// the scanner writes a synthetic organized-folder path into book.Filepath
// that never exists on disk. Writing a sidecar to that path would fail, so
// if book.Filepath doesn't resolve to an existing directory we fall back to
// anchoring the sidecar next to a file in the book (preferring a main file).
// The cover always lives next to the file in that case, so co-locating the
// book sidecar matches how the rest of the system resolves paths.
func WriteBookSidecarFromModel(book *models.Book) error {
	s := BookSidecarFromModel(book)
	return WriteBookSidecar(resolveBookSidecarAnchor(book), s)
}

// resolveBookSidecarAnchor returns the path to pass to BookSidecarPath for
// writing. Prefers book.Filepath when it resolves to an existing directory;
// otherwise falls back to a file in the book. Returns an empty string only
// when both book.Filepath and any file path are empty — WriteBookSidecar
// guards empty input.
func resolveBookSidecarAnchor(book *models.Book) string {
	if book == nil {
		return ""
	}
	if book.Filepath != "" {
		if info, err := os.Stat(book.Filepath); err == nil && info.IsDir() {
			return book.Filepath
		}
	}
	// Fall back to a file's path. Prefer a main file for stable naming.
	for _, f := range book.Files {
		if f != nil && f.FileRole == models.FileRoleMain && f.Filepath != "" {
			return f.Filepath
		}
	}
	for _, f := range book.Files {
		if f != nil && f.Filepath != "" {
			return f.Filepath
		}
	}
	return book.Filepath
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
