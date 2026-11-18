package books

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clearing a field in Edit leaves a protected empty slot: no value, with a
// manual source, so a later Scan leaves it empty (ADR 0027). Name, Language,
// and Abridged follow the same rule as every other field.
func TestUpdateFile_ClearLeavesProtectedEmptySlot(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	ctx := context.Background()

	library, _, file := seedBookAndFile(t, db, "Foo", nil, nil, models.FileRoleMain)
	name := "Edition"
	language := "en"
	abridged := true
	plugin := "plugin:test/enricher"
	file.Name, file.NameSource = &name, &plugin
	file.Language, file.LanguageSource = &language, &plugin
	file.Abridged, file.AbridgedSource = &abridged, &plugin
	_, err := db.NewUpdate().Model(file).
		Column("name", "name_source", "language", "language_source", "abridged", "abridged_source").
		WherePK().Exec(ctx)
	require.NoError(t, err)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	body := `{"name": "", "language": "", "abridged": ""}`
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	require.NoError(t, db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx))
	assert.Nil(t, updated.Name)
	assert.Nil(t, updated.Language)
	assert.Nil(t, updated.Abridged)
	manual := models.DataSourceManual
	assert.Equal(t, &manual, updated.NameSource)
	assert.Equal(t, &manual, updated.LanguageSource)
	assert.Equal(t, &manual, updated.AbridgedSource)
}
