package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// retypeM4AFiles makes every File under an .m4a path (any case) an M4B main
// File, now that Shisho parses .m4a as an M4B (ADR 0018). Before, such a
// File was a supplement typed by its extension, or a main File a plugin
// parser read. Each keeps its ID and everything attached to it. Clearing its
// stored mtime makes the next scan treat it as changed and read it with the
// built-in parser. Values a plugin parser or a sidecar set keep outranking
// the file's own metadata, as they would for any other File, and a file the
// MP4 parser can't read stays as a main File with a scan error, which the user
// can demote in Edit File.
func retypeM4AFiles(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
		UPDATE files
		SET file_type = 'm4b', file_role = 'main', file_modified_at = NULL
		WHERE lower(filepath) LIKE '%.m4a'
	`)
	return errors.Wrap(err, "failed to retype .m4a files as M4B")
}

func init() {
	up := retypeM4AFiles

	// This data migration is intentionally irreversible. Which Files were
	// supplements and which a plugin parsed isn't worth recording.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}
