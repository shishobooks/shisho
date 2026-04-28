package plugins

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePersistTestJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

// stubBookStoreForPersist is a minimal bookStore implementation used by
// persistMetadata cover-write tests. Only UpdateFile and RetrieveBook
// need to produce meaningful results for the cover-only persist path.
type stubBookStoreForPersist struct {
	book *models.Book
}

func (s *stubBookStoreForPersist) UpdateBook(_ context.Context, _ *models.Book, _ []string) error {
	return nil
}

func (s *stubBookStoreForPersist) RetrieveBook(_ context.Context, _ int) (*models.Book, error) {
	return s.book, nil
}

func (s *stubBookStoreForPersist) UpdateFile(_ context.Context, _ *models.File, _ []string) error {
	return nil
}

func (s *stubBookStoreForPersist) DeleteNarratorsForFile(_ context.Context, _ int) (int, error) {
	return 0, nil
}

func (s *stubBookStoreForPersist) CreateNarrator(_ context.Context, _ *models.Narrator) error {
	return nil
}

func (s *stubBookStoreForPersist) OrganizeBookFiles(_ context.Context, _ *models.Book) error {
	return nil
}

// TestPersistMetadata_CoverWrite_RootLevelFile_SyntheticBookPath is a
// regression test for a bug where persistMetadata wrote plugin-provided
// cover data unconditionally to book.Filepath as the cover directory. For
// root-level new files, scanFileCreateNew computes a synthetic organized-
// folder bookPath that does not exist on disk, so os.WriteFile failed with
// "no such file or directory" and the cover was silently dropped. The fix
// uses fileutils.ResolveCoverDirForWrite, which falls back to the file's
// parent directory when book.Filepath does not resolve to a real directory.
func TestPersistMetadata_CoverWrite_RootLevelFile_SyntheticBookPath(t *testing.T) {
	t.Parallel()

	// A real file lives at the library root.
	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0600))

	// Synthetic book.Filepath that does NOT exist on disk, mirroring what
	// scanFileCreateNew computes for root-level new files in libraries with
	// OrganizeFileStructure enabled.
	syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
	_, err := os.Stat(syntheticBookPath)
	require.True(t, os.IsNotExist(err), "synthetic bookPath must not exist on disk")

	file := &models.File{
		ID:       1,
		BookID:   1,
		Filepath: filePath,
		FileType: models.FileTypeEPUB,
	}
	book := &models.Book{
		ID:        1,
		LibraryID: 1,
		Filepath:  syntheticBookPath,
		Files:     []*models.File{file},
	}

	h := &handler{
		enrich: &enrichDeps{
			bookStore: &stubBookStoreForPersist{book: book},
		},
	}

	md := &mediafile.ParsedMetadata{
		CoverData:     makePersistTestJPEG(400, 600),
		CoverMimeType: "image/jpeg",
	}

	// persistMetadata will also attempt to write book/file sidecars under
	// the synthetic bookPath and log warnings when those writes fail. That
	// is expected: the test deliberately uses a nonexistent bookPath, and
	// sidecar failures are non-fatal. The assertions below only cover the
	// cover-write path.
	err = h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	// Cover must land next to the file in the library dir, not under the
	// nonexistent synthetic organized-folder path.
	expectedCoverPath := filepath.Join(libraryDir, "book.epub.cover.jpg")
	_, err = os.Stat(expectedCoverPath)
	require.NoError(t, err, "cover should be written next to the file at library dir")

	// Nothing should have been written under the synthetic path.
	_, err = os.Stat(syntheticBookPath)
	assert.True(t, os.IsNotExist(err), "synthetic bookPath must still not exist after persist")

	// CoverImageFilename should be set (to the filename only, not a full path).
	require.NotNil(t, file.CoverImageFilename, "CoverImageFilename should be set on the file")
	assert.Equal(t, "book.epub.cover.jpg", *file.CoverImageFilename)
}

