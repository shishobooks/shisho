package delivery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callHandler(t *testing.T, fn echo.HandlerFunc, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, &models.User{ID: 1})
	return rec, fn(c)
}

func decodeRules(t *testing.T, rec *httptest.ResponseRecorder) DefaultRulesResponse {
	t.Helper()
	var resp DefaultRulesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func TestHandler_SaveAndReset(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	insertTestUser(t, db, 1)
	h := &handler{service: NewService(db)}

	rec, err := callHandler(t, h.retrieve, "")
	require.NoError(t, err)
	got := decodeRules(t, rec)
	assert.True(t, got.FollowsBuiltIn)
	assert.Equal(t, BuiltInRules(), got.Rules)
	assert.Contains(t, got.Options, RuleOption{
		SourceFileType:     models.FileTypeCBZ,
		DeliveredFileTypes: []string{models.FileTypeCBZ, models.FileTypeKePub, models.FileTypeEPUB},
	})

	rec, err = callHandler(t, h.update, `{"rules":[{"source_file_type":"cbz","delivered_file_type":"epub"}]}`)
	require.NoError(t, err)
	got = decodeRules(t, rec)
	assert.False(t, got.FollowsBuiltIn)
	assert.Equal(t, []Rule{{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeEPUB}}, got.Rules)

	rec, err = callHandler(t, h.reset, "")
	require.NoError(t, err)
	got = decodeRules(t, rec)
	assert.True(t, got.FollowsBuiltIn)
	assert.Equal(t, BuiltInRules(), got.Rules)
}

func TestHandler_Update_RefusesAnInvalidList(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	insertTestUser(t, db, 1)
	h := &handler{service: NewService(db)}

	_, err := callHandler(t, h.update, `{"rules":[]}`)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode)
	assert.Equal(t, "Delivery Rules need at least one rule.", codeErr.Message)
}

func TestHandler_Update_StorageFailureIsAServerError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	insertTestUser(t, db, 1)
	h := &handler{service: NewService(db)}
	_, err := db.Exec("DROP TABLE user_delivery_rules")
	require.NoError(t, err)

	_, err = callHandler(t, h.update, `{"rules":[{"source_file_type":"epub","delivered_file_type":"epub"}]}`)
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr)
}
