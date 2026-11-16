package books

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateFile_ReadingDirection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileType string
		body     string
		status   int
		code     string
		want     *string
	}{
		{name: "sets right to left on a PDF", fileType: models.FileTypePDF, body: `{"reading_direction":"rtl"}`, status: http.StatusOK, want: strPtr(models.ReadingDirectionRTL)},
		{name: "sets left to right on a CBZ", fileType: models.FileTypeCBZ, body: `{"reading_direction":"ltr"}`, status: http.StatusOK, want: strPtr(models.ReadingDirectionLTR)},
		{name: "rejects a value that isn't a direction", fileType: models.FileTypeCBZ, body: `{"reading_direction":"up"}`, status: http.StatusUnprocessableEntity, code: "validation_error"},
		{name: "rejects an empty direction", fileType: models.FileTypeCBZ, body: `{"reading_direction":""}`, status: http.StatusUnprocessableEntity, code: "validation_error"},
		{name: "rejects a File that isn't Page-based", fileType: models.FileTypeEPUB, body: `{"reading_direction":"rtl"}`, status: http.StatusUnprocessableEntity, code: "invalid_state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			library, book := setupTestLibraryAndBook(t, db)
			var path string
			switch tt.fileType {
			case models.FileTypeCBZ:
				path = testgen.GenerateCBZ(t, book.Filepath, "book.cbz", testgen.CBZOptions{HasComicInfo: true})
			case models.FileTypePDF:
				path = testgen.GeneratePDF(t, book.Filepath, "book.pdf", testgen.PDFOptions{})
			default:
				path = testgen.WriteFile(t, book.Filepath, "book."+tt.fileType, []byte("content"))
			}
			file := setupTestFile(t, db, book, tt.fileType, path)
			user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

			req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)
			require.Equal(t, tt.status, rr.Code, rr.Body.String())
			if tt.code != "" {
				assert.Contains(t, rr.Body.String(), `"code":"`+tt.code+`"`)
			}

			var stored models.File
			require.NoError(t, db.NewSelect().Model(&stored).Where("id = ?", file.ID).Scan(t.Context()))
			assert.Equal(t, tt.want, stored.ReadingDirection)
			if tt.want == nil {
				assert.Nil(t, stored.ReadingDirectionSource)
				return
			}
			require.NotNil(t, stored.ReadingDirectionSource)
			assert.Equal(t, models.DataSourceManual, *stored.ReadingDirectionSource)

			written, err := sidecar.ReadFileSidecar(path)
			require.NoError(t, err)
			require.NotNil(t, written)
			assert.Equal(t, tt.want, written.ReadingDirection, "the edit is kept in the File's sidecar")
		})
	}
}
