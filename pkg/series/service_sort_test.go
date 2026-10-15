package series

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

func TestListSeries_SortsByBookCount(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db)
	lib := createSeriesDeleteLibrary(t, db)
	one := createNamedSeries(t, svc, lib, "One")
	three := createNamedSeries(t, svc, lib, "Three")
	two := createNamedSeries(t, svc, lib, "Two")
	createSeriesBook(t, db, lib, nil, one.ID, three.ID, two.ID)
	createSeriesBook(t, db, lib, nil, three.ID, two.ID)
	createSeriesBook(t, db, lib, nil, three.ID)

	series, err := svc.ListSeries(ctx, ListSeriesOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldBookCount, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	names := make([]string, len(series))
	for i, s := range series {
		names[i] = s.Name
	}
	assert.Equal(t, []string{"Three", "Two", "One"}, names)
}

// The list handler passes the sort through, and date_added orders by the
// creation time rather than the name.
func TestListSeriesHandler_SortsByDateAdded(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	lib := createSeriesDeleteLibrary(t, db)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, seed := range []struct {
		name string
		age  time.Duration
	}{{"Oldest", 0}, {"Newest", 2 * time.Hour}, {"Middle", time.Hour}} {
		require.NoError(t, NewService(db).CreateSeries(ctx, &models.Series{LibraryID: lib.ID, Name: seed.name, NameSource: models.DataSourceManual, CreatedAt: base.Add(seed.age)}))
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?library_id=%d&sort=date_added:desc", lib.ID), nil)
	rec := httptest.NewRecorder()
	c := newTestEchoSeries(t).NewContext(req, rec)
	setAllAccessUser(c)
	require.NoError(t, newSeriesHandler(db).list(c))

	var resp ListSeriesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Name
	}
	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, names)
}
