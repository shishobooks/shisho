package plugins

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newErrorTestContext builds a request context with the given path params
// (name, value pairs) and an optional JSON body.
func newErrorTestContext(body string, params ...string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	c := e.NewContext(req, httptest.NewRecorder())
	var names, values []string
	for i := 0; i+1 < len(params); i += 2 {
		names = append(names, params[i])
		values = append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	return c
}

// assertErrcode requires err to be an errcodes error with the given status,
// wire code, and message, which is what the error handler renders.
func assertErrcode(t *testing.T, err error, status int, code, message string) {
	t.Helper()
	require.Error(t, err)
	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
	assert.Equal(t, status, ecErr.HTTPCode)
	assert.Equal(t, code, ecErr.Code)
	assert.Equal(t, message, ecErr.Message)
}

// newFieldsTestHandler returns a handler whose manager holds one plugin,
// shisho/enricher, declaring the "title" field, and one plugin,
// shisho/plain, that is not a metadata enricher.
func newFieldsTestHandler() *handler {
	m := NewManager(nil, "", "")
	m.plugins[pluginKey("shisho", "enricher")] = &Runtime{
		scope:    "shisho",
		pluginID: "enricher",
		manifest: &Manifest{Capabilities: Capabilities{MetadataEnricher: &MetadataEnricherCap{Fields: []string{"title"}}}},
	}
	m.plugins[pluginKey("shisho", "plain")] = &Runtime{
		scope:    "shisho",
		pluginID: "plain",
		manifest: &Manifest{},
	}
	return &handler{manager: m}
}

func TestFieldSettingsHandlers_ReturnErrcodes(t *testing.T) {
	t.Parallel()
	h := newFieldsTestHandler()

	tests := []struct {
		name    string
		run     func() error
		status  int
		code    string
		message string
	}{
		{
			name: "global set on a plugin that is not an enricher",
			run: func() error {
				return h.setFieldSettings(newErrorTestContext(`{"fields":{}}`, "scope", "shisho", "id", "plain"))
			},
			status: http.StatusUnprocessableEntity, code: "invalid_state", message: "Plugin is not a metadata enricher.",
		},
		{
			name: "global set with an undeclared field",
			run: func() error {
				return h.setFieldSettings(newErrorTestContext(`{"fields":{"isbn":true}}`, "scope", "shisho", "id", "enricher"))
			},
			status: http.StatusUnprocessableEntity, code: "validation_error", message: "Unknown field: isbn",
		},
		{
			name: "library set on a plugin that is not an enricher",
			run: func() error {
				return h.setLibraryFieldSettings(newErrorTestContext(`{"fields":{}}`, "id", "1", "scope", "shisho", "pluginId", "plain"))
			},
			status: http.StatusUnprocessableEntity, code: "invalid_state", message: "Plugin is not a metadata enricher.",
		},
		{
			name: "library set with an undeclared field",
			run: func() error {
				return h.setLibraryFieldSettings(newErrorTestContext(`{"fields":{"isbn":true}}`, "id", "1", "scope", "shisho", "pluginId", "enricher"))
			},
			status: http.StatusUnprocessableEntity, code: "validation_error", message: "Unknown field: isbn",
		},
		{
			name:   "library get with a non-numeric library ID",
			run:    func() error { return h.getLibraryFieldSettings(newErrorTestContext("", "id", "abc")) },
			status: http.StatusNotFound, code: "not_found", message: "Library not found.",
		},
		{
			name:   "library set with a non-numeric library ID",
			run:    func() error { return h.setLibraryFieldSettings(newErrorTestContext("", "id", "abc")) },
			status: http.StatusNotFound, code: "not_found", message: "Library not found.",
		},
		{
			name:   "library reset with a non-numeric library ID",
			run:    func() error { return h.resetLibraryFieldSettings(newErrorTestContext("", "id", "abc")) },
			status: http.StatusNotFound, code: "not_found", message: "Library not found.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertErrcode(t, tt.run(), tt.status, tt.code, tt.message)
		})
	}
}

func TestLibraryOrderHandlers_NonNumericLibraryIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	h := &handler{}

	handlers := map[string]echo.HandlerFunc{
		"get":       h.getLibraryOrder,
		"set":       h.setLibraryOrder,
		"reset":     h.resetLibraryOrder,
		"reset all": h.resetAllLibraryOrders,
	}
	for name, fn := range handlers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := fn(newErrorTestContext("", "id", "abc", "hookType", "metadataEnricher"))
			assertErrcode(t, err, http.StatusNotFound, "not_found", "Library not found.")
		})
	}
}

// assertServerFault requires err to render as a 500: a plain error, not an
// errcodes error carrying a client status.
func assertServerFault(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var ecErr *errcodes.Error
	assert.NotErrorAs(t, err, &ecErr, "want a server fault, not an errcodes error")
}

// A search handler registered without its lookup dependencies is a server
// misconfiguration, not a bad request.
func TestSearchMetadata_MissingDependenciesIsServerFault(t *testing.T) {
	t.Parallel()
	h := &handler{manager: NewManager(nil, "", "")}
	c := newErrorTestContext(`{"query":"dune","book_id":1}`)
	auth.SetUser(c, &models.User{ID: 1, LibraryAccess: []*models.UserLibraryAccess{{}}})
	assertServerFault(t, h.searchMetadata(c))
}

// Search and apply load the book first. A missing book is a 404 and any
// other lookup failure is a 500, never a 404 that hides the fault.
func TestIdentifyHandlers_BookLookupErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		lookup   error
		notFound bool
	}{
		{name: "missing book", lookup: errcodes.NotFound("Book"), notFound: true},
		{name: "database fault", lookup: errors.New("database is locked")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{retrieveErr: tt.lookup}}
			h := newApplyTestHandler(store)

			search := newErrorTestContext(`{"query":"dune","book_id":1}`)
			auth.SetUser(search, &models.User{ID: 1, LibraryAccess: []*models.UserLibraryAccess{{}}})
			apply := newApplyEchoContext(t, map[string]any{"title": "Dune"})
			for _, err := range []error{h.searchMetadata(search), h.applyMetadata(apply)} {
				if tt.notFound {
					assertErrcode(t, err, http.StatusNotFound, "not_found", "Book not found.")
				} else {
					assertServerFault(t, err)
				}
			}
		})
	}
}

// A repository URL outside the allowed host is a rejected payload value, so
// it is a 422 that keeps its invalid_repo_url code.
func TestAddRepository_InvalidURLIsValidationError(t *testing.T) {
	t.Parallel()
	h := &handler{}
	c := newErrorTestContext(`{"url":"https://example.com/repo.json","scope":"community"}`)
	assertErrcode(t, h.addRepository(c), http.StatusUnprocessableEntity, "invalid_repo_url",
		"Invalid repository URL. Only GitHub raw content URLs are allowed (https://raw.githubusercontent.com/...).")
}
