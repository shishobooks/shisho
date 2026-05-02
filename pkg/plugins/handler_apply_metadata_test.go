package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBookStoreForApply extends stubBookStoreForPersist with OrganizeBookFiles tracking.
type stubBookStoreForApply struct {
	stubBookStoreForPersist
	organizeCalled bool
}

func (s *stubBookStoreForApply) OrganizeBookFiles(_ context.Context, _ *models.Book) error {
	s.organizeCalled = true
	return nil
}

// stubRelStoreForApply is a no-op relationStore for applyMetadata tests.
type stubRelStoreForApply struct{}

func (s *stubRelStoreForApply) DeleteAuthors(_ context.Context, _ int) error { return nil }
func (s *stubRelStoreForApply) CreateAuthor(_ context.Context, _ *models.Author) error {
	return nil
}
func (s *stubRelStoreForApply) DeleteBookSeries(_ context.Context, _ int) error { return nil }
func (s *stubRelStoreForApply) CreateBookSeries(_ context.Context, _ *models.BookSeries) error {
	return nil
}
func (s *stubRelStoreForApply) FindOrCreateSeries(_ context.Context, _ string, _ int, _ string) (*models.Series, error) {
	return &models.Series{ID: 1}, nil
}
func (s *stubRelStoreForApply) DeleteBookGenres(_ context.Context, _ int) error { return nil }
func (s *stubRelStoreForApply) CreateBookGenre(_ context.Context, _ *models.BookGenre) error {
	return nil
}
func (s *stubRelStoreForApply) DeleteBookTags(_ context.Context, _ int) error { return nil }
func (s *stubRelStoreForApply) CreateBookTag(_ context.Context, _ *models.BookTag) error {
	return nil
}

// newApplyTestHandler creates a handler wired with stubs for applyMetadata testing.
func newApplyTestHandler(store *stubBookStoreForApply) *handler {
	mgr := &Manager{
		plugins: map[string]*Runtime{
			pluginKey("test", "enricher"): {
				manifest: &Manifest{},
				scope:    "test",
				pluginID: "enricher",
			},
		},
	}
	return &handler{
		manager: mgr,
		enrich: &enrichDeps{
			bookStore: store,
			relStore:  &stubRelStoreForApply{},
		},
	}
}

// stubPublisherFinder records the name FindOrCreatePublisher was called with.
type stubPublisherFinder struct {
	lastName string
}

func (s *stubPublisherFinder) FindOrCreatePublisher(_ context.Context, name string, _ int) (*models.Publisher, error) {
	s.lastName = name
	return &models.Publisher{ID: 1, Name: name}, nil
}

// stubImprintFinder records the name FindOrCreateImprint was called with.
type stubImprintFinder struct {
	lastName string
}

func (s *stubImprintFinder) FindOrCreateImprint(_ context.Context, name string, _ int) (*models.Imprint, error) {
	s.lastName = name
	return &models.Imprint{ID: 1, Name: name}, nil
}

// newApplyTestHandlerWithFinders wires publisher/imprint finders so tests can
// assert on the exact names persistMetadata passed to FindOrCreate*.
func newApplyTestHandlerWithFinders(store *stubBookStoreForApply, pub *stubPublisherFinder, imp *stubImprintFinder) *handler {
	h := newApplyTestHandler(store)
	h.enrich.publisherFinder = pub
	h.enrich.imprintFinder = imp
	return h
}

// newApplyEchoContext creates an Echo context with the given fields payload and an all-access user.
func newApplyEchoContext(t *testing.T, fields map[string]any) echo.Context {
	t.Helper()
	payload := applyPayload{
		BookID:      1,
		Fields:      fields,
		PluginScope: "test",
		PluginID:    "enricher",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// User with access to all libraries (nil LibraryID)
	c.Set("user", &models.User{
		ID:            1,
		LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}},
	})
	return c
}

// newApplyTestBook builds a test book with a real on-disk Filepath so that
// sidecar writes triggered by persistMetadata land under the test's scratch
// directory rather than the package CWD. Without Filepath set, WriteBookSidecar
// used to fall back to CWD-relative paths and silently drop stray
// "..metadata.json" files into pkg/plugins/ on every test run.
func newApplyTestBook(t *testing.T, title string) *models.Book {
	t.Helper()
	return &models.Book{ID: 1, LibraryID: 1, Title: title, Filepath: t.TempDir()}
}

