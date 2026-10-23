package delivery

import (
	"context"
	"slices"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func insertTestUser(t *testing.T, db *bun.DB, id int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO users (id, username, password_hash, role_id) VALUES (?, ?, 'hash', 1)`, id, "reader"+string(rune('a'+id)))
	require.NoError(t, err)
}

// testFile is a File in a resolver case: its File Type, and whether it is a
// supplement.
type testFile struct {
	fileType   string
	supplement bool
}

// delivered is one File a resolver case expects to go: its index in the
// case's files and its delivered File Type.
type delivered struct {
	file int
	as   string
}

func buildBook(files []testFile) *models.Book {
	book := &models.Book{ID: 1, Title: "Book"}
	for i, f := range files {
		role := models.FileRoleMain
		if f.supplement {
			role = models.FileRoleSupplement
		}
		book.Files = append(book.Files, &models.File{ID: i + 1, BookID: 1, FileType: f.fileType, FileRole: role})
	}
	return book
}

func deliveredOf(t *testing.T, deliveries []Delivery) []delivered {
	t.Helper()
	got := []delivered{}
	for _, d := range deliveries {
		got = append(got, delivered{file: d.File.ID - 1, as: d.DeliveredFileType})
	}
	return got
}

// TestResolve_UserDefault saves each case's rules as the user's default (or
// leaves the default following the built-in rules when they are nil), loads
// the default back, and resolves the case's book against it.
func TestResolve_UserDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rules     []Rule
		files     []testFile
		ebook     []delivered
		audiobook []delivered
	}{
		{
			name:  "following the built-in rules delivers each File as itself",
			files: []testFile{{fileType: models.FileTypeCBZ}},
			ebook: []delivered{{0, models.FileTypeCBZ}},
		},
		{
			name:  "following the built-in rules, EPUB outranks CBZ and PDF",
			files: []testFile{{fileType: models.FileTypePDF}, {fileType: models.FileTypeCBZ}, {fileType: models.FileTypeEPUB}},
			ebook: []delivered{{2, models.FileTypeEPUB}},
		},
		{
			name:  "following the built-in rules, a KePub goes as itself",
			files: []testFile{{fileType: models.FileTypeKePub}, {fileType: models.FileTypeAZW3}},
			ebook: []delivered{{0, models.FileTypeKePub}},
		},
		{
			name:  "a custom list without a KePub rule does not deliver a KePub-only book",
			rules: []Rule{{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB}},
			files: []testFile{{fileType: models.FileTypeKePub}},
		},
		{
			name:  "following the built-in rules, EPUB outranks KePub",
			files: []testFile{{fileType: models.FileTypeKePub}, {fileType: models.FileTypeEPUB}},
			ebook: []delivered{{1, models.FileTypeEPUB}},
		},
		{
			name:  "a custom rule converts its source File Type",
			rules: []Rule{{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeEPUB}},
			files: []testFile{{fileType: models.FileTypeCBZ}},
			ebook: []delivered{{0, models.FileTypeEPUB}},
		},
		{
			name: "the earliest matching rule wins",
			rules: []Rule{
				{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeCBZ},
				{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeKePub},
			},
			files: []testFile{{fileType: models.FileTypeEPUB}, {fileType: models.FileTypeCBZ}},
			ebook: []delivered{{1, models.FileTypeCBZ}},
		},
		{
			name: "a rule with no matching File falls through to the next",
			rules: []Rule{
				{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeCBZ},
				{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeKePub},
			},
			files: []testFile{{fileType: models.FileTypeEPUB}},
			ebook: []delivered{{0, models.FileTypeKePub}},
		},
		{
			name:  "Files tied on the winning rule all go",
			rules: []Rule{{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB}, {SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeCBZ}},
			files: []testFile{{fileType: models.FileTypeEPUB}, {fileType: models.FileTypeCBZ}, {fileType: models.FileTypeEPUB}},
			ebook: []delivered{{0, models.FileTypeEPUB}, {2, models.FileTypeEPUB}},
		},
		{
			name:      "each category gets its own winner",
			files:     []testFile{{fileType: models.FileTypeM4B}, {fileType: models.FileTypeEPUB}},
			ebook:     []delivered{{1, models.FileTypeEPUB}},
			audiobook: []delivered{{0, models.FileTypeM4B}},
		},
		{
			name:  "an audiobook rule ahead of an ebook rule does not stop the ebook",
			rules: []Rule{{SourceFileType: models.FileTypeM4B, DeliveredFileType: models.FileTypeM4B}, {SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB}},
			files: []testFile{{fileType: models.FileTypeEPUB}, {fileType: models.FileTypeM4B}},
			ebook: []delivered{{0, models.FileTypeEPUB}}, audiobook: []delivered{{1, models.FileTypeM4B}},
		},
		{
			name:  "a File Type with no rule is not delivered",
			rules: []Rule{{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB}},
			files: []testFile{{fileType: models.FileTypeM4B}, {fileType: models.FileTypeCBZ}},
		},
		{
			name:  "supplements are never delivered",
			files: []testFile{{fileType: models.FileTypePDF, supplement: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			service := NewService(db)
			insertTestUser(t, db, 1)
			ctx := context.Background()
			if tt.rules != nil {
				_, err := service.SaveUserDefault(ctx, 1, tt.rules)
				require.NoError(t, err)
			}

			userRules, err := service.UserDefault(ctx, 1)
			require.NoError(t, err)
			assert.Equal(t, tt.rules == nil, userRules.FollowsBuiltIn)

			result := Resolve(userRules.Rules, buildBook(tt.files))
			assert.Equal(t, append([]delivered{}, tt.ebook...), deliveredOf(t, result.Ebook), "ebook")
			assert.Equal(t, append([]delivered{}, tt.audiobook...), deliveredOf(t, result.Audiobook), "audiobook")
		})
	}
}

func TestResolve_ChainReachesTheDeliveredFileType(t *testing.T) {
	t.Parallel()
	book := buildBook([]testFile{{fileType: models.FileTypeCBZ}})

	result := Resolve([]Rule{{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeKePub}}, book)

	require.Len(t, result.Ebook, 1)
	assert.Equal(t, []models.ConversionStep{{From: models.FileTypeCBZ, To: models.FileTypeKePub}}, result.Ebook[0].Chain)

	asItself := Resolve([]Rule{{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeCBZ}}, book)
	require.Len(t, asItself.Ebook, 1)
	assert.Empty(t, asItself.Ebook[0].Chain)
}

// A rule saved before a Conversion was removed can name a File Type its
// source no longer reaches. Such a rule matches nothing, so a later rule
// still gets its chance.
func TestResolve_RuleWithNoPathCountsAsUnmatched(t *testing.T) {
	t.Parallel()
	book := buildBook([]testFile{{fileType: models.FileTypePDF}, {fileType: models.FileTypeEPUB}})

	result := Resolve([]Rule{
		{SourceFileType: models.FileTypePDF, DeliveredFileType: models.FileTypeKePub},
		{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB},
	}, book)

	assert.Equal(t, []delivered{{1, models.FileTypeEPUB}}, deliveredOf(t, result.Ebook))
	assert.Equal(t, []string{models.FileTypeEPUB}, SourceFileTypes([]Rule{
		{SourceFileType: models.FileTypePDF, DeliveredFileType: models.FileTypeKePub},
		{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB},
	}))
}

func TestService_SaveUserDefault_RefusesInvalidLists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		rules []Rule
	}{
		{name: "an empty list", rules: []Rule{}},
		{name: "two rules for one source File Type", rules: []Rule{
			{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB},
			{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeKePub},
		}},
		{name: "a File Type no Conversion reaches", rules: []Rule{{SourceFileType: models.FileTypePDF, DeliveredFileType: models.FileTypeEPUB}}},
		{name: "an unknown source File Type", rules: []Rule{{SourceFileType: "cbr", DeliveredFileType: "cbr"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			service := NewService(db)
			insertTestUser(t, db, 1)
			ctx := context.Background()

			_, err := service.SaveUserDefault(ctx, 1, tt.rules)
			var rulesErr *RulesError
			require.ErrorAs(t, err, &rulesErr)

			userRules, err := service.UserDefault(ctx, 1)
			require.NoError(t, err)
			assert.True(t, userRules.FollowsBuiltIn, "a refused list leaves the default following the built-in rules")
		})
	}
}

func TestService_UserDefault_CustomListIsKeptInOrderAndResets(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1)
	insertTestUser(t, db, 2)
	ctx := context.Background()

	custom := []Rule{
		{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeEPUB},
		{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeKePub},
	}
	saved, err := service.SaveUserDefault(ctx, 1, custom)
	require.NoError(t, err)
	assert.False(t, saved.FollowsBuiltIn)
	assert.Equal(t, custom, saved.Rules)

	got, err := service.UserDefault(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, custom, got.Rules)

	other, err := service.UserDefault(ctx, 2)
	require.NoError(t, err)
	assert.True(t, other.FollowsBuiltIn, "one user's list never reaches another")
	assert.Equal(t, BuiltInRules(), other.Rules)

	_, err = service.ResetUserDefault(ctx, 1)
	require.NoError(t, err)
	reset, err := service.UserDefault(ctx, 1)
	require.NoError(t, err)
	assert.True(t, reset.FollowsBuiltIn)
	assert.Equal(t, BuiltInRules(), reset.Rules)
}

// A list that follows the built-in rules is never copied, so a change to the
// built-in rules reaches it without a data migration.
func TestService_UserDefault_FollowingListIsNeverCopied(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1)
	ctx := context.Background()

	_, err := service.UserDefault(ctx, 1)
	require.NoError(t, err)

	count, err := db.NewSelect().Model((*UserDeliveryRule)(nil)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestBuiltInRules_DeliverEveryBuiltInFileTypeAsItself(t *testing.T) {
	t.Parallel()
	rules := BuiltInRules()
	sources := make([]string, 0, len(rules))
	for _, rule := range rules {
		assert.Equal(t, rule.SourceFileType, rule.DeliveredFileType)
		sources = append(sources, rule.SourceFileType)
	}
	assert.ElementsMatch(t, models.BuiltInFileTypes, sources)
	require.NoError(t, Validate(rules))
}

// DeliveredAs and Resolve agree on every book: a book meets one of the
// matches for a File Type exactly when Resolve delivers some File of it as
// that type.
func TestDeliveredAs_AgreesWithResolve(t *testing.T) {
	t.Parallel()
	ruleLists := map[string][]Rule{
		"built-in": BuiltInRules(),
		"CBZ as EPUB first": {
			{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeEPUB},
			{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeKePub},
			{SourceFileType: models.FileTypeM4B, DeliveredFileType: models.FileTypeM4B},
		},
		"a rule with no chain first": {
			{SourceFileType: models.FileTypePDF, DeliveredFileType: models.FileTypeKePub},
			{SourceFileType: models.FileTypeKePub, DeliveredFileType: models.FileTypeKePub},
			{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB},
		},
	}
	books := [][]testFile{
		{{fileType: models.FileTypeKePub}},
		{{fileType: models.FileTypeEPUB}, {fileType: models.FileTypeCBZ}},
		{{fileType: models.FileTypeKePub}, {fileType: models.FileTypeEPUB}},
		{{fileType: models.FileTypeEPUB}, {fileType: models.FileTypeM4B}},
		{{fileType: models.FileTypePDF}, {fileType: models.FileTypeCBZ, supplement: true}},
		{{fileType: models.FileTypeMOBI}, {fileType: models.FileTypeAZW3}},
	}
	for name, rules := range ruleLists {
		for i, files := range books {
			book := buildBook(files)
			has := map[string]bool{}
			for _, f := range book.Files {
				if f.FileRole == models.FileRoleMain {
					has[f.FileType] = true
				}
			}
			delivered := map[string]bool{}
			for _, d := range Resolve(rules, book).All() {
				delivered[d.DeliveredFileType] = true
			}
			for _, fileType := range models.BuiltInFileTypes {
				meets := false
				for _, m := range DeliveredAs(rules, fileType) {
					if has[m.SourceFileType] && !slices.ContainsFunc(m.Unless, func(u string) bool { return has[u] }) {
						meets = true
					}
				}
				assert.Equal(t, delivered[fileType], meets, "%s, book %d, as %s", name, i, fileType)
			}
		}
	}
}