// stubPageExtractor records calls and returns a fixed (filename, mimeType).
// Set `wantErr` to simulate a failed extraction.
type stubPageExtractor struct {
	calls    []stubPageExtractorCall
	filename string
	mimeType string
	wantErr  error
}

type stubPageExtractorCall struct {
	FileID       int
	BookFilepath string
	Page         int
}

func (s *stubPageExtractor) ExtractCoverPage(file *models.File, bookFilepath string, page int, _ logger.Logger) (string, string, error) {
	s.calls = append(s.calls, stubPageExtractorCall{FileID: file.ID, BookFilepath: bookFilepath, Page: page})
	if s.wantErr != nil {
		return "", "", s.wantErr
	}
	return s.filename, s.mimeType, nil
}

func TestPersistMetadata_CoverPage_CBZ_HappyPath(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "comic.cbz")
	require.NoError(t, os.WriteFile(filePath, []byte("fake cbz"), 0600))

	pageCount := 10
	file := &models.File{
		ID:        1,
		BookID:    1,
		Filepath:  filePath,
		FileType:  models.FileTypeCBZ,
		PageCount: &pageCount,
	}
	book := &models.Book{
		ID:        1,
		LibraryID: 1,
		Filepath:  libraryDir,
		Files:     []*models.File{file},
	}

	extractor := &stubPageExtractor{filename: "comic.cbz.cover.jpg", mimeType: "image/jpeg"}

	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			pageExtractor: extractor,
		},
	}

	page := 3
	md := &mediafile.ParsedMetadata{CoverPage: &page}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	require.Len(t, extractor.calls, 1)
	assert.Equal(t, 1, extractor.calls[0].FileID)
	assert.Equal(t, 3, extractor.calls[0].Page)

	require.NotNil(t, file.CoverPage)
	assert.Equal(t, 3, *file.CoverPage)
	require.NotNil(t, file.CoverImageFilename)
	assert.Equal(t, "comic.cbz.cover.jpg", *file.CoverImageFilename)
	require.NotNil(t, file.CoverMimeType)
	assert.Equal(t, "image/jpeg", *file.CoverMimeType)
	require.NotNil(t, file.CoverSource)
	assert.Equal(t, models.PluginDataSource("test", "plugin-id"), *file.CoverSource)
}

func TestPersistMetadata_CoverPage_PDF_HappyPath(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.pdf")
	require.NoError(t, os.WriteFile(filePath, []byte("fake pdf"), 0600))

	pageCount := 100
	file := &models.File{
		ID: 2, BookID: 2, Filepath: filePath, FileType: models.FileTypePDF, PageCount: &pageCount,
	}
	book := &models.Book{ID: 2, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}

	extractor := &stubPageExtractor{filename: "book.pdf.cover.jpg", mimeType: "image/jpeg"}
	h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

	page := 7
	md := &mediafile.ParsedMetadata{CoverPage: &page}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	require.Len(t, extractor.calls, 1)
	assert.Equal(t, 7, extractor.calls[0].Page)
	require.NotNil(t, file.CoverPage)
	assert.Equal(t, 7, *file.CoverPage)
}

func TestPersistMetadata_CoverPage_OutOfBounds(t *testing.T) {
	t.Parallel()

	intPointer := func(v int) *int { return &v }

	cases := []struct {
		name      string
		pageCount *int
		page      int
	}{
		{"negative", intPointer(10), -1},
		{"page equals count", intPointer(5), 5},
		{"page above count", intPointer(5), 99},
		{"page count unknown", nil, 3},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			libraryDir := t.TempDir()
			filePath := filepath.Join(libraryDir, "comic.cbz")
			require.NoError(t, os.WriteFile(filePath, []byte("fake cbz"), 0600))

			file := &models.File{
				ID: 1, BookID: 1, Filepath: filePath, FileType: models.FileTypeCBZ, PageCount: tc.pageCount,
			}
			book := &models.Book{ID: 1, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}
			extractor := &stubPageExtractor{filename: "x", mimeType: "image/jpeg"}
			h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

			md := &mediafile.ParsedMetadata{CoverPage: &tc.page}

			err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
			require.NoError(t, err)

			assert.Empty(t, extractor.calls, "extractor should not be called for invalid page")
			assert.Nil(t, file.CoverPage, "file.CoverPage should remain unchanged")
			assert.Nil(t, file.CoverImageFilename, "file.CoverImageFilename should remain unchanged")
		})
	}
}