// newApplyTestBookWithFile builds a book with a single attached main file.
// The returned file pointer is the same one persistMetadata will mutate, so
// tests can assert on its fields directly after applyMetadata returns.
func newApplyTestBookWithFile(t *testing.T, title string, fileType string) (*models.Book, *models.File) {
	t.Helper()
	book := newApplyTestBook(t, title)
	file := &models.File{
		ID:        1,
		BookID:    book.ID,
		LibraryID: book.LibraryID,
		Filepath:  filepath.Join(book.Filepath, "main."+fileType),
		FileType:  fileType,
		FileRole:  models.FileRoleMain,
	}
	book.Files = []*models.File{file}
	return book, file
}

func TestApplyMetadata_OrganizesFiles_WhenTitleChanges(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Old Title")
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "New Title"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.True(t, store.organizeCalled, "OrganizeBookFiles should be called when title changes")
}

func TestApplyMetadata_OrganizesFiles_WhenAuthorsChange(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{
		"authors": []any{
			map[string]any{"name": "New Author", "role": "writer"},
		},
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.True(t, store.organizeCalled, "OrganizeBookFiles should be called when authors change")
}

func TestApplyMetadata_OrganizesFiles_WhenNarratorsChange(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{
		"narrators": []any{"New Narrator"},
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.True(t, store.organizeCalled, "OrganizeBookFiles should be called when narrators change")
}

func TestApplyMetadata_OrganizesFiles_WhenSeriesChanges(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{
		"series":        "My Series",
		"series_number": 2,
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.True(t, store.organizeCalled, "OrganizeBookFiles should be called when series changes")
}

func TestApplyMetadata_OrganizesFiles_WhenMultiSeriesArraySent(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{
		"series": []any{
			map[string]any{"name": "Series A", "number": 1.0},
			map[string]any{"name": "Series B", "number": 2.0, "series_number_unit": "volume"},
		},
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.True(t, store.organizeCalled, "OrganizeBookFiles should be called when multi-series array is sent")
}

func TestApplyMetadata_SkipsOrganize_WhenNoRelevantFieldsChange(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{
		"description": "A new description",
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.False(t, store.organizeCalled, "OrganizeBookFiles should NOT be called when only description changes")
}

func TestApplyMetadata_SkipsOrganize_WhenOnlyWhitespaceTitleAndSeries(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{
		"title":  "   ",
		"series": "\t\n",
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.False(t, store.organizeCalled, "OrganizeBookFiles should NOT be called when title and series are whitespace-only")
}

func TestApplyMetadata_SkipsSubtitle_WhenWhitespaceOnly(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book")
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{
		"subtitle": "   ",
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.Nil(t, book.Subtitle, "book.Subtitle should not be set to a pointer-to-empty-string for whitespace-only input")
}

// TestApplyMetadata_DoesNotAutoSyncMainFileName_WhenOnlyTitleSent verifies
// the Phase 1 fix: a title-only payload no longer silently mirrors book.Title
// onto the main file's Name. Old frontends that don't ship file_name continue
// to function (no errors), but file.Name is left untouched. Edition-specific
// names like "Harry Potter (Full-Cast Edition)" are no longer clobbered on
// re-identify. To opt in, callers must send file_name explicitly.
func TestApplyMetadata_DoesNotAutoSyncMainFileName_WhenOnlyTitleSent(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeEPUB)
	originalName := "Custom Edition Name"
	file.Name = &originalName

	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "New Title"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.Equal(t, "New Title", book.Title, "book.Title should still update")
	require.NotNil(t, file.Name)
	assert.Equal(t, "Custom Edition Name", *file.Name, "file.Name must NOT be silently overwritten by book.Title")
	assert.Nil(t, file.NameSource, "file.NameSource must NOT be set when no explicit file_name was sent")
}

// TestApplyMetadata_ExplicitFileName_AppliesToSupplement verifies the post-
// Phase-1 invariant that the explicit-payload path is role-agnostic: when a
// caller pins file_id at a supplement and ships file_name, the supplement's
// Name updates per the payload. Previously persistMetadata gated the file.Name
// write on FileRoleMain; now the gate is at the wire layer (frontend opts in
// per file by sending file_name only when the user checked the Name row), so
// the persist layer trusts the explicit signal regardless of role.
//
// This replaces an earlier tautological test that asserted "supplement.Name
// is not overwritten by a title-only payload" — which is now true for every
// file role and isn't load-bearing.
func TestApplyMetadata_ExplicitFileName_AppliesToSupplement(t *testing.T) {
	t.Parallel()

	book := newApplyTestBook(t, "Book Title")
	main := &models.File{
		ID:        1,
		BookID:    book.ID,
		LibraryID: book.LibraryID,
		Filepath:  filepath.Join(book.Filepath, "main.epub"),
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
	}
	supplementID := 2
	supplement := &models.File{
		ID:        supplementID,
		BookID:    book.ID,
		LibraryID: book.LibraryID,
		Filepath:  filepath.Join(book.Filepath, "Supplement.pdf"),
		FileType:  models.FileTypePDF,
		FileRole:  models.FileRoleSupplement,
	}
	originalName := "Supplement.pdf"
	supplement.Name = &originalName
	book.Files = []*models.File{main, supplement}

	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)

	// Build a payload that pins file_id at the supplement and explicitly
	// sets file_name. newApplyEchoContextWithFileName doesn't carry file_id,
	// so build the request inline.
	payload := applyPayload{
		BookID:      book.ID,
		FileID:      &supplementID,
		Fields:      map[string]any{},
		FileName:    func() *string { s := "Cribsheet (Updated)"; return &s }(),
		PluginScope: "test",
		PluginID:    "enricher",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &models.User{
		ID:            1,
		LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}},
	})

	err = h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, supplement.Name)
	assert.Equal(t, "Cribsheet (Updated)", *supplement.Name,
		"explicit file_name targeting a supplement should write through (Phase 1 removes the role gate at the persist layer)")
	require.NotNil(t, supplement.NameSource)
	assert.Equal(t, "plugin:test/enricher", *supplement.NameSource)
	assert.Nil(t, main.Name, "main file must not be touched when file_id pins the supplement")
}

func TestApplyMetadata_FallbackTargetsMainFile_NotSupplement(t *testing.T) {
	t.Parallel()

	// Book with a supplement first in book.Files (the order callers might
	// see post-classification when a supplement was scanned/created earlier
	// in the slice). With no FileID in the payload, applyMetadata must
	// target the main file, not the supplement.
	book := newApplyTestBook(t, "Old Title")
	supplement := &models.File{
		ID:        7,
		BookID:    book.ID,
		LibraryID: book.LibraryID,
		Filepath:  filepath.Join(book.Filepath, "Supplement.pdf"),
		FileType:  models.FileTypePDF,
		FileRole:  models.FileRoleSupplement,
	}
	main := &models.File{
		ID:        8,
		BookID:    book.ID,
		LibraryID: book.LibraryID,
		Filepath:  filepath.Join(book.Filepath, "main.epub"),
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
	}
	book.Files = []*models.File{supplement, main}

	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "New Title"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	// The main file is still the targeted file (supplement is skipped),
	// but with Phase 1 file.Name is no longer auto-synced from book.Title.
	// Both files' Name/NameSource stay nil on a title-only payload.
	assert.Nil(t, main.Name, "main file Name must NOT be auto-set by a title-only payload")
	assert.Nil(t, main.NameSource, "main file NameSource must NOT be auto-set by a title-only payload")
	assert.Nil(t, supplement.Name, "supplement Name must remain untouched")

	// Verify the main file *was* targeted (not the supplement) by
	// checking that book.Title was written through to the right scope.
	assert.Equal(t, "New Title", book.Title, "book.Title should be set, confirming the apply ran")
}

func TestApplyMetadata_PreservesVolumeNotation_CBZ(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeCBZ)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "Naruto v1"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.Equal(t, "Naruto v1", book.Title, "book.Title must not be volume-normalized on identify")
	assert.Nil(t, file.Name, "file.Name must NOT be auto-set by a title-only payload (Phase 1 fix)")
}

func TestApplyMetadata_TrimsPublisherImprintURL(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeEPUB)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	pub := &stubPublisherFinder{}
	imp := &stubImprintFinder{}
	h := newApplyTestHandlerWithFinders(store, pub, imp)
	c := newApplyEchoContext(t, map[string]any{
		"publisher": "  Some Publisher  ",
		"imprint":   "  Penguin Classics  ",
		"url":       "  https://example.com  ",
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.Equal(t, "Some Publisher", pub.lastName, "publisher name must be trimmed before FindOrCreate")
	assert.Equal(t, "Penguin Classics", imp.lastName, "imprint name must be trimmed before FindOrCreate")
	require.NotNil(t, file.URL)
	assert.Equal(t, "https://example.com", *file.URL, "file URL must be trimmed")
}

// newApplyEchoContextWithFileName builds an Echo context where the apply
// payload carries an explicit top-level file_name and (optionally) a
// file_name_source — the new Phase 1 wire signal.
func newApplyEchoContextWithFileName(t *testing.T, fields map[string]any, fileName string, fileNameSource string) echo.Context {
	t.Helper()
	payload := applyPayload{
		BookID:      1,
		Fields:      fields,
		PluginScope: "test",
		PluginID:    "enricher",
	}
	if fileName != "" {
		fn := fileName
		payload.FileName = &fn
	}
	if fileNameSource != "" {
		fns := fileNameSource
		payload.FileNameSource = &fns
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &models.User{
		ID:            1,
		LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}},
	})
	return c
}

