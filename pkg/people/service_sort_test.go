package people

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

func personNames(people []*models.Person) []string {
	names := make([]string, len(people))
	for i, p := range people {
		names[i] = p.Name
	}
	return names
}

func TestListPeople_SortsByAuthoredAndNarratedCounts(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db)
	lib := createPersonDeleteLibrary(t, db)
	writer := createNamedPerson(t, svc, lib, "Writer")
	narrator := createNamedPerson(t, svc, lib, "Narrator")
	both := createNamedPerson(t, svc, lib, "Both")
	createAuthoredBook(t, db, lib, models.DataSourceManual, writer.ID, both.ID)
	createAuthoredBook(t, db, lib, models.DataSourceManual, writer.ID)
	createNarratedFile(t, db, lib, nil, narrator.ID, both.ID)
	createNarratedFile(t, db, lib, nil, narrator.ID)

	byAuthored, err := svc.ListPeople(ctx, ListPeopleOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldAuthoredBookCount, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Writer", "Both", "Narrator"}, personNames(byAuthored))

	byNarrated, err := svc.ListPeople(ctx, ListPeopleOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldNarratedFileCount, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Narrator", "Both", "Writer"}, personNames(byNarrated))
}

// The list handler passes the sort through, and date_added orders by the
// creation time rather than the name.
func TestListPeopleHandler_SortsByDateAdded(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, seed := range []struct {
		name string
		age  time.Duration
	}{{"Oldest", 0}, {"Newest", 2 * time.Hour}, {"Middle", time.Hour}} {
		require.NoError(t, NewService(db).CreatePerson(ctx, &models.Person{LibraryID: lib.ID, Name: seed.name, CreatedAt: base.Add(seed.age)}))
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?library_id=%d&sort=date_added:desc", lib.ID), nil)
	rec := httptest.NewRecorder()
	c := newTestEcho(t).NewContext(req, rec)
	setAllAccessUser(c)
	require.NoError(t, newTestHandler(db).list(c))

	var resp ListPeopleResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Name
	}
	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, names)
}

func TestListPeople_SortsByAuthoredBooksNotRoles(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db)
	lib := createPersonDeleteLibrary(t, db)
	artist := createNamedPerson(t, svc, lib, "Artist")
	writer := createNamedPerson(t, svc, lib, "Writer")
	addAuthorRoles(t, db, createAuthoredBook(t, db, lib, models.DataSourceManual), artist.ID,
		models.AuthorRoleWriter, models.AuthorRolePenciller, models.AuthorRoleInker)
	createAuthoredBook(t, db, lib, models.DataSourceManual, writer.ID)
	createAuthoredBook(t, db, lib, models.DataSourceManual, writer.ID)

	people, err := svc.ListPeople(ctx, ListPeopleOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldAuthoredBookCount, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Writer", "Artist"}, personNames(people))
}
