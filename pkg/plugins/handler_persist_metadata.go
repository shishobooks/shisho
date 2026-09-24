package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
)

func applyFieldSelected(overrides *ApplyOverrides, field string) bool {
	return overrides != nil && overrides.SelectedFields[field]
}

// equalIntSets reports whether two int-keyed sets contain the same keys.
func equalIntSets(a, b map[int]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// persistMetadata applies metadata to a book and its target file without plugin field filtering.
// Non-empty parsed values are persisted, and selected zero values in overrides clear metadata.
// pluginScope and pluginID identify the data source.
// targetFile is the specific file to apply file-level metadata (identifiers, cover) to; may be nil.
// The returned warnings describe selected values that were skipped rather
// than applied (currently only covers); they are user-facing and the caller
// surfaces them alongside the successful apply.
func (h *handler) persistMetadata(ctx context.Context, book *models.Book, targetFile *models.File, md *mediafile.ParsedMetadata, pluginScope, pluginID string, overrides *ApplyOverrides, log logger.Logger) ([]string, error) {
	var warnings []string
	pluginSource := models.PluginDataSource(pluginScope, pluginID)
	// A semantic no-op preserves provenance. Changed scalars, relationship
	// collections (including Series memberships), and identifiers use the
	// submitted intent. Covers have no edit state, so a chosen proposal is
	// always stamped with the plugin source (see the cover block below).
	attr := newApplyAttribution(pluginSource, overrides)
	var columns []string

	// Accumulate file-level column updates so Title/Narrator/Publisher/etc.
	// can all contribute, then flush once at the end.
	var fileColumns []string

	// Previous cover files that the image-based cover path superseded. They
	// are removed only after the column flush succeeds, so a failed write
	// never leaves the database pointing at a deleted file.
	var staleCovers []string

	// Track whether changes ran that would affect series_fts aggregate
	// columns (book_titles / book_authors). When they did, the series
	// block re-indexes the attached series even if its attachment to this
	// book didn't change — otherwise the aggregate columns would go stale
	// until something else triggered IndexSeries.
	seriesAggregateMayBeStale := false
	bookIndexChanged := false

	// Title
	title := strings.TrimSpace(md.Title)
	if title != "" && title != book.Title {
		book.Title = title
		book.TitleSource = attr.sourceFor("title")
		columns = append(columns, "title", "title_source")
		// Same convention as the Edit form: a derived sort title is regenerated
		// unless the user pinned it, and is never stamped manual or plugin, so
		// a later Title edit still regenerates it.
		if book.SortTitleSource != models.DataSourceManual {
			book.SortTitle = sortname.ForTitle(title)
			book.SortTitleSource = models.DataSourceFilepath
			columns = append(columns, "sort_title", "sort_title_source")
		}
		seriesAggregateMayBeStale = true
	}

	// Subtitle
	subtitle := strings.TrimSpace(md.Subtitle)
	if subtitle != "" || applyFieldSelected(overrides, "subtitle") {
		if applyOptionalString(subtitle, &book.Subtitle, &book.SubtitleSource, attr.sourceFor("subtitle")) {
			columns = append(columns, "subtitle", "subtitle_source")
		}
	}

	// Description
	desc := htmlutil.StripTags(strings.TrimSpace(md.Description))
	if desc != "" || applyFieldSelected(overrides, "description") {
		if applyOptionalString(desc, &book.Description, &book.DescriptionSource, attr.sourceFor("description")) {
			columns = append(columns, "description", "description_source")
		}
	}

	// Apply scalar column updates
	if len(columns) > 0 {
		bookIndexChanged = true
		if err := h.enrich.bookStore.UpdateBook(ctx, book, columns); err != nil {
			return nil, errors.Wrap(err, "failed to update book")
		}
	}

	// Authors
	if (len(md.Authors) > 0 || applyFieldSelected(overrides, "authors")) && h.enrich.relStore != nil && (len(md.Authors) == 0 || h.enrich.personFinder != nil) {
		changed, err := h.applyAuthors(ctx, book, md.Authors, attr, log)
		if err != nil {
			return nil, err
		}
		seriesAggregateMayBeStale = seriesAggregateMayBeStale || changed
		bookIndexChanged = bookIndexChanged || changed
	}

	// Series. The multi-entry path (Identify form) takes precedence over the
	// scalar path (plugin results). Both resolve, compare, and attribute the
	// membership collection the same way.
	var seriesEntries []SeriesEntry
	seriesTouched := false
	switch {
	case overrides != nil && overrides.SeriesEntries != nil:
		seriesEntries = *overrides.SeriesEntries
		seriesTouched = true
	case md.Series != "":
		seriesEntries = []SeriesEntry{{Name: md.Series, Number: md.SeriesNumber, NumberEnd: md.SeriesNumberEnd, SeriesNumberUnit: md.SeriesNumberUnit}}
		seriesTouched = true
	}
	if seriesTouched && h.enrich.relStore != nil {
		changed, err := h.applySeries(ctx, book, seriesEntries, attr, seriesAggregateMayBeStale, log)
		if err != nil {
			return nil, err
		}
		bookIndexChanged = bookIndexChanged || changed
	} else if seriesAggregateMayBeStale {
		h.indexSeries(ctx, attachedSeries(book), log)
	}

	// Genres
	if (len(md.Genres) > 0 || applyFieldSelected(overrides, "genres")) && h.enrich.relStore != nil && (len(md.Genres) == 0 || h.enrich.genreFinder != nil) {
		changed, err := h.applyGenres(ctx, book, md.Genres, attr, log)
		if err != nil {
			return nil, err
		}
		bookIndexChanged = bookIndexChanged || changed
	}

	// Tags
	if (len(md.Tags) > 0 || applyFieldSelected(overrides, "tags")) && h.enrich.relStore != nil && (len(md.Tags) == 0 || h.enrich.tagFinder != nil) {
		changed, err := h.applyTags(ctx, book, md.Tags, attr, log)
		if err != nil {
			return nil, err
		}
		bookIndexChanged = bookIndexChanged || changed
	}

	// Narrators (file-level, applied only to M4B target files)
	if (len(md.Narrators) > 0 || applyFieldSelected(overrides, "narrators")) && targetFile != nil && targetFile.FileType == models.FileTypeM4B && (len(md.Narrators) == 0 || h.enrich.personFinder != nil) {
		changed, err := h.applyNarrators(ctx, book.LibraryID, targetFile, md.Narrators, attr, log)
		if err != nil {
			return nil, err
		}
		if changed {
			fileColumns = append(fileColumns, "narrator_source")
		}
	}

	// Publisher (file-level, applied to target file)
	publisherName := strings.TrimSpace(md.Publisher)
	if targetFile != nil && applyFieldSelected(overrides, "publisher") && publisherName == "" {
		if targetFile.PublisherID != nil || targetFile.PublisherSource != nil {
			targetFile.PublisherID = nil
			targetFile.Publisher = nil
			targetFile.PublisherSource = nil
			fileColumns = append(fileColumns, "publisher_id", "publisher_source")
		}
	} else if publisherName != "" && targetFile != nil && h.enrich.publisherFinder != nil {
		// Resolve first, then compare IDs, so an alias or different spelling of
		// the stored publisher is a no-op rather than a change.
		publisher, pErr := h.enrich.publisherFinder.FindOrCreatePublisher(ctx, publisherName, book.LibraryID)
		if pErr != nil {
			log.Warn("failed to find/create publisher", logger.Data{"name": publisherName, "error": pErr.Error()})
		} else if targetFile.PublisherID == nil || *targetFile.PublisherID != publisher.ID {
			publisherSource := attr.sourceFor("publisher")
			targetFile.PublisherID = &publisher.ID
			targetFile.Publisher = publisher
			targetFile.PublisherSource = &publisherSource
			fileColumns = append(fileColumns, "publisher_id", "publisher_source")
			if h.enrich.searchIndexer != nil {
				if err := h.enrich.searchIndexer.IndexPublisher(ctx, publisher); err != nil {
					log.Warn("failed to index publisher", logger.Data{"publisher_id": publisher.ID, "error": err.Error()})
				}
			}
		}
	}

	// URL (file-level, applied to target file)
	url := strings.TrimSpace(md.URL)
	if (url != "" || applyFieldSelected(overrides, "url")) && targetFile != nil {
		if applyOptionalString(url, &targetFile.URL, &targetFile.URLSource, attr.sourceFor("url")) {
			fileColumns = append(fileColumns, "url", "url_source")
		}
	}

	// Name (file-level, applied to target file). Only written when the
	// caller explicitly opted in via overrides.FileName. An empty selected
	// value clears the edition name so naming falls back to book.Title.
	if overrides != nil && overrides.FileName != nil && targetFile != nil {
		if applyOptionalString(*overrides.FileName, &targetFile.Name, &targetFile.NameSource, attr.sourceFor(SourcesKeyFileName)) {
			fileColumns = append(fileColumns, "name", "name_source")
		}
	}

	// Release date (file-level, applied to target file)
	if (md.ReleaseDate != nil || applyFieldSelected(overrides, "release_date")) && targetFile != nil {
		if applyOptional(md.ReleaseDate, &targetFile.ReleaseDate, &targetFile.ReleaseDateSource, attr.sourceFor("release_date"), sameCalendarDate) {
			fileColumns = append(fileColumns, "release_date", "release_date_source")
		}
	}

	// Language (file-level, applied to target file)
	if (md.Language != nil || applyFieldSelected(overrides, "language")) && targetFile != nil {
		if applyOptionalValue(md.Language, &targetFile.Language, &targetFile.LanguageSource, attr.sourceFor("language")) {
			fileColumns = append(fileColumns, "language", "language_source")
		}
	}

	// Abridged (file-level, applied to target file)
	if (md.Abridged != nil || applyFieldSelected(overrides, "abridged")) && targetFile != nil {
		if applyOptionalValue(md.Abridged, &targetFile.Abridged, &targetFile.AbridgedSource, attr.sourceFor("abridged")) {
			fileColumns = append(fileColumns, "abridged", "abridged_source")
		}
	}

	// Identifiers (file-level, applied to target file). A valid selected empty
	// collection clears all identifiers. Non-empty malformed collections are
	// not marked selected by convertFieldsToOverrides and remain a no-op.
	if (len(md.Identifiers) > 0 || applyFieldSelected(overrides, "identifiers")) && targetFile != nil {
		changed, err := h.applyIdentifiers(ctx, targetFile, md.Identifiers, attr, overrides)
		if err != nil {
			return nil, err
		}
		if changed {
			fileColumns = append(fileColumns, "identifier_source")
		}
	}

	// Apply cover data. Precedence is strict: page-based files (CBZ, PDF)
	// only accept coverPage; other formats only accept coverData / coverUrl.
	// Cover has no edit state: keeping the current Cover sends no cover field,
	// and choosing the proposed Cover is always a Proposal Acceptance that
	// stamps the plugin source. Both paths write the complete Cover state
	// (filename, MIME type, source, and page where applicable) in one column
	// set, and a failed download, extraction, or write leaves the previous
	// Cover metadata untouched.
	if targetFile != nil {
		if models.IsPageBasedFileType(targetFile.FileType) {
			// Page-based: apply coverPage, silently ignore coverData/coverUrl.
			if md.CoverPage != nil {
				switch err := h.applyCoverPage(targetFile, book.Filepath, *md.CoverPage, pluginSource, log); {
				case err == nil:
					fileColumns = append(fileColumns, "cover_page", "cover_image_filename", "cover_mime_type", "cover_source")
				case errors.Is(err, errCoverUnchanged):
					// Same page, cover present: nothing to do and nothing to report.
				default:
					log.Warn("plugin-provided coverPage skipped", logger.Data{"file_id": targetFile.ID, "cover_page": *md.CoverPage, "error": err.Error()})
					warnings = append(warnings, coverWarning(err))
				}
			}
		} else if len(md.CoverData) > 0 {
			// Image-based: write the downloaded or plugin-supplied bytes.
			stale, err := applyCoverImage(targetFile, book.Filepath, md, pluginSource)
			if err != nil {
				log.Warn("plugin-provided cover skipped", logger.Data{"file_id": targetFile.ID, "error": err.Error()})
				warnings = append(warnings, coverWarning(err))
			} else {
				fileColumns = append(fileColumns, "cover_image_filename", "cover_mime_type", "cover_source")
				staleCovers = stale
			}
		}
	}

	// Flush all file-level column updates in a single DB call
	bookIndexChanged = bookIndexChanged || len(fileColumns) > 0
	if len(fileColumns) > 0 && targetFile != nil {
		if err := h.enrich.bookStore.UpdateFile(ctx, targetFile, fileColumns); err != nil {
			return nil, errors.Wrap(err, "failed to update file metadata")
		}
		for _, stalePath := range staleCovers {
			if err := os.Remove(stalePath); err != nil && !os.IsNotExist(err) {
				log.Warn("failed to remove stale cover", logger.Data{"file_id": targetFile.ID, "path": stalePath, "error": err.Error()})
			}
		}
	}

	// Write sidecars to keep them in sync
	updatedBook, err := h.enrich.bookStore.RetrieveBook(ctx, book.ID)
	if err == nil {
		if sErr := sidecar.WriteBookSidecarFromModel(updatedBook); sErr != nil {
			log.Warn("failed to write book sidecar", logger.Data{"error": sErr.Error()})
		}
		for _, file := range updatedBook.Files {
			if sErr := sidecar.WriteFileSidecarFromModel(file); sErr != nil {
				log.Warn("failed to write file sidecar", logger.Data{"file_id": file.ID, "error": sErr.Error()})
			}
		}
	}

	// Update FTS index
	if h.enrich.searchIndexer != nil && updatedBook != nil && bookIndexChanged {
		if err := h.enrich.searchIndexer.IndexBook(ctx, updatedBook); err != nil {
			log.Warn("failed to update search index", logger.Data{"error": err.Error()})
		}
	}

	return warnings, nil
}

// errCoverUnchanged reports that the proposed cover is already the stored
// cover, so nothing was written and existing provenance is preserved.
var errCoverUnchanged = errors.New("cover unchanged")

// coverWarning turns a cover skip reason into the user-facing warning that
// rides along with an otherwise successful apply.
func coverWarning(err error) string {
	return "Cover was not applied: " + err.Error() + "."
}

// applyCoverPage extracts the given page as the file's cover and sets the
// complete Cover state on targetFile. It returns nil when the cover columns
// changed, errCoverUnchanged for the identity no-op, and any other error when
// the page was skipped. Cover identity for page-based files is the page
// number: when the proposed page equals the stored cover_page and its cover
// image exists on disk, nothing is extracted and the existing provenance is
// preserved.
func (h *handler) applyCoverPage(targetFile *models.File, bookFilepath string, page int, pluginSource string, log logger.Logger) error {
	switch {
	case page < 0:
		return errors.Errorf("cover page %d is negative", page)
	case targetFile.CoverPage != nil && *targetFile.CoverPage == page && coverImageExists(targetFile):
		return errCoverUnchanged
	case targetFile.PageCount == nil:
		return errors.New("the file's page count is unknown")
	case page >= *targetFile.PageCount:
		return errors.Errorf("cover page %d is out of range for a file with %d pages", page, *targetFile.PageCount)
	case h.enrich.pageExtractor == nil:
		return errors.New("no page extractor is configured")
	}

	coverFilename, mimeType, err := h.enrich.pageExtractor.ExtractCoverPage(targetFile, bookFilepath, page, log)
	if err != nil {
		return errors.Wrapf(err, "cover page %d could not be extracted", page)
	}
	coverFilename = filepath.Base(coverFilename)
	targetFile.CoverPage = &page
	targetFile.CoverImageFilename = &coverFilename
	targetFile.CoverMimeType = &mimeType
	targetFile.CoverSource = &pluginSource
	return nil
}

// coverImageExists reports whether the file's stored cover image is present
// on disk. Read-side resolution is relative to the file, never the book path.
func coverImageExists(file *models.File) bool {
	if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(filepath.Dir(file.Filepath), filepath.Base(*file.CoverImageFilename)))
	return err == nil
}