func TestPersistMetadata_CoverPage_ExtractorError(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "comic.cbz")
	require.NoError(t, os.WriteFile(filePath, []byte("fake cbz"), 0600))

	pageCount := 10
	file := &models.File{
		ID: 1, BookID: 1, Filepath: filePath, FileType: models.FileTypeCBZ, PageCount: &pageCount,
	}
	book := &models.Book{ID: 1, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}

	extractor := &stubPageExtractor{wantErr: errors.New("extraction failed")}
	h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

	page := 3
	md := &mediafile.ParsedMetadata{CoverPage: &page}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err, "extractor errors should be logged, not returned")

	require.Len(t, extractor.calls, 1, "extractor should still be called once")
	assert.Nil(t, file.CoverPage, "file.CoverPage should remain unchanged")
	assert.Nil(t, file.CoverImageFilename, "file.CoverImageFilename should remain unchanged")
	assert.Nil(t, file.CoverSource, "file.CoverSource should remain unchanged")
}

// Plugin returns both coverPage and coverData for a CBZ file — coverPage wins,
// coverData is silently ignored (no .cover file written via the data path).
func TestPersistMetadata_CoverPage_CBZ_BeatsCoverData(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "comic.cbz")
	require.NoError(t, os.WriteFile(filePath, []byte("fake cbz"), 0600))

	pageCount := 10
	file := &models.File{
		ID: 1, BookID: 1, Filepath: filePath, FileType: models.FileTypeCBZ, PageCount: &pageCount,
	}
	book := &models.Book{ID: 1, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}
	extractor := &stubPageExtractor{filename: "comic.cbz.cover.jpg", mimeType: "image/jpeg"}
	h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

	page := 2
	md := &mediafile.ParsedMetadata{
		CoverPage:     &page,
		CoverData:     makePersistTestJPEG(400, 600),
		CoverMimeType: "image/jpeg",
	}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	// coverPage path taken
	require.Len(t, extractor.calls, 1)
	require.NotNil(t, file.CoverPage)
	assert.Equal(t, 2, *file.CoverPage)

	// coverData file must NOT have been written alongside the file
	_, err = os.Stat(filepath.Join(libraryDir, "comic.cbz.cover.png"))
	assert.True(t, os.IsNotExist(err), "coverData should not have been written for a page-based file")
	_, err = os.Stat(filepath.Join(libraryDir, "comic.cbz.cover.jpg"))
	assert.True(t, os.IsNotExist(err), "coverData should not have been written for a page-based file (jpg path)")
}

// stubIdentStoreForPersist is a minimal identifierStore implementation that
// records writes for assertion. It does NOT enforce the unique-by-type
// constraint — that's the job of BulkCreateFileIdentifiers in the real impl,
// and the unit-level tests for that helper live in pkg/books.
type stubIdentStoreForPersist struct {
	deleteCalls []int
	bulkCalls   [][]*models.FileIdentifier
}

func (s *stubIdentStoreForPersist) DeleteIdentifiersForFile(_ context.Context, fileID int) (int, error) {
	s.deleteCalls = append(s.deleteCalls, fileID)
	return 0, nil
}

func (s *stubIdentStoreForPersist) BulkCreateFileIdentifiers(_ context.Context, ids []*models.FileIdentifier) error {
	s.bulkCalls = append(s.bulkCalls, ids)
	return nil
}

