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

func TestApplyMetadata_UpdatesMainFileName_WhenTitleChanges(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeEPUB)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "New Title"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name, "main file Name should be set")
	assert.Equal(t, "New Title", *file.Name)
	require.NotNil(t, file.NameSource, "main file NameSource should be set")
	assert.Equal(t, "plugin:test/enricher", *file.NameSource)
}

func TestApplyMetadata_DoesNotUpdateSupplementFileName(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypePDF)
	file.FileRole = models.FileRoleSupplement
	originalName := "Supplement.pdf"
	file.Name = &originalName

	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "New Title"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name)
	assert.Equal(t, "Supplement.pdf", *file.Name, "supplement Name must not be overwritten with book title")
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

	// The main file's Name should be set to the new title; the supplement's
	// should remain untouched (persistMetadata also guards this, but only
	// gets the chance because we picked the right targetFile).
	require.NotNil(t, main.Name, "main file Name should be set when applyMetadata falls back to main")
	assert.Equal(t, "New Title", *main.Name)
	assert.Nil(t, supplement.Name, "supplement Name must not be touched when applyMetadata falls back to main")
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
	require.NotNil(t, file.Name)
	assert.Equal(t, "Naruto v1", *file.Name, "file.Name must mirror the verbatim title")
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