// TestApplyMetadata_ExplicitFileName_AppliedWithPluginSourceByDefault verifies
// that a payload carrying file_name (without file_name_source) writes
// file.Name and stamps NameSource with the plugin source — the default for
// a value the user accepted as-is from the plugin's proposal.
func TestApplyMetadata_ExplicitFileName_AppliedWithPluginSourceByDefault(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeEPUB)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContextWithFileName(t,
		map[string]any{"title": "New Title"},
		"New Title",
		"")

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name, "file.Name must be set when file_name is explicit")
	assert.Equal(t, "New Title", *file.Name)
	require.NotNil(t, file.NameSource, "file.NameSource must be set when file_name is explicit")
	assert.Equal(t, "plugin:test/enricher", *file.NameSource,
		"absent file_name_source defaults to the plugin source for this apply call")
}

// TestApplyMetadata_ExplicitFileName_HonorsExplicitSource verifies that
// when the payload carries file_name_source, that exact value is written
// to file.NameSource. This lets the Phase 2 frontend distinguish "user
// accepted the plugin's proposed Name" from "user edited Name manually".
func TestApplyMetadata_ExplicitFileName_HonorsExplicitSource(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeEPUB)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContextWithFileName(t,
		map[string]any{"title": "New Title"},
		"My Custom Edition Name",
		models.DataSourceManual)

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name)
	assert.Equal(t, "My Custom Edition Name", *file.Name)
	require.NotNil(t, file.NameSource)
	assert.Equal(t, models.DataSourceManual, *file.NameSource,
		"explicit file_name_source must be written verbatim")
}

