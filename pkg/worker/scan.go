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
	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
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

func (w *Worker) ProcessScanJob(ctx context.Context, _ *models.Job) error {
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

		// After scanning all files, generate canonical covers for each book
		err = w.generateCanonicalCovers(ctx, library.ID)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// TODO: go through and delete files/books that have been deleted

	log.Info("finished scan job")
	return nil
}

func (w *Worker) scanFile(ctx context.Context, path string, libraryID int) error {
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

	// Determine if this is a root-level file by checking if the file is directly in a library path
	tempBookPath := filepath.Dir(path)
	filename := filepath.Base(tempBookPath)
	isRootLevelFile := false

	// Get the library to check its paths
	library, err := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check if file is directly in any library path
	for _, libraryPath := range library.LibraryPaths {
		if tempBookPath == libraryPath.Filepath {
			isRootLevelFile = true
			break
		}
	}

	// Set book path based on whether this is a root-level file
	var bookPath string
	if isRootLevelFile {
		// For root-level files, each file is its own book - use the full file path
		bookPath = path
		filename = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	} else {
		// For directory-based files, use the directory path (multiple files can belong to same book)
		bookPath = tempBookPath
	}

	title := strings.TrimSpace(filepathNarratorRE.ReplaceAllString(filepathAuthorRE.ReplaceAllString(filename, ""), ""))
	titleSource := models.DataSourceFilepath
	authors := make([]*models.Author, 0)
	authorSource := models.DataSourceFilepath
	series := ""
	seriesSource := models.DataSourceFilepath
	var seriesNumber *float64
	var coverMimeType *string
	var coverSource *string

	// Extract metadata from each file based on its file type.
	var metadata *mediafile.ParsedMetadata
	switch fileType {
	case models.FileTypeEPUB:
		log.Info("parsing file as epub", logger.Data{"file_type": fileType})
		metadata, err = epub.Parse(path)
		if err != nil {
			return errors.WithStack(err)
		}
	case models.FileTypeCBZ:
		log.Info("parsing file as cbz", logger.Data{"file_type": fileType})
		metadata, err = cbz.Parse(path)
		if err != nil {
			return errors.WithStack(err)
		}
	case models.FileTypeM4B:
		log.Info("parsing file as m4b", logger.Data{"file_type": fileType})
		metadata, err = mp4.Parse(path)
		if err != nil {
			// TODO: save this as a job log so we can surface in the UI
			log.Error("failed to parse as m4b", logger.Data{"file_type": fileType, "error": err.Error()})
			return nil
		}
	}

	if metadata != nil {
		title = metadata.Title
		titleSource = metadata.DataSource
		authorSource = metadata.DataSource
		series = metadata.Series
		seriesSource = metadata.DataSource
		seriesNumber = metadata.SeriesNumber
		for _, author := range metadata.Authors {
			authors = append(authors, &models.Author{
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
				authors = append(authors, &models.Author{
					Name: author,
				})
			}
		}
	}

	// Normalize volume indicators in title for CBZ files (after all metadata extraction)
	if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(title, fileType); hasVolume {
		title = normalizedTitle
	}

	// First, check if there's already a book for this file path
	// This covers the case where a file was previously organized but moved back to root level
	existingBookByFile, err := w.bookService.RetrieveBookByFilePath(ctx, path, libraryID)
	if err != nil && !errors.Is(err, errcodes.NotFound("Book")) {
		return errors.WithStack(err)
	}

	// Check if we should organize this file (only for root-level files when organize_file_structure is enabled)
	var organizeResult *fileutils.OrganizeFileResult
	if library.OrganizeFileStructure && isRootLevelFile {
		// If there's already a book for this exact file path, skip organization to avoid duplicates
		if existingBookByFile != nil {
			log.Info("skipping organization - book already exists for this file", logger.Data{
				"book_id":   existingBookByFile.ID,
				"file_path": path,
			})
		} else {
			log.Info("organizing root-level file", logger.Data{
				"organize_file_structure": library.OrganizeFileStructure,
				"is_root_level":           isRootLevelFile,
			})

			// Create organized name options
			organizeOpts := fileutils.OrganizedNameOptions{
				Authors:      authors,
				Title:        title,
				SeriesNumber: seriesNumber,
				FileType:     fileType,
			}

			// Organize the file
			organizeResult, err = fileutils.OrganizeRootLevelFile(path, organizeOpts)
			if err != nil {
				log.Error("failed to organize file", logger.Data{"error": err.Error()})
				// Don't fail the entire scan if organization fails
			} else if organizeResult.Moved {
				logData := logger.Data{
					"original_path":  organizeResult.OriginalPath,
					"new_path":       organizeResult.NewPath,
					"folder_created": organizeResult.FolderCreated,
					"covers_moved":   organizeResult.CoversMoved,
				}
				if organizeResult.CoversError != nil {
					logData["covers_error"] = organizeResult.CoversError.Error()
				}
				log.Info("file organized", logData)

				// Update path and book path for further processing
				path = organizeResult.NewPath
				bookPath = filepath.Dir(path)
			}
		}
	}

	// Determine which existing book to use
	var existingBook *models.Book
	if existingBookByFile != nil {
		// If we found a book by the original file path, use that
		existingBook = existingBookByFile
		log.Info("using existing book found by file path", logger.Data{"book_id": existingBook.ID})
	} else {
		// Otherwise, check for existing book by the final book path (after potential organization)
		existingBook, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
			Filepath:  &bookPath,
			LibraryID: &libraryID,
		})
		if err != nil && !errors.Is(err, errcodes.NotFound("Book")) {
			return errors.WithStack(err)
		}
	}

	if existingBook != nil {
		log.Info("book already exists", logger.Data{"book_id": existingBook.ID})

		// Check to see if we need to update any of the metadata on the book.
		updateOptions := books.UpdateBookOptions{Columns: make([]string, 0)}
		if models.DataSourcePriority[titleSource] < models.DataSourcePriority[existingBook.TitleSource] && existingBook.Title != title {
			log.Info("updating title", logger.Data{"new_title": title, "old_title": existingBook.Title})
			existingBook.Title = title
			existingBook.TitleSource = titleSource
			updateOptions.Columns = append(updateOptions.Columns, "title", "title_source")
		}
		if models.DataSourcePriority[authorSource] < models.DataSourcePriority[existingBook.AuthorSource] {
			log.Info("updating authors", logger.Data{"new_author_count": len(authors), "old_author_count": len(existingBook.Authors)})
			existingBook.Authors = authors
			existingBook.AuthorSource = authorSource
			updateOptions.UpdateAuthors = true
		}
		// Update series if we have a higher priority source
		if series != "" && (existingBook.SeriesSource == nil || models.DataSourcePriority[seriesSource] < models.DataSourcePriority[*existingBook.SeriesSource]) {
			log.Info("updating series", logger.Data{"new_series": series, "old_series": existingBook.Series})
			existingBook.Series = &series
			existingBook.SeriesSource = &seriesSource
			updateOptions.Columns = append(updateOptions.Columns, "series", "series_source")
		}
		if seriesNumber != nil && (existingBook.SeriesSource == nil || models.DataSourcePriority[seriesSource] < models.DataSourcePriority[*existingBook.SeriesSource]) {
			log.Info("updating series number", logger.Data{"new_series_number": *seriesNumber, "old_series_number": existingBook.SeriesNumber})
			existingBook.SeriesNumber = seriesNumber
			updateOptions.Columns = append(updateOptions.Columns, "series_number")
		}

		// If file was organized and we're using an existing book by file path,
		// update the book and file paths to reflect the new locations
		if organizeResult != nil && organizeResult.Moved && existingBookByFile != nil {
			log.Info("updating book and file paths after organization", logger.Data{
				"old_book_path": existingBook.Filepath,
				"new_book_path": bookPath,
				"old_file_path": organizeResult.OriginalPath,
				"new_file_path": organizeResult.NewPath,
			})

			// Update book filepath
			existingBook.Filepath = bookPath
			updateOptions.Columns = append(updateOptions.Columns, "filepath")

			// Also need to update the file record
			for _, file := range existingBook.Files {
				if file.Filepath == organizeResult.OriginalPath {
					file.Filepath = organizeResult.NewPath
					err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
						Columns: []string{"filepath"},
					})
					if err != nil {
						log.Error("failed to update file path", logger.Data{
							"file_id": file.ID,
							"error":   err.Error(),
						})
					}
					break
				}
			}
		}

		err := w.bookService.UpdateBook(ctx, existingBook, updateOptions)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		log.Info("creating book", logger.Data{"title": title})
		var seriesPtr *string
		var seriesSourcePtr *string
		if series != "" {
			seriesPtr = &series
			seriesSourcePtr = &seriesSource
		}
		existingBook = &models.Book{
			LibraryID:    libraryID,
			Filepath:     bookPath,
			Title:        title,
			TitleSource:  titleSource,
			Authors:      authors,
			AuthorSource: authorSource,
			Series:       seriesPtr,
			SeriesSource: seriesSourcePtr,
			SeriesNumber: seriesNumber,
		}
		err := w.bookService.CreateBook(ctx, existingBook)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Handle cover extraction before creating the file so we can set the cover source
	if metadata != nil && len(metadata.CoverData) > 0 {
		// Save the cover image as a separate file using filename_cover.ext format
		baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		coverFilepath := filepath.Join(bookPath, baseName+"_cover"+metadata.CoverExtension())

		// Check if cover already exists before extracting a new one
		if _, err := os.Stat(coverFilepath); os.IsNotExist(err) {
			log.Info("saving cover", logger.Data{"mime": metadata.CoverMimeType})
			coverFile, err := os.Create(coverFilepath)
			if err != nil {
				return errors.WithStack(err)
			}
			defer coverFile.Close()
			_, err = io.Copy(coverFile, bytes.NewReader(metadata.CoverData))
			if err != nil {
				return errors.WithStack(err)
			}
			// Set cover source to the metadata source since we extracted it
			coverSource = &metadata.DataSource
		} else {
			log.Info("cover already exists, skipping extraction", logger.Data{"cover_path": coverFilepath})
			// Set cover source to existing cover since we're using the existing one
			existingCoverSource := models.DataSourceExistingCover
			coverSource = &existingCoverSource
		}
	}

	log.Info("creating file", logger.Data{"filesize": size})
	file := &models.File{
		LibraryID:     libraryID,
		BookID:        existingBook.ID,
		Filepath:      path,
		FileType:      fileType,
		FilesizeBytes: size,
		CoverMimeType: coverMimeType,
		CoverSource:   coverSource,
	}
	err = w.bookService.CreateFile(ctx, file)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (w *Worker) generateCanonicalCovers(ctx context.Context, libraryID int) error {
	log := logger.FromContext(ctx)
	log.Info("generating canonical covers for library", logger.Data{"library_id": libraryID})

	// Get all books in this library
	allBooks, err := w.bookService.ListBooks(ctx, books.ListBooksOptions{
		LibraryID: &libraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	for _, book := range allBooks {
		err := w.generateBookCanonicalCover(ctx, book)
		if err != nil {
			log.Error("failed to generate canonical cover for book", logger.Data{"book_id": book.ID, "error": err.Error()})
			// Don't fail the entire job if one book fails
			continue
		}
	}

	return nil
}

func (w *Worker) generateBookCanonicalCover(ctx context.Context, book *models.Book) error {
	log := logger.FromContext(ctx).Data(logger.Data{"book_id": book.ID, "book_path": book.Filepath})

	// Get all files for this book with cover data
	bookWithFiles, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		ID: &book.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check if this is a root-level book by seeing if any file is directly in a library path
	isRootLevelBook := false
	if len(bookWithFiles.Files) > 0 {
		// Get the library to check its paths
		library, err := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
			ID: &book.LibraryID,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		// Check if any file is directly in a library path
		for _, file := range bookWithFiles.Files {
			fileDir := filepath.Dir(file.Filepath)
			for _, libraryPath := range library.LibraryPaths {
				if fileDir == libraryPath.Filepath {
					isRootLevelBook = true
					break
				}
			}
			if isRootLevelBook {
				break
			}
		}
	}

	var bookCoverSource, audiobookCoverSource string
	var bookCoverExt, audiobookCoverExt string

	// Find the best covers from the available files
	for _, file := range bookWithFiles.Files {
		if file.CoverMimeType == nil {
			continue
		}

		baseName := strings.TrimSuffix(filepath.Base(file.Filepath), filepath.Ext(file.Filepath))
		var individualCoverPath string
		if isRootLevelBook {
			// For root-level books, cover is in the same directory as the file
			individualCoverPath = filepath.Join(filepath.Dir(file.Filepath), baseName+"_cover"+file.CoverExtension())
		} else {
			// For directory-based books, cover is in the book directory
			individualCoverPath = filepath.Join(book.Filepath, baseName+"_cover"+file.CoverExtension())
		}

		// Check if the individual cover file exists
		if _, err := os.Stat(individualCoverPath); err != nil {
			continue
		}

		// Determine cover type based on file type
		switch file.FileType {
		case models.FileTypeEPUB, models.FileTypeCBZ:
			if bookCoverSource == "" {
				bookCoverSource = individualCoverPath
				bookCoverExt = file.CoverExtension()
			}
		case models.FileTypeM4B:
			if audiobookCoverSource == "" {
				audiobookCoverSource = individualCoverPath
				audiobookCoverExt = file.CoverExtension()
			}
		}
	}

	// For root-level books, the canonical cover is just the individual cover filename
	// For directory-based books, generate separate canonical cover files
	var canonicalCover string
	if isRootLevelBook {
		// For root-level files, canonical cover is just the individual cover filename
		if bookCoverSource != "" {
			canonicalCover = filepath.Base(bookCoverSource)
			log.Info("using individual cover as canonical for root-level book", logger.Data{"canonical": canonicalCover})
		} else if audiobookCoverSource != "" {
			canonicalCover = filepath.Base(audiobookCoverSource)
			log.Info("using individual audiobook cover as canonical for root-level book", logger.Data{"canonical": canonicalCover})
		}
	} else {
		// Directory-based books: generate separate canonical cover files
		if bookCoverSource != "" {
			canonicalCover = "cover" + bookCoverExt
			err := w.copyFile(bookCoverSource, filepath.Join(book.Filepath, canonicalCover))
			if err != nil {
				return errors.WithStack(err)
			}
			log.Info("generated canonical book cover", logger.Data{"source": bookCoverSource, "canonical": canonicalCover})
		} else if audiobookCoverSource != "" {
			canonicalCover = "audiobook_cover" + audiobookCoverExt
			err := w.copyFile(audiobookCoverSource, filepath.Join(book.Filepath, canonicalCover))
			if err != nil {
				return errors.WithStack(err)
			}
			log.Info("generated canonical audiobook cover", logger.Data{"source": audiobookCoverSource, "canonical": canonicalCover})
		}
	}

	// Update the book's cover_image_path if we generated a canonical cover
	if canonicalCover != "" {
		book.CoverImagePath = &canonicalCover
		err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{
			Columns: []string{"cover_image_path"},
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (w *Worker) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return errors.WithStack(err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return errors.WithStack(err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return errors.WithStack(err)
}
