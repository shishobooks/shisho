package kobo

import (
	"regexp"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int { return &i }

func entryOn(id string, bookID int, fileID *int, deliveredType string) *SyncEntry {
	return &SyncEntry{DeviceID: "d", EntryID: id, BookID: &bookID, FileID: fileID, DeliveredFileType: deliveredType, RevisionID: "rev-" + id}
}

func TestPlanEntries(t *testing.T) {
	t.Parallel()
	kepub := models.FileTypeKePub

	t.Run("a still-qualifying File keeps its entry, even beside a newly tied File", func(t *testing.T) {
		t.Parallel()
		plan := planEntries(
			[]*SyncEntry{entryOn("a", 1, intPtr(10), kepub)},
			[]DeliveredFile{{FileID: 10, BookID: 1, DeliveredFileType: kepub}, {FileID: 11, BookID: 1, DeliveredFileType: kepub}},
		)
		assert.Equal(t, []string{"a"}, entryIDs(plan.Kept))
		assert.Empty(t, plan.HandedOver)
		assert.Empty(t, plan.Removed)
		assert.Equal(t, []DeliveredFile{{FileID: 11, BookID: 1, DeliveredFileType: kepub}}, plan.Added, "tied Files each hold their own entry")
	})

	t.Run("a File that stops qualifying hands its entry to a newly qualifying File of the same type", func(t *testing.T) {
		t.Parallel()
		plan := planEntries(
			[]*SyncEntry{entryOn("a", 1, intPtr(10), kepub)},
			[]DeliveredFile{{FileID: 12, BookID: 1, DeliveredFileType: kepub}},
		)
		require.Len(t, plan.HandedOver, 1)
		assert.Equal(t, "a", plan.HandedOver[0].Entry.EntryID)
		assert.Equal(t, 12, plan.HandedOver[0].To.FileID)
		assert.Empty(t, plan.Added)
		assert.Empty(t, plan.Removed)
	})

	t.Run("an entry whose File was deleted is handed over too", func(t *testing.T) {
		t.Parallel()
		plan := planEntries(
			[]*SyncEntry{entryOn("a", 1, nil, kepub)},
			[]DeliveredFile{{FileID: 12, BookID: 1, DeliveredFileType: kepub}},
		)
		require.Len(t, plan.HandedOver, 1)
		assert.Equal(t, 12, plan.HandedOver[0].To.FileID)
	})

	t.Run("with no File to take it, the entry is removed", func(t *testing.T) {
		t.Parallel()
		plan := planEntries(
			[]*SyncEntry{entryOn("a", 1, intPtr(10), kepub), entryOn("b", 2, intPtr(20), kepub)},
			// Book 2's File qualifies again, but in another book.
			[]DeliveredFile{{FileID: 30, BookID: 3, DeliveredFileType: kepub}},
		)
		assert.Equal(t, []string{"a", "b"}, entryIDs(plan.Removed))
		assert.Equal(t, []DeliveredFile{{FileID: 30, BookID: 3, DeliveredFileType: kepub}}, plan.Added)
	})

	t.Run("a change of delivered type is a removal and an addition", func(t *testing.T) {
		t.Parallel()
		plan := planEntries(
			[]*SyncEntry{entryOn("a", 1, intPtr(10), kepub)},
			[]DeliveredFile{{FileID: 10, BookID: 1, DeliveredFileType: models.FileTypeEPUB}},
		)
		assert.Equal(t, []string{"a"}, entryIDs(plan.Removed))
		assert.Equal(t, []DeliveredFile{{FileID: 10, BookID: 1, DeliveredFileType: models.FileTypeEPUB}}, plan.Added)
		assert.Empty(t, plan.HandedOver)
	})

	t.Run("two freed entries and one File: the older entry takes it", func(t *testing.T) {
		t.Parallel()
		older := entryOn("shisho-5", 1, intPtr(10), kepub)
		newer := entryOn("shisho-e1", 1, intPtr(11), kepub)
		newer.CreatedAt = older.CreatedAt.Add(1)
		plan := planEntries(
			[]*SyncEntry{newer, older},
			[]DeliveredFile{{FileID: 12, BookID: 1, DeliveredFileType: kepub}},
		)
		require.Len(t, plan.HandedOver, 1)
		assert.Equal(t, "shisho-5", plan.HandedOver[0].Entry.EntryID)
		assert.Equal(t, []string{"shisho-e1"}, entryIDs(plan.Removed))
	})

	t.Run("a File moved to another book keeps its entry", func(t *testing.T) {
		t.Parallel()
		plan := planEntries(
			[]*SyncEntry{entryOn("a", 1, intPtr(10), kepub)},
			[]DeliveredFile{{FileID: 10, BookID: 2, DeliveredFileType: kepub}},
		)
		assert.Equal(t, []string{"a"}, entryIDs(plan.Kept))
	})
}

// A new entry's id can never be read as a File's Shisho id, which the
// entries recorded before entries had ids use.
func TestNewEntryID_CannotMatchAFileID(t *testing.T) {
	t.Parallel()
	fileIDForm := regexp.MustCompile(`^shisho-\d+$`)
	seen := map[string]bool{}
	for range 100 {
		id := newEntryID()
		assert.NotRegexp(t, fileIDForm, id)
		key, ok := parseEntryID(id + "-abcd1234")
		assert.True(t, ok)
		assert.Equal(t, id, key, "a cover id's suffix comes off")
		assert.False(t, seen[id])
		seen[id] = true
	}
}

func TestParseEntryID(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		in, want string
		ok       bool
	}{
		{"shisho-12", "shisho-12", true},
		{"shisho-12-abcd1234", "shisho-12", true},
		{"shisho-eabc123", "shisho-eabc123", true},
		{"shisho-", "", false},
		{"shisho--x", "", false},
		{"kobo-123", "", false},
		{"0b5f6c2e-store-book", "", false},
	} {
		got, ok := parseEntryID(tt.in)
		assert.Equal(t, tt.ok, ok, tt.in)
		assert.Equal(t, tt.want, got, tt.in)
	}
}
