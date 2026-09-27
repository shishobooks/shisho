package plugins

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
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
			status: http.StatusBadRequest, code: "bad_request", message: "Plugin is not a metadata enricher.",
		},
		{
			name: "global set with an undeclared field",
			run: func() error {
				return h.setFieldSettings(newErrorTestContext(`{"fields":{"isbn":true}}`, "scope", "shisho", "id", "enricher"))
			},
			status: http.StatusBadRequest, code: "bad_request", message: "Unknown field: isbn",
		},
		{
			name: "library set on a plugin that is not an enricher",
			run: func() error {
				return h.setLibraryFieldSettings(newErrorTestContext(`{"fields":{}}`, "id", "1", "scope", "shisho", "pluginId", "plain"))
			},
			status: http.StatusBadRequest, code: "bad_request", message: "Plugin is not a metadata enricher.",
		},
		{
			name: "library set with an undeclared field",
			run: func() error {
				return h.setLibraryFieldSettings(newErrorTestContext(`{"fields":{"isbn":true}}`, "id", "1", "scope", "shisho", "pluginId", "enricher"))
			},
			status: http.StatusBadRequest, code: "bad_request", message: "Unknown field: isbn",
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