// TestPersistMetadata_BulkInsertsIdentifiers verifies that a plugin payload
// of identifiers is forwarded as a single BulkCreateFileIdentifiers call
// (instead of the legacy per-item CreateFileIdentifier loop). Blank
// type/value entries are filtered out before the bulk call.
func TestPersistMetadata_BulkInsertsIdentifiers(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0600))

	file := &models.File{
		ID:       1,
		BookID:   1,
		Filepath: filePath,
		FileType: models.FileTypeEPUB,
	}
	book := &models.Book{
		ID:        1,
		LibraryID: 1,
		Filepath:  libraryDir,
		Files:     []*models.File{file},
	}

	identStore := &stubIdentStoreForPersist{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:  &stubBookStoreForPersist{book: book},
			identStore: identStore,
		},
	}

	md := &mediafile.ParsedMetadata{
		Identifiers: []mediafile.ParsedIdentifier{
			{Type: "asin", Value: "B01ABC1234"},
			{Type: "", Value: "ignored"}, // blank type filtered
			{Type: "isbn_13", Value: ""}, // blank value filtered
			{Type: "isbn_13", Value: "9780316769488"},
		},
	}

	err := h.persistMetadata(context.Background(), book, file, md, "shisho", "audnexus", testLogger())
	require.NoError(t, err)

	require.Equal(t, []int{file.ID}, identStore.deleteCalls)
	require.Len(t, identStore.bulkCalls, 1, "exactly one BulkCreateFileIdentifiers call")

	got := identStore.bulkCalls[0]
	require.Len(t, got, 2, "blanks filtered out, two valid identifiers remain")
	expectedSource := models.PluginDataSource("shisho", "audnexus")
	assert.Equal(t, "asin", got[0].Type)
	assert.Equal(t, "B01ABC1234", got[0].Value)
	assert.Equal(t, expectedSource, got[0].Source)
	assert.Equal(t, "isbn_13", got[1].Type)
	assert.Equal(t, "9780316769488", got[1].Value)
	assert.Equal(t, expectedSource, got[1].Source)
}

// TestPersistMetadata_AllBlanksPreservesExistingIdentifiers documents that a
// plugin payload containing only blank type/value entries does NOT delete the
// file's existing identifiers. The delete must be gated on having at least
// one valid identifier to insert; otherwise blanks would silently wipe data.
func TestPersistMetadata_AllBlanksPreservesExistingIdentifiers(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0600))

	file := &models.File{
		ID:       1,
		BookID:   1,
		Filepath: filePath,
		FileType: models.FileTypeEPUB,
	}
	book := &models.Book{
		ID:        1,
		LibraryID: 1,
		Filepath:  libraryDir,
		Files:     []*models.File{file},
	}

	identStore := &stubIdentStoreForPersist{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:  &stubBookStoreForPersist{book: book},
			identStore: identStore,
		},
	}

	md := &mediafile.ParsedMetadata{
		Identifiers: []mediafile.ParsedIdentifier{
			{Type: "", Value: "ignored"},
			{Type: "isbn_13", Value: ""},
		},
	}

	err := h.persistMetadata(context.Background(), book, file, md, "shisho", "audnexus", testLogger())
	require.NoError(t, err)

	assert.Empty(t, identStore.deleteCalls, "delete must NOT fire when no valid identifiers to insert")
	assert.Empty(t, identStore.bulkCalls, "bulk insert must NOT fire when no valid identifiers")
}

// Plugin returns coverPage for a non-page-based format (EPUB) — coverPage is
// silently ignored, coverData (if provided) is applied.
func TestPersistMetadata_CoverPage_EPUB_Ignored(t *testing.T) {
	t.Parallel()

	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0600))

	file := &models.File{
		ID: 1, BookID: 1, Filepath: filePath, FileType: models.FileTypeEPUB,
	}
	book := &models.Book{ID: 1, LibraryID: 1, Filepath: libraryDir, Files: []*models.File{file}}
	extractor := &stubPageExtractor{filename: "x", mimeType: "image/jpeg"}
	h := &handler{enrich: &enrichDeps{bookStore: &stubBookStoreForPersist{book: book}, pageExtractor: extractor}}

	page := 3
	md := &mediafile.ParsedMetadata{
		CoverPage:     &page,
		CoverData:     makePersistTestJPEG(400, 600),
		CoverMimeType: "image/jpeg",
	}

	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	// Extractor must not be called for non-page-based files
	assert.Empty(t, extractor.calls)
	// file.CoverPage must remain unchanged
	assert.Nil(t, file.CoverPage)
	// coverData write path ran — CoverImageFilename is set
	require.NotNil(t, file.CoverImageFilename)
	assert.Equal(t, "book.epub.cover.jpg", *file.CoverImageFilename)
}

