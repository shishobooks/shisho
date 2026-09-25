package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// deleteNegativePDFChapters removes chapters stored with a negative
// start_page on PDF files. PDF outline items whose destination has no page
// (e.g. /Dest [null 0 0 1]) came back from PDFium with page index -1 and
// were saved as chapters with start_page -1. The scanner now skips them, but
// an ordinary Scan cannot repair rows it already wrote: it skips files whose
// size and mtime are unchanged, never replaces chapters with an empty list,
// and leaves higher priority chapter sources alone. The chapter edit API also
// rejects a negative start_page now, so leaving these rows in place would
// block saving the File's chapters.
//
// A deleted chapter's descendants are deleted with it, matching the PDF
// download, which drops the whole subtree of a chapter with an invalid page.
// They are collected explicitly instead of relying on ON DELETE CASCADE,
// which only fires on a connection with PRAGMA foreign_keys enabled.
func deleteNegativePDFChapters(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
		WITH RECURSIVE doomed(id) AS (
			SELECT c.id
			FROM chapters c
			JOIN files f ON f.id = c.file_id
			WHERE f.file_type = 'pdf' AND c.start_page < 0
			UNION
			SELECT c.id
			FROM chapters c
			JOIN doomed d ON c.parent_id = d.id
		)
		DELETE FROM chapters WHERE id IN (SELECT id FROM doomed)
	`)
	return errors.Wrap(err, "failed to delete negative PDF chapters")
}

func init() {
	up := deleteNegativePDFChapters

	// This data migration is intentionally irreversible. The deleted chapters
	// pointed at no page, so there is nothing meaningful to restore.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}
