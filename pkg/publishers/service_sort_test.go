package publishers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The file count a publisher sorts by includes its sub-publishers' files,
// matching the count the list shows.
func TestListPublishers_SortsByFileCountIncludingDescendants(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := createTestLibrary(t, db)
	parent := createMergePublisher(t, db, lib, "Parent", nil)
	child := createMergePublisher(t, db, lib, "Child", &parent.ID)
	other := createMergePublisher(t, db, lib, "Other", nil)
	createFilesForPublisher(t, db, lib, parent.ID, 1)
	createFilesForPublisher(t, db, lib, child.ID, 3)
	createFilesForPublisher(t, db, lib, other.ID, 2)

	publishers, err := NewService(db).ListPublishers(context.Background(), ListPublishersOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldFileCount, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	names := make([]string, len(publishers))
	for i, p := range publishers {
		names[i] = p.Name
	}
	assert.Equal(t, []string{"Parent", "Child", "Other"}, names)
}

// The list handler passes the sort through, and date_added orders by the
// creation time rather than the name.
func TestListPublishersHandler_SortsByDateAdded(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, seed := range []struct {
		name string
		age  time.Duration
	}{{"Oldest", 0}, {"Newest", 2 * time.Hour}, {"Middle", time.Hour}} {
		require.NoError(t, NewService(db).CreatePublisher(ctx, &models.Publisher{LibraryID: lib.ID, Name: seed.name, CreatedAt: base.Add(seed.age)}))
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?library_id=%d&sort=date_added:desc", lib.ID), nil)
	rec := httptest.NewRecorder()
	c := newTestEcho(t).NewContext(req, rec)
	setAllAccessUser(c)
	require.NoError(t, newTestHandler(db).list(c))

	var resp ListPublishersResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Name
	}
	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, names)
}
