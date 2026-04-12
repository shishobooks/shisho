package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		},
	}
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

func TestApplyMetadata_OrganizesFiles_WhenTitleChanges(t *testing.T) {
	t.Parallel()

	book := &models.Book{ID: 1, LibraryID: 1, Title: "Old Title"}
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

	book := &models.Book{ID: 1, LibraryID: 1, Title: "Book"}
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

	book := &models.Book{ID: 1, LibraryID: 1, Title: "Book"}
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

func TestApplyMetadata_SkipsOrganize_WhenNoRelevantFieldsChange(t *testing.T) {
	t.Parallel()

	book := &models.Book{ID: 1, LibraryID: 1, Title: "Book"}
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
