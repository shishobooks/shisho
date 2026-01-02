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
	info, err := os.Stat(bookPath)
	if err != nil {
		// If we can't stat, assume it's a file path
		dir := filepath.Dir(bookPath)
		base := strings.TrimSuffix(filepath.Base(bookPath), filepath.Ext(bookPath))
		return filepath.Join(dir, base+SidecarSuffix)
	}

	if info.IsDir() {
		// Directory-based book: use directory name
		dirName := filepath.Base(bookPath)
		return filepath.Join(bookPath, dirName+SidecarSuffix)
	}

	// Root-level book (single file): use filename without extension
	dir := filepath.Dir(bookPath)
	base := strings.TrimSuffix(filepath.Base(bookPath), filepath.Ext(bookPath))
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
		Version:  CurrentVersion,
		Title:    book.Title,
		Subtitle: book.Subtitle,
	}

	// Convert authors from Authors
	for _, author := range book.Authors {
		if author.Person != nil {
			s.Authors = append(s.Authors, AuthorMetadata{
				Name:      author.Person.Name,
				SortOrder: author.SortOrder,
			})
		}
	}

	// Convert series from BookSeries
	for _, bs := range book.BookSeries {
		if bs.Series != nil {
			s.Series = append(s.Series, SeriesMetadata{
				Name:      bs.Series.Name,
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
		Version: CurrentVersion,
	}

	// Convert narrators from Narrators
	for _, narrator := range file.Narrators {
		if narrator.Person != nil {
			s.Narrators = append(s.Narrators, NarratorMetadata{
				Name:      narrator.Person.Name,
				SortOrder: narrator.SortOrder,
			})
		}
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
