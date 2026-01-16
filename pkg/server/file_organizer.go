package server

import (
	"context"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/uptrace/bun"
)

// fileOrganizer implements people.FileOrganizer interface.
// It bridges the people package to the books and libraries packages.
type fileOrganizer struct {
	db             *bun.DB
	bookService    *books.Service
	libraryService *libraries.Service
}

// NewFileOrganizer creates a new FileOrganizer implementation.
func NewFileOrganizer(db *bun.DB) people.FileOrganizer {
	return &fileOrganizer{
		db:             db,
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
	}
}

// GetLibraryOrganizeSetting checks if a library has OrganizeFileStructure enabled.
func (fo *fileOrganizer) GetLibraryOrganizeSetting(ctx context.Context, libraryID int) (bool, error) {
	library, err := fo.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return false, errors.WithStack(err)
	}
	return library.OrganizeFileStructure, nil
}

// OrganizeBookFiles reorganizes files for a book with the given ID.
func (fo *fileOrganizer) OrganizeBookFiles(ctx context.Context, bookID int) error {
	// Retrieve the book with authors
	book, err := fo.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		ID: &bookID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Use the book service's UpdateBook with OrganizeFiles flag
	// This triggers the existing organizeBookFiles logic
	return fo.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{
		OrganizeFiles: true,
	})
}

// RenameNarratedFile renames an M4B file to include the updated narrator name.
func (fo *fileOrganizer) RenameNarratedFile(ctx context.Context, fileID int) (string, error) {
	log := logger.FromContext(ctx)

	// Retrieve the file with its book and narrators
	file, err := fo.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
		ID: &fileID,
	})
	if err != nil {
		return "", errors.WithStack(err)
	}

	// Only process M4B files
	if file.FileType != models.FileTypeM4B {
		return file.Filepath, nil
	}

	// Get the library to check OrganizeFileStructure
	library, err := fo.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &file.LibraryID,
	})
	if err != nil {
		return file.Filepath, errors.WithStack(err)
	}

	if !library.OrganizeFileStructure {
		return file.Filepath, nil
	}

	// Get the book to get author names
	book, err := fo.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		ID: &file.BookID,
	})
	if err != nil {
		return file.Filepath, errors.WithStack(err)
	}

	// Build author names
	authorNames := make([]string, 0, len(book.Authors))
	for _, a := range book.Authors {
		if a.Person != nil {
			authorNames = append(authorNames, a.Person.Name)
		}
	}

	// Build narrator names from the file's narrators
	narratorNames := make([]string, 0, len(file.Narrators))
	for _, n := range file.Narrators {
		if n.Person != nil {
			narratorNames = append(narratorNames, n.Person.Name)
		}
	}

	// Use file.Name for title if available, otherwise book.Title
	title := book.Title
	if file.Name != nil && *file.Name != "" {
		title = *file.Name
	}

	// Generate organized name options
	organizeOpts := fileutils.OrganizedNameOptions{
		AuthorNames:   authorNames,
		NarratorNames: narratorNames,
		Title:         title,
		FileType:      file.FileType,
	}

	// Rename the file
	// Use RenameOrganizedFileForSupplement to avoid renaming the book sidecar.
	// Narrator changes are file-level, not book-level.
	newPath, err := fileutils.RenameOrganizedFileOnly(file.Filepath, organizeOpts)
	if err != nil {
		return file.Filepath, errors.WithStack(err)
	}

	if newPath != file.Filepath {
		log.Info("renamed narrated file after person name change", logger.Data{
			"file_id":  file.ID,
			"old_path": file.Filepath,
			"new_path": newPath,
		})

		// Update file path in database
		now := time.Now()
		_, err = fo.db.NewUpdate().
			Model((*models.File)(nil)).
			Set("filepath = ?, updated_at = ?", newPath, now).
			Where("id = ?", file.ID).
			Exec(ctx)
		if err != nil {
			// Log error but return the new path since the file was renamed
			log.Error("failed to update file path in database after rename", logger.Data{
				"file_id":  file.ID,
				"new_path": newPath,
				"error":    err.Error(),
			})
		}

		// Update cover path if it exists
		if file.CoverImagePath != nil {
			newCoverPath := filepath.Base(fileutils.ComputeNewCoverPath(*file.CoverImagePath, newPath))
			_, err = fo.db.NewUpdate().
				Model((*models.File)(nil)).
				Set("cover_image_path = ?", newCoverPath).
				Where("id = ?", file.ID).
				Exec(ctx)
			if err != nil {
				log.Warn("failed to update cover path in database after rename", logger.Data{
					"file_id":        file.ID,
					"new_cover_path": newCoverPath,
					"error":          err.Error(),
				})
			}
		}
	}

	return newPath, nil
}