// applyCoverImage writes md.CoverData next to the file and sets the complete
// Cover state on targetFile. It returns the previous cover files that now
// need removing, or an error when the cover was skipped. Bytes that do not
// decode as a raster image (SVG, AVIF, an error body served as image/*) are
// rejected so they never replace a working cover. The stored extension and
// MIME type describe the normalized bytes on disk, not the download's
// Content-Type. Stale covers are found on disk by base name, matching how
// the scanner discovers covers, and the caller removes them only after the
// column write succeeds.
func applyCoverImage(targetFile *models.File, bookFilepath string, md *mediafile.ParsedMetadata, pluginSource string) ([]string, error) {
	if fileutils.ImageResolution(md.CoverData) == 0 {
		return nil, errors.Errorf("the downloaded file is not a decodable image (%s)", md.CoverMimeType)
	}

	coverDir := fileutils.ResolveCoverDirForWrite(bookFilepath, targetFile.Filepath)
	coverBaseName := filepath.Base(targetFile.Filepath) + ".cover"

	// A decodable image normalizes to exactly one of JPEG or PNG.
	normalizedData, normalizedMime, _ := fileutils.NormalizeImage(md.CoverData, md.CoverMimeType)
	coverExt := ".png"
	if normalizedMime == "image/jpeg" {
		coverExt = ".jpg"
	}

	coverFilename := coverBaseName + coverExt
	coverFilepath := filepath.Join(coverDir, coverFilename)

	if err := os.WriteFile(coverFilepath, normalizedData, 0600); err != nil {
		return nil, errors.Wrap(err, "the cover file could not be written")
	}

	var stale []string
	for _, ext := range fileutils.CoverImageExtensions {
		candidate := coverBaseName + ext
		if candidate == coverFilename {
			continue
		}
		candidatePath := filepath.Join(coverDir, candidate)
		if _, err := os.Stat(candidatePath); err == nil {
			stale = append(stale, candidatePath)
		}
	}

	targetFile.CoverImageFilename = &coverFilename
	targetFile.CoverMimeType = &normalizedMime
	targetFile.CoverSource = &pluginSource
	return stale, nil
}
