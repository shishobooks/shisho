package worker

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/mp4"
)

var extensionsToScan = map[string]map[string]struct{}{
	".epub": {"application/epub+zip": {}},
	".m4b":  {"audio/x-m4a": {}, "video/mp4": {}},
	".cbz":  {"application/zip": {}},
}

var (
	filepathAuthorRE   = regexp.MustCompile(`\[(.*)]`)
	filepathNarratorRE = regexp.MustCompile(`\{(.*)}`)
)

func (w *Worker) ProcessScanJob(ctx context.Context, _ *jobs.Job) error {
	log := logger.FromContext(ctx)
	log.Info("processing scan job")

	allLibraries, err := w.libraryService.ListLibraries(ctx, libraries.ListLibrariesOptions{})
	if err != nil {
		return errors.WithStack(err)
	}

	log.Info("processing libraries", logger.Data{"count": len(allLibraries)})

	for _, library := range allLibraries {
		log.Info("processing library", logger.Data{"library_id": library.ID})
		filesToScan := make([]string, 0)

		// Go through all the library paths to find all the .cbz files.
		for _, libraryPath := range library.LibraryPaths {
			log := log.Data(logger.Data{"library_path_id": libraryPath.ID, "library_path": libraryPath.Filepath})
			log.Info("processing library path")
			err := filepath.WalkDir(libraryPath.Filepath, func(path string, info fs.DirEntry, err error) error {
				if err != nil {
					return errors.WithStack(err)
				}
				if info.IsDir() {
					// We don't do anything explicitly to directories.
					return nil
				}
				// TODO: support having cover.jpg and cover_audiobook.jpg
				expectedMimeTypes, ok := extensionsToScan[filepath.Ext(path)]
				if !ok {
					// We're only looking for certain files right now.
					return nil
				}
				mtype, err := mimetype.DetectFile(path)
				if err != nil {
					// We can't detect the mime type, so we just skip it.
					log.Warn("can't detect the mime type of a file with a valid extension", logger.Data{"path": path, "err": err.Error()})
					return nil
				}
				if _, ok := expectedMimeTypes[mtype.String()]; !ok {
					// Since files can have any extension, we try to check it against the mime type that we expect it to
					// be. This might be overly restrictive in the future, so it might be something that we remove, but
					// we can keep it for now.
					log.Warn("mime type is not expected for extension", logger.Data{"path": path, "mimetype": mtype.String()})
					return nil
				}

				// This is a file that we care about, so store it in the slice. We do this so that we can know the total
				// number of files that we need to scan before we start doing any real work so that we can accurately
				// update the progress of the job.
				filesToScan = append(filesToScan, path)

				return nil
			})
			if err != nil {
				return errors.WithStack(err)
			}
		}

		for _, path := range filesToScan {
			err := w.scanFile(ctx, path, library.ID)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}

	// TODO: go through and delete files/books that have been deleted

	log.Info("finished scan job")
	return nil
}

func (w *Worker) scanFile(ctx context.Context, path string, libraryID string) error {
	log := logger.FromContext(ctx).Data(logger.Data{"path": path})
	log.Info("processing file")

	// Check if this file already exists based on its filepath.
	existingFile, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
		Filepath:  &path,
		LibraryID: &libraryID,
	})
	if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
		return errors.WithStack(err)
	}
	if existingFile != nil {
		log.Info("file already exists", logger.Data{"file_id": existingFile.ID})
		return nil
	}

	// Get the size of the file.
	stats, err := os.Stat(path)
	if err != nil {
		return errors.WithStack(err)
	}
	size := stats.Size()
	fileType := strings.ToLower(strings.ReplaceAll(filepath.Ext(path), ".", ""))
	bookPath := filepath.Dir(path)
	filename := filepath.Base(bookPath)

	title := strings.TrimSpace(filepathNarratorRE.ReplaceAllString(filepathAuthorRE.ReplaceAllString(filename, ""), ""))
	titleSource := books.DataSourceFilepath
	authors := make([]*books.Author, 0)
	authorSource := books.DataSourceFilepath
	var coverMimeType *string

	// Extract metadata from each file based on its file type.
	var metadata *mediafile.ParsedMetadata
	switch fileType {
	case books.FileTypeEPUB:
		log.Info("parsing file as epub", logger.Data{"file_type": fileType})
		metadata, err = epub.Parse(path)
		if err != nil {
			return errors.WithStack(err)
		}
	case books.FileTypeM4B:
		log.Info("parsing file as m4b", logger.Data{"file_type": fileType})
		metadata, err = mp4.Parse(path)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if metadata != nil {
		title = metadata.Title
		titleSource = metadata.DataSource
		authorSource = metadata.DataSource
		for _, author := range metadata.Authors {
			authors = append(authors, &books.Author{
				Name: author,
			})
		}
		if metadata.CoverMimeType != "" {
			coverMimeType = &metadata.CoverMimeType
		}
	}

	// If we didn't find any authors in the metadata, try getting it from the filename.
	if len(authors) == 0 && filepathAuthorRE.MatchString(filename) {
		log.Info("no authors found in metadata; parsing filename", logger.Data{"filename": filename})
		matches := filepathAuthorRE.FindAllString(filename, -1)
		if len(matches) > 0 {
			names := strings.Split(matches[0], ",")
			for _, author := range names {
				authors = append(authors, &books.Author{
					Name: author,
				})
			}
		}
	}

	existingBook, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		Filepath:  &bookPath,
		LibraryID: &libraryID,
	})
	if err != nil && !errors.Is(err, errcodes.NotFound("Book")) {
		return errors.WithStack(err)
	}
	if existingBook != nil {
		log.Info("book already exists", logger.Data{"book_id": existingBook.ID})

		// Check to see if we need to update any of the metadata on the book.
		updateOptions := books.UpdateBookOptions{Columns: make([]string, 0)}
		if books.DataSourcePriority[titleSource] < books.DataSourcePriority[existingBook.TitleSource] && existingBook.Title != title {
			log.Info("updating title", logger.Data{"new_title": title, "old_title": existingBook.Title})
			existingBook.Title = title
			existingBook.TitleSource = titleSource
			updateOptions.Columns = append(updateOptions.Columns, "title", "title_source")
		}
		if books.DataSourcePriority[authorSource] < books.DataSourcePriority[existingBook.AuthorSource] {
			log.Info("updating authors", logger.Data{"new_author_count": len(authors), "old_author_count": len(existingBook.Authors)})
			existingBook.Authors = authors
			existingBook.AuthorSource = authorSource
			updateOptions.UpdateAuthors = true
		}

		err := w.bookService.UpdateBook(ctx, existingBook, updateOptions)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		log.Info("creating book", logger.Data{"title": title})
		existingBook = &books.Book{
			LibraryID:    libraryID,
			Filepath:     bookPath,
			Title:        title,
			TitleSource:  titleSource,
			Authors:      authors,
			AuthorSource: authorSource,
		}
		err := w.bookService.CreateBook(ctx, existingBook)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	log.Info("creating file", logger.Data{"filesize": size})
	file := &books.File{
		LibraryID:     libraryID,
		BookID:        existingBook.ID,
		Filepath:      path,
		FileType:      fileType,
		FilesizeBytes: size,
		CoverMimeType: coverMimeType,
	}
	err = w.bookService.CreateFile(ctx, file)
	if err != nil {
		return errors.WithStack(err)
	}

	if metadata != nil && len(metadata.CoverData) > 0 {
		log.Info("saving cover", logger.Data{"mime": metadata.CoverMimeType})
		// Save the cover image as a separate file.
		coverFilepath := filepath.Join(bookPath, file.ID+metadata.CoverExtension())
		coverFile, err := os.Create(coverFilepath)
		if err != nil {
			return errors.WithStack(err)
		}
		defer coverFile.Close()
		_, err = io.Copy(coverFile, bytes.NewReader(metadata.CoverData))
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
