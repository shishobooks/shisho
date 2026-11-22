package shishoapp

import (
	"archive/zip"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entryOf is the first entry of kind for id in resp, or nil.
func entryOf(resp FeedResponse, kind string, id int) *FeedEntry {
	for i := range resp.Entries {
		if resp.Entries[i].Kind == kind && resp.Entries[i].ID == id {
			return &resp.Entries[i]
		}
	}
	return nil
}

// addCatalog fills Library 1 with a Book that names a Series, a Person, a
// Genre, a Tag, and a Publisher, each with an alias, and a File with an
// identifier.
func (f *fixture) addCatalog() {
	f.t.Helper()
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	f.exec(`UPDATE books SET description = 'Spice.' WHERE id = 1`)
	f.exec(`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'Frank Herbert', 'Herbert, Frank', 'file')`)
	f.exec(`INSERT INTO person_aliases (person_id, name, library_id) VALUES (1, 'F. Herbert', 1)`)
	f.exec(`INSERT INTO authors (id, book_id, person_id, sort_order) VALUES (1, 1, 1, 1)`)
	f.exec(`INSERT INTO series (id, library_id, name, name_source, sort_name, sort_name_source, description) VALUES (1, 1, 'Dune Chronicles', 'file', 'Dune Chronicles', 'file', 'Arrakis.')`)
	f.exec(`INSERT INTO series_aliases (series_id, name, library_id) VALUES (1, 'Dune Saga', 1)`)
	f.exec(`INSERT INTO book_series (id, book_id, series_id, series_number, sort_order) VALUES (1, 1, 1, 1, 1)`)
	f.exec(`INSERT INTO genres (id, library_id, name) VALUES (1, 1, 'Science Fiction')`)
	f.exec(`INSERT INTO genre_aliases (genre_id, name, library_id) VALUES (1, 'SF', 1)`)
	f.exec(`INSERT INTO book_genres (id, book_id, genre_id) VALUES (1, 1, 1)`)
	f.exec(`INSERT INTO tags (id, library_id, name) VALUES (1, 1, 'Desert')`)
	f.exec(`INSERT INTO tag_aliases (tag_id, name, library_id) VALUES (1, 'Sand', 1)`)
	f.exec(`INSERT INTO book_tags (id, book_id, tag_id) VALUES (1, 1, 1)`)
	f.exec(`INSERT INTO publishers (id, library_id, name) VALUES (1, 1, 'Chilton')`)
	f.exec(`INSERT INTO publisher_aliases (publisher_id, name, library_id) VALUES (1, 'Chilton Books', 1)`)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, publisher_id) VALUES (1, 1, 1, '/dune.epub', 'epub', 1, 1)`)
	f.exec(`INSERT INTO file_identifiers (id, file_id, type, value, source) VALUES (1, 1, 'isbn_13', '9780441013593', 'file')`)
}

func TestFeed_CatalogResourceDocuments(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addCatalog()
	f.addUser()
	credential := f.signIn()

	resp := f.changes(credential, 0, 0)

	book := entryOf(resp, FeedKindBook, 1)
	require.NotNil(t, book)
	require.NotNil(t, book.Book)
	require.NotNil(t, book.Book.Description)
	assert.Equal(t, "Spice.", *book.Book.Description)
	assert.Equal(t, []FeedGenreRef{{GenreID: 1, Name: "Science Fiction"}}, book.Book.Genres)
	assert.Equal(t, []FeedTagRef{{TagID: 1, Name: "Desert"}}, book.Book.Tags)
	require.Len(t, book.Book.Files, 1)
	file := book.Book.Files[0]
	require.NotNil(t, file.Publisher)
	assert.Equal(t, FeedPublisherRef{PublisherID: 1, Name: "Chilton"}, *file.Publisher)
	assert.Equal(t, []FeedIdentifier{{Type: "isbn_13", Value: "9780441013593"}}, file.Identifiers)
	assert.Equal(t, []string{}, file.Pages, "only a CBZ has pages")

	series := entryOf(resp, FeedKindSeries, 1)
	require.NotNil(t, series)
	require.NotNil(t, series.Series)
	assert.Equal(t, 1, series.LibraryID)
	assert.Equal(t, "Dune Chronicles", series.Series.Name)
	require.NotNil(t, series.Series.Description)
	assert.Equal(t, "Arrakis.", *series.Series.Description)
	assert.Equal(t, []string{"Dune Saga"}, series.Series.Aliases)

	person := entryOf(resp, FeedKindPerson, 1)
	require.NotNil(t, person)
	require.NotNil(t, person.Person)
	assert.Equal(t, "Herbert, Frank", person.Person.SortName)
	assert.Equal(t, []string{"F. Herbert"}, person.Person.Aliases)

	genre := entryOf(resp, FeedKindGenre, 1)
	require.NotNil(t, genre)
	require.NotNil(t, genre.Genre)
	assert.Equal(t, FeedGenre{ID: 1, LibraryID: 1, Name: "Science Fiction", Aliases: []string{"SF"}}, *genre.Genre)

	tag := entryOf(resp, FeedKindTag, 1)
	require.NotNil(t, tag)
	require.NotNil(t, tag.Tag)
	assert.Equal(t, FeedTag{ID: 1, LibraryID: 1, Name: "Desert", Aliases: []string{"Sand"}}, *tag.Tag)

	publisher := entryOf(resp, FeedKindPublisher, 1)
	require.NotNil(t, publisher)
	require.NotNil(t, publisher.Publisher)
	assert.Equal(t, FeedPublisher{ID: 1, LibraryID: 1, Name: "Chilton", Aliases: []string{"Chilton Books"}}, *publisher.Publisher)
}

// A CBZ's pages are the archive's images in the order the server reads them
// (ADR 0025): naturally sorted, without ComicInfo.xml or other files.
func TestFeed_CBZCarriesItsPages(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Comics")
	f.addBook(1, 1, "Saga")
	path := filepath.Join(t.TempDir(), "saga.cbz")
	out, err := os.Create(path)
	require.NoError(t, err)
	w := zip.NewWriter(out)
	for _, name := range []string{"ComicInfo.xml", "page10.jpg", "page2.jpg", "page1.jpg", "notes.txt"} {
		entry, err := w.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte("x"))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, out.Close())
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (1, 1, 1, ?, 'cbz', 1)`, path)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (2, 1, 1, '/missing.cbz', 'cbz', 1)`)
	f.addUser()
	credential := f.signIn()

	book := entryOf(f.changes(credential, 0, 0), FeedKindBook, 1)
	require.NotNil(t, book)
	require.Len(t, book.Book.Files, 2)
	assert.Equal(t, []string{"page1.jpg", "page2.jpg", "page10.jpg"}, book.Book.Files[0].Pages)
	assert.Equal(t, []string{}, book.Book.Files[1].Pages, "a CBZ the server can't read has none")
}

// A change to a resource moves its own entry, and to a join table or a name
// a Book carries, the Book's (ADR 0023).
func TestFeed_ResourceAndJoinTableChangesMoveTheirRecords(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addCatalog()
	f.addBook(2, 1, "Emma")
	userID := f.addUser()
	credential := f.signIn()

	steps := []struct {
		name  string
		write string
		want  []string
	}{
		{"description", `UPDATE books SET description = 'Melange.' WHERE id = 1`, []string{"book:1"}},
		{"person renamed", `UPDATE persons SET name = 'F. Herbert' WHERE id = 1`, []string{"book:1", "person:1"}},
		{"person alias added", `INSERT INTO person_aliases (id, person_id, name, library_id) VALUES (9, 1, 'Frank H.', 1)`, []string{"person:1"}},
		{"person alias renamed", `UPDATE person_aliases SET name = 'Frank Herbert Jr.' WHERE id = 9`, []string{"person:1"}},
		{"person alias removed", `DELETE FROM person_aliases WHERE id = 9`, []string{"person:1"}},
		{"series described", `UPDATE series SET description = 'Desert planet.' WHERE id = 1`, []string{"series:1"}},
		{"series renamed", `UPDATE series SET name = 'Dune' WHERE id = 1`, []string{"book:1", "series:1"}},
		{"series alias", `INSERT INTO series_aliases (series_id, name, library_id) VALUES (1, 'Arrakis', 1)`, []string{"series:1"}},
		{"genre added", `INSERT INTO book_genres (id, book_id, genre_id) VALUES (2, 2, 1)`, []string{"book:2"}},
		{"genre renamed", `UPDATE genres SET name = 'SF' WHERE id = 1`, []string{"book:1", "book:2", "genre:1"}},
		{"genre alias", `INSERT INTO genre_aliases (genre_id, name, library_id) VALUES (1, 'Sci-Fi', 1)`, []string{"genre:1"}},
		{"genre removed", `DELETE FROM book_genres WHERE id = 2`, []string{"book:2"}},
		{"tag added", `INSERT INTO book_tags (id, book_id, tag_id) VALUES (2, 2, 1)`, []string{"book:2"}},
		{"tag renamed", `UPDATE tags SET name = 'Dunes' WHERE id = 1`, []string{"book:1", "book:2", "tag:1"}},
		{"tag alias", `INSERT INTO tag_aliases (tag_id, name, library_id) VALUES (1, 'Sands', 1)`, []string{"tag:1"}},
		{"tag removed", `DELETE FROM book_tags WHERE id = 2`, []string{"book:2"}},
		{"publisher renamed", `UPDATE publishers SET name = 'Chilton Co.' WHERE id = 1`, []string{"book:1", "publisher:1"}},
		{"publisher alias", `INSERT INTO publisher_aliases (publisher_id, name, library_id) VALUES (1, 'Chilton Company', 1)`, []string{"publisher:1"}},
		{"publisher cleared", `UPDATE files SET publisher_id = NULL WHERE id = 1`, []string{"book:1"}},
		{"identifier added", `INSERT INTO file_identifiers (id, file_id, type, value, source) VALUES (2, 1, 'asin', 'B000', 'file')`, []string{"book:1"}},
		{"identifier changed", `UPDATE file_identifiers SET value = 'B001' WHERE id = 2`, []string{"book:1"}},
		{"identifier removed", `DELETE FROM file_identifiers WHERE id = 2`, []string{"book:1"}},
		{"genre deleted", `DELETE FROM genres WHERE id = 1`, []string{"book:1", "genre:1-"}},
		{"tag deleted", `DELETE FROM tags WHERE id = 1`, []string{"book:1", "tag:1-"}},
		{"publisher deleted", `DELETE FROM publishers WHERE id = 1`, []string{"publisher:1-"}},
		{"new genre", `INSERT INTO genres (id, library_id, name) VALUES (2, 1, 'Romance')`, []string{"genre:2"}},
		{"rated", fmt.Sprintf(`INSERT INTO user_books (user_id, book_id, rating) VALUES (%d, 1, 18)`, userID), []string{"user_book:1"}},
		{"on To Read", fmt.Sprintf(`UPDATE user_books SET to_read_position = 1 WHERE user_id = %d`, userID), []string{"user_book:1"}},
		{"reference", fmt.Sprintf(`UPDATE user_books SET to_read_position = NULL, reference = TRUE WHERE user_id = %d`, userID), []string{"user_book:1"}},
		{"record cleared", fmt.Sprintf(`DELETE FROM user_books WHERE user_id = %d`, userID), []string{"user_book:1-"}},
		{"book deleted", `DELETE FROM books WHERE id = 2`, []string{"book:2-"}},
		{"person deleted", `DELETE FROM persons WHERE id = 1`, []string{"book:1", "person:1-"}},
		{"series deleted", `DELETE FROM series WHERE id = 1`, []string{"book:1", "series:1-"}},
	}
	cursor := f.changes(credential, 0, 0).Cursor
	for _, step := range steps {
		f.exec(step.write)
		resp := f.changes(credential, cursor, 0)
		assert.ElementsMatch(t, step.want, summary(resp), step.name)
		cursor = resp.Cursor
	}
}

func TestFeed_UserBookDocument(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	userID := f.addUser()
	other := f.addOtherUser()
	f.exec(`INSERT INTO user_books (user_id, book_id, rating, to_read_position) VALUES (?, 1, 14, 2), (?, 1, 4, NULL)`, userID, other)
	credential := f.signIn()

	resp := f.changes(credential, 0, 0)
	entry := entryOf(resp, FeedKindUserBook, 1)
	require.NotNil(t, entry)
	require.NotNil(t, entry.UserBook)
	assert.Equal(t, 0, entry.LibraryID)
	rating, position := 14, 2
	assert.Equal(t, FeedUserBook{BookID: 1, Rating: &rating, ToReadPosition: &position}, *entry.UserBook)
	count := 0
	for _, e := range resp.Entries {
		if e.Kind == FeedKindUserBook {
			count++
		}
	}
	assert.Equal(t, 1, count, "another user's record stays theirs")
}

// A List reaches its owner and everyone it's shared with, each with their
// own permission, and nobody else.
func TestFeed_ListsReachTheirOwnerAndShares(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 1, "Emma")
	userID := f.addUser()
	other := f.addOtherUser()
	f.exec(`INSERT INTO lists (id, user_id, name, description, is_ordered, default_sort) VALUES (1, ?, 'Mine', 'Read soon', TRUE, 'manual')`, userID)
	f.exec(`INSERT INTO lists (id, user_id, name, default_sort) VALUES (2, ?, 'Shared', 'added_at_desc'), (3, ?, 'Private', 'added_at_desc')`, other, other)
	f.exec(`INSERT INTO list_books (list_id, book_id, added_at, sort_order) VALUES (1, 2, '2026-01-02 00:00:00', 1), (1, 1, '2026-01-01 00:00:00', 2)`)
	f.exec(`INSERT INTO list_shares (id, list_id, user_id, permission) VALUES (1, 2, ?, 'editor')`, userID)
	credential := f.signIn()

	resp := f.changes(credential, 0, 0)
	assert.Nil(t, entryOf(resp, FeedKindList, 3), "a List not shared with the user stays hidden")
	mine := entryOf(resp, FeedKindList, 1)
	require.NotNil(t, mine)
	require.NotNil(t, mine.List)
	assert.Equal(t, "Mine", mine.List.Name)
	assert.Equal(t, "owner", mine.List.Permission)
	assert.True(t, mine.List.IsOrdered)
	assert.Equal(t, "manual", mine.List.DefaultSort)
	assert.Equal(t, userID, mine.List.Owner.ID)
	require.Len(t, mine.List.Books, 2)
	assert.Equal(t, 2, mine.List.Books[0].BookID, "in sort order")
	shared := entryOf(resp, FeedKindList, 2)
	require.NotNil(t, shared)
	assert.Equal(t, "editor", shared.List.Permission)
	assert.Equal(t, "other", shared.List.Owner.Username)
	assert.ElementsMatch(t, []int{1, 2}, resp.Access.ListIDs)

	steps := []struct {
		name  string
		write string
		want  []string
	}{
		{"renamed", `UPDATE lists SET name = 'Mine!' WHERE id = 1`, []string{"list:1"}},
		{"book added", `INSERT INTO list_books (list_id, book_id) VALUES (2, 1)`, []string{"list:2"}},
		{"reordered", `UPDATE list_books SET sort_order = 3 WHERE list_id = 1 AND book_id = 2`, []string{"list:1"}},
		{"permission", `UPDATE list_shares SET permission = 'viewer' WHERE id = 1`, []string{"list:2"}},
		{"book deleted", `DELETE FROM books WHERE id = 1`, []string{"book:1-", "list:1", "list:2"}},
		{"unseen list", `UPDATE lists SET name = 'Still private' WHERE id = 3`, []string{}},
		{"deleted", `DELETE FROM lists WHERE id = 1`, []string{"list:1-"}},
	}
	cursor := resp.Cursor
	for _, step := range steps {
		f.exec(step.write)
		resp := f.changes(credential, cursor, 0)
		assert.ElementsMatch(t, step.want, summary(resp), step.name)
		cursor = resp.Cursor
	}

	// Unsharing is answered outside the cursor: the List leaves the access.
	f.exec(`DELETE FROM list_shares WHERE id = 1`)
	resp = f.changes(credential, cursor, 0)
	assert.Empty(t, resp.Access.ListIDs)
}

// Every page says which Libraries and Lists the user can see now, so the
// app drops what it lost and fetches what it gained (ADR 0023).
func TestFeed_AccessIsAnsweredOutsideTheCursor(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 2, "Emma")
	userID := f.addUser(1)
	credential := f.signIn()

	first := f.changes(credential, 0, 0)
	assert.Equal(t, []int{1}, first.Access.LibraryIDs)

	f.exec(`INSERT INTO user_library_access (user_id, library_id) VALUES (?, 2)`, userID)
	gained := f.changes(credential, first.Cursor, 0)
	assert.Empty(t, gained.Entries, "a Library gained has no entries after the cursor")
	assert.Equal(t, []int{1, 2}, gained.Access.LibraryIDs)

	// The app fetches the Library it gained whole.
	whole := f.changesIn(credential, "library_id=2", 0)
	assert.Equal(t, []string{"library:2", "book:2"}, summary(whole))

	f.exec(`DELETE FROM user_library_access WHERE user_id = ? AND library_id = 1`, userID)
	lost := f.changes(credential, gained.Cursor, 0)
	assert.Equal(t, []int{2}, lost.Access.LibraryIDs)

	// A role with every Library lists every one.
	f.exec(`DELETE FROM user_library_access WHERE user_id = ?`, userID)
	f.exec(`INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)`, userID)
	assert.Equal(t, []int{1, 2}, f.changes(credential, lost.Cursor, 0).Access.LibraryIDs)
}

func TestFeed_ScopedFetchNeedsAccess(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	userID := f.addUser(1)
	other := f.addOtherUser()
	f.exec(`INSERT INTO lists (id, user_id, name, default_sort) VALUES (1, ?, 'Theirs', 'added_at_desc')`, other)
	credential := f.signIn()

	rec := f.do(call{path: "/mobile/changes?cursor=0&library_id=2", credential: credential})
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = f.do(call{path: "/mobile/changes?cursor=0&list_id=1", credential: credential})
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	f.exec(`INSERT INTO list_shares (list_id, user_id, permission) VALUES (1, ?, 'viewer')`, userID)
	assert.Equal(t, []string{"list:1"}, summary(f.changesIn(credential, "list_id=1", 0)))
}

// A Library's whole fetch pages like the feed, and leaves out deletions and
// the user's own records, which the feed itself carries.
func TestFeed_ScopedFetchPages(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	for id := 1; id <= 3; id++ {
		f.addBook(id, 1, fmt.Sprintf("Book %d", id))
	}
	f.exec(`DELETE FROM books WHERE id = 2`)
	f.exec(`INSERT INTO genres (id, library_id, name) VALUES (1, 1, 'Drama')`)
	userID := f.addUser()
	f.exec(`INSERT INTO user_books (user_id, book_id, rating) VALUES (?, 1, 4)`, userID)
	credential := f.signIn()

	first := f.changesIn(credential, "library_id=1", 2)
	assert.True(t, first.HasMore)
	rest := f.changesIn(credential, fmt.Sprintf("library_id=1&cursor=%d", first.Cursor), 2)
	assert.False(t, rest.HasMore)
	assert.ElementsMatch(t, []string{"library:1", "book:1", "book:3", "genre:1"}, append(summary(first), summary(rest)...))
}

// A Book moved into a Library the user can't access leaves their copy.
func TestFeed_BookMovedOutOfReachLeaves(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	f.addBook(1, 1, "Dune")
	f.addUser(1)
	credential := f.signIn()

	cursor := f.changes(credential, 0, 0).Cursor
	f.exec(`UPDATE books SET library_id = 2 WHERE id = 1`)
	resp := f.changes(credential, cursor, 0)
	require.Equal(t, []string{"book_left:1-"}, summary(resp))
	assert.Equal(t, 1, resp.Entries[0].LibraryID, "it names the Library the Book left")
}

// changesIn reads one page of a scoped fetch, query naming the scope and
// any cursor.
func (f *fixture) changesIn(credential, query string, limit int) FeedResponse {
	f.t.Helper()
	path := "/mobile/changes?" + query
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}
	rec := f.do(call{path: path, credential: credential})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp FeedResponse
	require.NoError(f.t, decodeJSON(rec, &resp))
	return resp
}
