package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The viewer role loses Libraries Read and keeps the rest of its permissions;
// editor and admin keep theirs. Rolling back restores the one row.
func TestDropViewerLibrariesRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261111000001")

	permissions := func(role string) []string {
		t.Helper()
		var rows []string
		require.NoError(t, db.NewRaw(`
			SELECT p.resource || ':' || p.operation
			FROM permissions p JOIN roles r ON r.id = p.role_id
			WHERE r.name = ?
			ORDER BY p.resource, p.operation
		`, role).Scan(ctx, &rows))
		return rows
	}

	viewerBefore := permissions("viewer")
	require.Contains(t, viewerBefore, "libraries:read")
	editorBefore := permissions("editor")
	adminBefore := permissions("admin")
	require.Contains(t, editorBefore, "libraries:read")
	require.Contains(t, adminBefore, "libraries:read")

	viewerAfter := make([]string, 0, len(viewerBefore))
	for _, p := range viewerBefore {
		if p != "libraries:read" {
			viewerAfter = append(viewerAfter, p)
		}
	}

	for range 2 {
		_, err := migrator.Migrate(ctx)
		require.NoError(t, err)
		assert.Equal(t, viewerAfter, permissions("viewer"))
		assert.Equal(t, editorBefore, permissions("editor"))
		assert.Equal(t, adminBefore, permissions("admin"))

		_, err = migrator.Rollback(ctx)
		require.NoError(t, err)
		assert.Equal(t, viewerBefore, permissions("viewer"))
		assert.Equal(t, editorBefore, permissions("editor"))
		assert.Equal(t, adminBefore, permissions("admin"))
	}
}

// An admin who already took Libraries Read away from the viewer role, or
// granted it back after the upgrade, must not trip the migration either way.
func TestDropViewerLibrariesRead_AlreadyRemoved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261111000001")

	_, err := db.ExecContext(ctx, `
		DELETE FROM permissions
		WHERE resource = 'libraries' AND operation = 'read'
			AND role_id = (SELECT id FROM roles WHERE name = 'viewer')
	`)
	require.NoError(t, err)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO permissions (role_id, resource, operation)
		SELECT id, 'libraries', 'read' FROM roles WHERE name = 'viewer'
	`)
	require.NoError(t, err)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	count, err := db.NewSelect().
		TableExpr("permissions p").
		Join("JOIN roles r ON r.id = p.role_id").
		Where("r.name = 'viewer' AND p.resource = 'libraries' AND p.operation = 'read'").
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}