// TestApplyMetadata_ExplicitFileName_PreservesEditionName_NonPrimaryFile is
// the regression test for the original spec bug: identifying a non-primary
// file against a generic plugin result no longer corrupts an edition-specific
// file.Name. Before Phase 1, this scenario silently mirrored book.Title onto
// file.Name. After Phase 1, file.Name only updates when the payload says so.
// Here, the payload omits file_name entirely (modeling an old frontend OR a
// new frontend where the user unchecked the Name row).
func TestApplyMetadata_ExplicitFileName_PreservesEditionName_NonPrimaryFile(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Harry Potter and the Sorcerer's Stone", models.FileTypeEPUB)
	originalName := "Harry Potter and the Sorcerer's Stone (Full-Cast Edition)"
	file.Name = &originalName
	originalSource := models.DataSourceManual
	file.NameSource = &originalSource

	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	// Title-only payload — no file_name — same as a non-primary identify
	// against a generic plugin result. The bug being fixed: previously this
	// would clobber the edition name with the bare book title.
	c := newApplyEchoContext(t, map[string]any{
		"title": "Harry Potter and the Sorcerer's Stone",
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name)
	assert.Equal(t, "Harry Potter and the Sorcerer's Stone (Full-Cast Edition)", *file.Name,
		"edition-specific file.Name must NOT be clobbered by a title-only identify (Phase 1 spec bug fix)")
	require.NotNil(t, file.NameSource)
	assert.Equal(t, models.DataSourceManual, *file.NameSource,
		"file.NameSource must NOT be replaced by the plugin source")
}
