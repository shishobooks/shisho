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

// persistMetadata applies metadata to a book and its target file unconditionally (no field filtering).
// Every non-empty field in md is persisted. pluginScope and pluginID identify the data source.
// targetFile is the specific file to apply file-level metadata (identifiers, cover) to; may be nil.
func (h *handler) persistMetadata(ctx context.Context, book *models.Book, targetFile *models.File, md *mediafile.ParsedMetadata, pluginScope, pluginID string, log logger.Logger) error {
	pluginSource := models.PluginDataSource(pluginScope, pluginID)
	var columns []string

	// Accumulate file-level column updates so Title/Narrator/Publisher/etc.
	// can all contribute, then flush once at the end.
	var fileColumns []string

	// Title
	title := strings.TrimSpace(md.Title)
	if title != "" {
		book.Title = title
		book.TitleSource = pluginSource
		book.SortTitle = sortname.ForTitle(title)
		book.SortTitleSource = pluginSource
		columns = append(columns, "title", "title_source", "sort_title", "sort_title_source")

		// Mirror the identified title onto the target main file's Name so
		// file organization and downloads reflect it. Supplements keep their
		// own filename-based label.
		if targetFile != nil && targetFile.FileRole == models.FileRoleMain {
			titleCopy := title
			targetFile.Name = &titleCopy
			targetFile.NameSource = &pluginSource
			fileColumns = append(fileColumns, "name", "name_source")
		}
	}

	// Subtitle
	subtitle := strings.TrimSpace(md.Subtitle)
	if subtitle != "" {
		book.Subtitle = &subtitle
		book.SubtitleSource = &pluginSource
		columns = append(columns, "subtitle", "subtitle_source")
	}

	// Description
	if md.Description != "" {
		desc := htmlutil.StripTags(strings.TrimSpace(md.Description))
		if desc != "" {
			book.Description = &desc
			book.DescriptionSource = &pluginSource
			columns = append(columns, "description", "description_source")
		}
	}

	// Apply scalar column updates
	if len(columns) > 0 {
		if err := h.enrich.bookStore.UpdateBook(ctx, book, columns); err != nil {
			return errors.Wrap(err, "failed to update book")
		}
	}

	// Authors
	if len(md.Authors) > 0 && h.enrich.personFinder != nil {
		if err := h.enrich.relStore.DeleteAuthors(ctx, book.ID); err != nil {
			return errors.Wrap(err, "failed to delete authors")
		}
		for i, pa := range md.Authors {
			if pa.Name == "" {
				continue
			}
			person, pErr := h.enrich.personFinder.FindOrCreatePerson(ctx, pa.Name, book.LibraryID)
			if pErr != nil {
				log.Warn("failed to find/create person", logger.Data{"name": pa.Name, "error": pErr.Error()})
				continue
			}
			var role *string
			if pa.Role != "" {
				role = &pa.Role
			}
			if err := h.enrich.relStore.CreateAuthor(ctx, &models.Author{
				BookID:    book.ID,
				PersonID:  person.ID,
				Role:      role,
				SortOrder: i + 1,
			}); err != nil {
				log.Warn("failed to create author", logger.Data{"error": err.Error()})
			}
		}
		book.AuthorSource = pluginSource
		if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"author_source"}); err != nil {
			return errors.Wrap(err, "failed to update author source")
		}
	}

	// Series
	if md.Series != "" {
		if err := h.enrich.relStore.DeleteBookSeries(ctx, book.ID); err != nil {
			return errors.Wrap(err, "failed to delete series")
		}
		seriesRecord, sErr := h.enrich.relStore.FindOrCreateSeries(ctx, md.Series, book.LibraryID, pluginSource)
		if sErr != nil {
			log.Warn("failed to find/create series", logger.Data{"name": md.Series, "error": sErr.Error()})
		} else {
			if err := h.enrich.relStore.CreateBookSeries(ctx, &models.BookSeries{
				BookID:       book.ID,
				SeriesID:     seriesRecord.ID,
				SeriesNumber: md.SeriesNumber,
				SortOrder:    1,
			}); err != nil {
				log.Warn("failed to create book series", logger.Data{"error": err.Error()})
			}
		}
	}

	// Genres
	if len(md.Genres) > 0 && h.enrich.genreFinder != nil {
		if err := h.enrich.relStore.DeleteBookGenres(ctx, book.ID); err != nil {
			return errors.Wrap(err, "failed to delete genres")
		}
		for _, genreName := range md.Genres {
			if genreName == "" {
				continue
			}
			genre, gErr := h.enrich.genreFinder.FindOrCreateGenre(ctx, genreName, book.LibraryID)
			if gErr != nil {
				log.Warn("failed to find/create genre", logger.Data{"genre": genreName, "error": gErr.Error()})
				continue
			}
			if err := h.enrich.relStore.CreateBookGenre(ctx, &models.BookGenre{
				BookID:  book.ID,
				GenreID: genre.ID,
			}); err != nil {
				log.Warn("failed to create book genre", logger.Data{"error": err.Error()})
			}
		}
		book.GenreSource = &pluginSource
		if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"genre_source"}); err != nil {
			return errors.Wrap(err, "failed to update genre source")
		}
	}

	// Tags
	if len(md.Tags) > 0 && h.enrich.tagFinder != nil {
		if err := h.enrich.relStore.DeleteBookTags(ctx, book.ID); err != nil {
			return errors.Wrap(err, "failed to delete tags")
		}
		for _, tagName := range md.Tags {
			if tagName == "" {
				continue
			}
			tag, tErr := h.enrich.tagFinder.FindOrCreateTag(ctx, tagName, book.LibraryID)
			if tErr != nil {
				log.Warn("failed to find/create tag", logger.Data{"tag": tagName, "error": tErr.Error()})
				continue
			}
			if err := h.enrich.relStore.CreateBookTag(ctx, &models.BookTag{
				BookID: book.ID,
				TagID:  tag.ID,
			}); err != nil {
				log.Warn("failed to create book tag", logger.Data{"error": err.Error()})
			}
		}
		book.TagSource = &pluginSource
		if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"tag_source"}); err != nil {
			return errors.Wrap(err, "failed to update tag source")
		}
	}

	// Narrators (file-level, applied to target file)
	if len(md.Narrators) > 0 && targetFile != nil && h.enrich.personFinder != nil {
		if _, err := h.enrich.bookStore.DeleteNarratorsForFile(ctx, targetFile.ID); err != nil {
			return errors.Wrap(err, "failed to delete narrators")
		}
		for i, narratorName := range md.Narrators {
			if narratorName == "" {
				continue
			}
			person, pErr := h.enrich.personFinder.FindOrCreatePerson(ctx, narratorName, book.LibraryID)
			if pErr != nil {
				log.Warn("failed to find/create person for narrator", logger.Data{"name": narratorName, "error": pErr.Error()})
				continue
			}
			if err := h.enrich.bookStore.CreateNarrator(ctx, &models.Narrator{
				FileID:    targetFile.ID,
				PersonID:  person.ID,
				SortOrder: i + 1,
			}); err != nil {
				log.Warn("failed to create narrator", logger.Data{"error": err.Error()})
			}
		}
		targetFile.NarratorSource = &pluginSource
		fileColumns = append(fileColumns, "narrator_source")
	}

	// Publisher (file-level, applied to target file)
	publisherName := strings.TrimSpace(md.Publisher)
	if publisherName != "" && targetFile != nil && h.enrich.publisherFinder != nil {
		publisher, pErr := h.enrich.publisherFinder.FindOrCreatePublisher(ctx, publisherName, book.LibraryID)
		if pErr != nil {
			log.Warn("failed to find/create publisher", logger.Data{"name": publisherName, "error": pErr.Error()})
		} else {
			targetFile.PublisherID = &publisher.ID
			targetFile.PublisherSource = &pluginSource
			fileColumns = append(fileColumns, "publisher_id", "publisher_source")
		}
	}

	// Imprint (file-level, applied to target file)
	imprintName := strings.TrimSpace(md.Imprint)
	if imprintName != "" && targetFile != nil && h.enrich.imprintFinder != nil {
		imprint, iErr := h.enrich.imprintFinder.FindOrCreateImprint(ctx, imprintName, book.LibraryID)
		if iErr != nil {
			log.Warn("failed to find/create imprint", logger.Data{"name": imprintName, "error": iErr.Error()})
		} else {
			targetFile.ImprintID = &imprint.ID
			targetFile.ImprintSource = &pluginSource
			fileColumns = append(fileColumns, "imprint_id", "imprint_source")
		}
	}

	// URL (file-level, applied to target file)
	url := strings.TrimSpace(md.URL)
	if url != "" && targetFile != nil {
		targetFile.URL = &url
		targetFile.URLSource = &pluginSource
		fileColumns = append(fileColumns, "url", "url_source")
	}

	// Release date (file-level, applied to target file)
	if md.ReleaseDate != nil && targetFile != nil {
		targetFile.ReleaseDate = md.ReleaseDate
		targetFile.ReleaseDateSource = &pluginSource
		fileColumns = append(fileColumns, "release_date", "release_date_source")
	}

	// Language (file-level, applied to target file)
	if md.Language != nil && *md.Language != "" && targetFile != nil {
		targetFile.Language = md.Language
		targetFile.LanguageSource = &pluginSource
		fileColumns = append(fileColumns, "language", "language_source")
	}

	// Abridged (file-level, applied to target file)
	if md.Abridged != nil && targetFile != nil {
		targetFile.Abridged = md.Abridged
		targetFile.AbridgedSource = &pluginSource
		fileColumns = append(fileColumns, "abridged", "abridged_source")
	}

	// Identifiers (file-level, applied to target file). Filter out blanks the
	// plugin may have emitted, then bulk-insert. The bulk helper dedupes by
	// type with last-wins and warns, so a misbehaving plugin never trips the
	// UNIQUE(file_id, type) constraint. The delete is gated on having at
	// least one valid identifier to insert, so a payload of only-blanks
	// preserves the existing identifiers instead of silently wiping them.
	if len(md.Identifiers) > 0 && targetFile != nil {
		toInsert := make([]*models.FileIdentifier, 0, len(md.Identifiers))
		for _, ident := range md.Identifiers {
			if ident.Type == "" || ident.Value == "" {
				continue
			}
			toInsert = append(toInsert, &models.FileIdentifier{
				FileID: targetFile.ID,
				Type:   ident.Type,
				Value:  ident.Value,
				Source: pluginSource,
			})
		}
		if len(toInsert) > 0 {
			if _, err := h.enrich.identStore.DeleteIdentifiersForFile(ctx, targetFile.ID); err != nil {
				return errors.Wrap(err, "failed to delete identifiers")
			}
			if err := h.enrich.identStore.BulkCreateFileIdentifiers(ctx, toInsert); err != nil {
				return errors.Wrap(err, "failed to bulk-create identifiers")
			}
		}
	}

	// Apply cover data. Precedence is strict: page-based files (CBZ, PDF)
	// only accept coverPage; other formats only accept coverData / coverUrl.
	if targetFile != nil {
		if models.IsPageBasedFileType(targetFile.FileType) {
			// Page-based: apply coverPage, silently ignore coverData/coverUrl.
			if md.CoverPage != nil {
				page := *md.CoverPage
				switch {
				case page < 0:
					log.Warn("plugin-provided coverPage is negative, skipping", logger.Data{"file_id": targetFile.ID, "cover_page": page})
				case targetFile.PageCount == nil:
					log.Warn("plugin-provided coverPage skipped: page count unknown", logger.Data{"file_id": targetFile.ID, "cover_page": page})
				case page >= *targetFile.PageCount:
					log.Warn("plugin-provided coverPage is out of range, skipping", logger.Data{"file_id": targetFile.ID, "cover_page": page, "page_count": *targetFile.PageCount})
				case h.enrich.pageExtractor == nil:
					log.Warn("plugin-provided coverPage skipped: no page extractor configured", logger.Data{"file_id": targetFile.ID})
				default:
					coverFilename, mimeType, extractErr := h.enrich.pageExtractor.ExtractCoverPage(targetFile, book.Filepath, page, log)
					if extractErr != nil {
						log.Warn("failed to extract plugin-provided cover page", logger.Data{"file_id": targetFile.ID, "cover_page": page, "error": extractErr.Error()})
					} else {
						targetFile.CoverPage = &page
						targetFile.CoverImageFilename = &coverFilename
						targetFile.CoverMimeType = &mimeType
						source := models.PluginDataSource(pluginScope, pluginID)
						targetFile.CoverSource = &source
						fileColumns = append(fileColumns, "cover_page", "cover_image_filename", "cover_mime_type", "cover_source")
					}
				}
			}
		} else {
			// Non-page-based: existing coverData write path.
			if len(md.CoverData) > 0 {
				coverDir := fileutils.ResolveCoverDirForWrite(book.Filepath, targetFile.Filepath)
				coverBaseName := filepath.Base(targetFile.Filepath) + ".cover"

				normalizedData, normalizedMime, _ := fileutils.NormalizeImage(md.CoverData, md.CoverMimeType)
				coverExt := ".png"
				if normalizedMime == md.CoverMimeType {
					coverExt = md.CoverExtension()
				}

				coverFilename := coverBaseName + coverExt
				coverFilepath := filepath.Join(coverDir, coverFilename)

				if err := os.WriteFile(coverFilepath, normalizedData, 0600); err != nil {
					log.Warn("failed to write cover file", logger.Data{"error": err.Error()})
				} else {
					targetFile.CoverImageFilename = &coverFilename
					fileColumns = append(fileColumns, "cover_image_filename")
				}
			}
		}
	}

	// Flush all file-level column updates in a single DB call
	if len(fileColumns) > 0 && targetFile != nil {
		if err := h.enrich.bookStore.UpdateFile(ctx, targetFile, fileColumns); err != nil {
			return errors.Wrap(err, "failed to update file metadata")
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
	if h.enrich.searchIndexer != nil && updatedBook != nil {
		if err := h.enrich.searchIndexer.IndexBook(ctx, updatedBook); err != nil {
			log.Warn("failed to update search index", logger.Data{"error": err.Error()})
		}
	}

	return nil
}
