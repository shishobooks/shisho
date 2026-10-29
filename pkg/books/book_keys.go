package books

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// A Book's filepath is its key. A Book in a folder of its own is keyed by the
// folder. A Book without one, like a root-level single-file Book or a Book
// split out of a shared folder with organizing off, is keyed by one of its
// files. A library root is never a Book's folder.

// isLibraryRoot reports whether path is one of libraryPaths. Library paths are
// stored as entered, so both sides are cleaned.
func isLibraryRoot(path string, libraryPaths []*models.LibraryPath) bool {
	clean := filepath.Clean(path)
	for _, lp := range libraryPaths {
		if filepath.Clean(lp.Filepath) == clean {
			return true
		}
	}
	return false
}

// listLibraryPaths returns the paths of the library.
func (svc *Service) listLibraryPaths(ctx context.Context, libraryID int) ([]*models.LibraryPath, error) {
	var paths []*models.LibraryPath
	err := svc.db.NewSelect().
		Model(&paths).
		Where("library_id = ?", libraryID).
		Scan(ctx)
	return paths, errors.WithStack(err)
}

// bookOwnsDirectory reports whether book.Filepath is a folder on disk that
// can be removed with the book: a real directory that is not a library root
// and holds no other Book's files.
func (svc *Service) bookOwnsDirectory(ctx context.Context, book *models.Book) (bool, error) {
	if book.Filepath == "" {
		return false, nil
	}
	if info, err := os.Stat(book.Filepath); err != nil || !info.IsDir() {
		return false, nil
	}
	libraryPaths, err := svc.listLibraryPaths(ctx, book.LibraryID)
	if err != nil {
		return false, err
	}
	if isLibraryRoot(book.Filepath, libraryPaths) {
		return false, nil
	}
	return !svc.folderClaimedByOtherBook(ctx, book.LibraryID, book.ID)(book.Filepath), nil
}

// rekeyBooksAwayFromFiles gives each Book keyed by one of files another of its
// files as its key, so the files can leave the Book. Main files are preferred.
// A Book with no other file keeps its key; it is deleted once empty.
func rekeyBooksAwayFromFiles(ctx context.Context, db bun.IDB, files []*models.File) error {
	leaving := make([]int, 0, len(files))
	for _, f := range files {
		leaving = append(leaving, f.ID)
	}
	for _, f := range files {
		var remaining []*models.File
		err := db.NewSelect().
			Model(&remaining).
			Where("book_id = ?", f.BookID).
			Where("id NOT IN (?)", bun.List(leaving)).
			OrderExpr("file_role = ? DESC, id ASC", models.FileRoleMain).
			Limit(1).
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		if len(remaining) == 0 {
			continue
		}
		_, err = db.NewUpdate().
			Model((*models.Book)(nil)).
			Set("filepath = ?, updated_at = ?", remaining[0].Filepath, time.Now()).
			Where("id = ? AND filepath = ?", f.BookID, f.Filepath).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