// stubSearchIndexer records calls to each Index* method so tests can assert
// that persistMetadata keeps the FTS index in sync with newly-created or
// re-associated entities. Without these calls, an entity created during the
// identify-apply flow exists in its table but has no FTS row, so it is
// invisible to search-driven dropdowns (e.g. the series combobox in the
// IdentifyReviewForm).
type stubSearchIndexer struct {
	indexedBookIDs   []int
	indexedSeriesIDs []int
	indexedPersonIDs []int
	indexedGenreIDs  []int
	indexedTagIDs    []int
}

func (s *stubSearchIndexer) IndexBook(_ context.Context, b *models.Book) error {
	s.indexedBookIDs = append(s.indexedBookIDs, b.ID)
	return nil
}

func (s *stubSearchIndexer) IndexSeries(_ context.Context, sr *models.Series) error {
	s.indexedSeriesIDs = append(s.indexedSeriesIDs, sr.ID)
	return nil
}

func (s *stubSearchIndexer) IndexPerson(_ context.Context, p *models.Person) error {
	s.indexedPersonIDs = append(s.indexedPersonIDs, p.ID)
	return nil
}

func (s *stubSearchIndexer) IndexGenre(_ context.Context, g *models.Genre) error {
	s.indexedGenreIDs = append(s.indexedGenreIDs, g.ID)
	return nil
}

func (s *stubSearchIndexer) IndexTag(_ context.Context, tag *models.Tag) error {
	s.indexedTagIDs = append(s.indexedTagIDs, tag.ID)
	return nil
}

// stubPersonFinderForPersist returns a person with a sequential ID for each
// FindOrCreatePerson call so tests can assert on the indexed person IDs.
type stubPersonFinderForPersist struct {
	nextID int
}

func (s *stubPersonFinderForPersist) FindOrCreatePerson(_ context.Context, name string, libraryID int) (*models.Person, error) {
	s.nextID++
	return &models.Person{ID: s.nextID, LibraryID: libraryID, Name: name}, nil
}

// stubGenreFinderForPersist returns a genre with a sequential ID for each call.
type stubGenreFinderForPersist struct {
	nextID int
}

func (s *stubGenreFinderForPersist) FindOrCreateGenre(_ context.Context, name string, libraryID int) (*models.Genre, error) {
	s.nextID++
	return &models.Genre{ID: s.nextID, LibraryID: libraryID, Name: name}, nil
}

// stubTagFinderForPersist returns a tag with a sequential ID for each call.
type stubTagFinderForPersist struct {
	nextID int
}

func (s *stubTagFinderForPersist) FindOrCreateTag(_ context.Context, name string, libraryID int) (*models.Tag, error) {
	s.nextID++
	return &models.Tag{ID: s.nextID, LibraryID: libraryID, Name: name}, nil
}

// TestPersistMetadata_IndexesNewSeries is a regression test for the bug where
// a series created via the identify-apply flow ("Create 'Series'" option in
// the IdentifyReviewForm combobox) never received a series_fts row, so it
// was invisible to subsequent series search dropdowns even though the row
// existed in the series table. The fix calls IndexSeries after CreateBookSeries
// to keep the FTS index in sync.
func TestPersistMetadata_IndexesNewSeries(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	indexer := &stubSearchIndexer{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			relStore:      &stubRelStoreForApply{},
			searchIndexer: indexer,
		},
	}

	md := &mediafile.ParsedMetadata{Series: "My New Series"}
	err := h.persistMetadata(context.Background(), book, nil, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	assert.Contains(t, indexer.indexedSeriesIDs, 1, "series created via identify-apply must be added to series_fts so it shows up in series search dropdowns")
}

