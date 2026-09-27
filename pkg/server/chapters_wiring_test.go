package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Replacing a File's chapters through the real routes recomputes its review
// state when the review criteria require chapters. This covers the wiring in
// server.go that hands the chapters handler a books service with app
// settings attached.
func TestReplaceChapters_RecomputesReviewedForFile(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeM4B, nil)
	criteria := review.Default()
	criteria.AudioFields = append(criteria.AudioFields, review.FieldChapters)
	require.NoError(t, review.Save(f.ctx, appsettings.NewService(f.db), criteria))
	require.NoError(t, review.RecomputeForBook(f.ctx, f.db, seeded.bookID, criteria))
	require.False(t, f.reviewed(seeded.fileID), "precondition: the File is not reviewed without chapters")

	token, err := f.authSvc.GenerateToken(f.admin)
	require.NoError(t, err)
	body := `{"chapters":[{"title":"Opening","start_timestamp_ms":0,"children":[]}]}`
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/books/files/%d/chapters", seeded.fileID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "response body: %s", rec.Body.String())

	assert.True(t, f.reviewed(seeded.fileID), "the File becomes reviewed once it has chapters")
}
