package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const renameReaderSettingsMigrationName = "20261009000000"

// The font size, theme, and flow settings cover every Reflowable format, not
// only EPUB. Renaming the columns must keep what each user saved.
func TestRenameReaderSettingsToReflowable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, renameReaderSettingsMigrationName)

	for _, query := range []string{
		`INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'reader', 'x', 1)`,
		`INSERT INTO user_settings (user_id, viewer_epub_font_size, viewer_epub_theme, viewer_epub_flow)
			VALUES (1, 140, 'sepia', 'scrolled')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	type saved struct {
		FontSize int    `bun:"font_size"`
		Theme    string `bun:"theme"`
		Flow     string `bun:"flow"`
	}
	read := func(prefix string) saved {
		var s saved
		require.NoError(t, db.NewRaw(
			"SELECT "+prefix+"_font_size AS font_size, "+prefix+"_theme AS theme, "+prefix+"_flow AS flow FROM user_settings WHERE user_id = 1",
		).Scan(ctx, &s))
		return s
	}
	want := saved{FontSize: 140, Theme: "sepia", Flow: "scrolled"}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, read("viewer_reflowable"))

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, read("viewer_epub"))

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