// TestPersistMetadata_IndexesNewAuthorsNarratorsGenresTags is a regression
// test for the same FTS-sync gap on the other entities created during the
// identify-apply flow. Persons, genres, and tags all use FTS for their list
// search endpoints (persons_fts, genres_fts, tags_fts), so newly-created rows
// must also be indexed to be findable in their respective dropdowns.
func TestPersistMetadata_IndexesNewAuthorsNarratorsGenresTags(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeEPUB)
	indexer := &stubSearchIndexer{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			relStore:      &stubRelStoreForApply{},
			personFinder:  &stubPersonFinderForPersist{},
			genreFinder:   &stubGenreFinderForPersist{},
			tagFinder:     &stubTagFinderForPersist{},
			searchIndexer: indexer,
		},
	}

	md := &mediafile.ParsedMetadata{
		Authors:   []mediafile.ParsedAuthor{{Name: "New Author", Role: "writer"}},
		Narrators: []string{"New Narrator"},
		Genres:    []string{"New Genre"},
		Tags:      []string{"New Tag"},
	}
	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	assert.Len(t, indexer.indexedPersonIDs, 2, "both new author and new narrator must be added to persons_fts")
	assert.Len(t, indexer.indexedGenreIDs, 1, "new genre must be added to genres_fts")
	assert.Len(t, indexer.indexedTagIDs, 1, "new tag must be added to tags_fts")
}

// TestPersistMetadata_ReindexesDetachedOldSeries verifies that when an apply
// replaces a book's series, the previously-attached series gets re-indexed
// so its series_fts.book_titles / book_authors aggregate columns stop
// listing this book. Without this, searching by the moved book's title or
// author would still surface the old series.
func TestPersistMetadata_ReindexesDetachedOldSeries(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	// Pre-attach the book to an existing series (ID 99). When apply runs
	// with a different series, the stub FindOrCreateSeries returns ID 1,
	// so series 99 becomes detached and must be re-indexed for aggregate
	// freshness.
	oldSeries := &models.Series{ID: 99, LibraryID: book.LibraryID, Name: "Old Series"}
	book.BookSeries = []*models.BookSeries{{
		BookID:   book.ID,
		SeriesID: oldSeries.ID,
		Series:   oldSeries,
	}}

	indexer := &stubSearchIndexer{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			relStore:      &stubRelStoreForApply{},
			searchIndexer: indexer,
		},
	}

	md := &mediafile.ParsedMetadata{Series: "New Series"}
	err := h.persistMetadata(context.Background(), book, nil, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	assert.Contains(t, indexer.indexedSeriesIDs, 1, "newly-attached series (id 1 from stub) must be indexed")
	assert.Contains(t, indexer.indexedSeriesIDs, 99, "detached old series (id 99) must be re-indexed so its book_titles aggregate stops listing this book")
}

// TestPersistMetadata_SkipsSeriesIndexWhenAttachmentUnchanged verifies that
// re-applying the same series to a book does not trigger an unnecessary
// IndexSeries call. The new series ID matches the existing attachment, so
// neither the membership nor the aggregate columns change — re-indexing
// would be pure DELETE+INSERT churn against series_fts.
func TestPersistMetadata_SkipsSeriesIndexWhenAttachmentUnchanged(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	// Pre-attach the book to series ID 1 — same ID the stub returns for
	// every FindOrCreateSeries call.
	existingSeries := &models.Series{ID: 1, LibraryID: book.LibraryID, Name: "Same Series"}
	book.BookSeries = []*models.BookSeries{{
		BookID:   book.ID,
		SeriesID: existingSeries.ID,
		Series:   existingSeries,
	}}

	indexer := &stubSearchIndexer{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			relStore:      &stubRelStoreForApply{},
			searchIndexer: indexer,
		},
	}

	md := &mediafile.ParsedMetadata{Series: "Same Series"}
	err := h.persistMetadata(context.Background(), book, nil, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	assert.NotContains(t, indexer.indexedSeriesIDs, 1, "series whose attachment to this book did not change must NOT be re-indexed (avoids series_fts churn)")
}

// TestPersistMetadata_ReindexesSameSeriesWhenTitleChanges verifies that a
// series whose attachment to this book did not change is still re-indexed
// when book.title changes during the apply — series_fts has aggregate
// columns (book_titles, book_authors) that would otherwise go stale.
// Without this branch, searching for a book by its new title within a
// series search would still surface the old title.
func TestPersistMetadata_ReindexesSameSeriesWhenTitleChanges(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Old Title")
	existingSeries := &models.Series{ID: 1, LibraryID: book.LibraryID, Name: "Same Series"}
	book.BookSeries = []*models.BookSeries{{
		BookID:   book.ID,
		SeriesID: existingSeries.ID,
		Series:   existingSeries,
	}}

	indexer := &stubSearchIndexer{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			relStore:      &stubRelStoreForApply{},
			searchIndexer: indexer,
		},
	}

	md := &mediafile.ParsedMetadata{
		Title:  "New Title",
		Series: "Same Series",
	}
	err := h.persistMetadata(context.Background(), book, nil, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	assert.Contains(t, indexer.indexedSeriesIDs, 1, "series whose membership did not change must still be re-indexed when book.title changed, otherwise series_fts.book_titles goes stale")
}

// TestPersistMetadata_SkipsSeriesIndexWhenSameTitleReapplied verifies that
// re-applying the same title (alongside an unchanged series) does not
// trigger an IndexSeries call. The previous over-trigger keyed off "title
// block ran" rather than "title actually changed", causing one extra
// series_fts DELETE+INSERT per identical-title apply.
func TestPersistMetadata_SkipsSeriesIndexWhenSameTitleReapplied(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Same Title")
	existingSeries := &models.Series{ID: 1, LibraryID: book.LibraryID, Name: "Same Series"}
	book.BookSeries = []*models.BookSeries{{
		BookID:   book.ID,
		SeriesID: existingSeries.ID,
		Series:   existingSeries,
	}}

	indexer := &stubSearchIndexer{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			relStore:      &stubRelStoreForApply{},
			searchIndexer: indexer,
		},
	}

	md := &mediafile.ParsedMetadata{
		Title:  "Same Title",
		Series: "Same Series",
	}
	err := h.persistMetadata(context.Background(), book, nil, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	assert.NotContains(t, indexer.indexedSeriesIDs, 1, "series whose attachment did not change AND whose aggregate inputs (title) did not actually change must NOT be re-indexed")
}

// TestPersistMetadata_SkipsPersonIndexForUnchangedAuthor verifies that an
// author who was already attached to the book before the apply is not
// re-indexed when the same author is re-applied. persons_fts has no
// aggregate columns, so re-indexing an unchanged person is pure churn.
func TestPersistMetadata_SkipsPersonIndexForUnchangedAuthor(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeEPUB)
	// Pre-attach an existing author with ID 1 (matches what the stub
	// person finder returns for the first call).
	existingPerson := &models.Person{ID: 1, LibraryID: book.LibraryID, Name: "Existing Author"}
	book.Authors = []*models.Author{{
		BookID:   book.ID,
		PersonID: existingPerson.ID,
		Person:   existingPerson,
	}}

	indexer := &stubSearchIndexer{}
	h := &handler{
		enrich: &enrichDeps{
			bookStore:     &stubBookStoreForPersist{book: book},
			relStore:      &stubRelStoreForApply{},
			personFinder:  &stubPersonFinderForPersist{},
			searchIndexer: indexer,
		},
	}

	md := &mediafile.ParsedMetadata{
		Authors: []mediafile.ParsedAuthor{{Name: "Existing Author", Role: "writer"}},
	}
	err := h.persistMetadata(context.Background(), book, file, md, "test", "plugin-id", testLogger())
	require.NoError(t, err)

	assert.NotContains(t, indexer.indexedPersonIDs, 1, "author whose attachment did not change must NOT be re-indexed (avoids persons_fts churn)")
}
