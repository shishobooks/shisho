package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			// How far through its chapter a position is, which a Kobo reports
			// with its span and which places the span when the span can't be
			// found.
			`ALTER TABLE reading_positions ADD COLUMN chapter_percentage REAL CHECK (chapter_percentage BETWEEN 0 AND 1)`,
			// The reading minutes a Kobo last reported for the book an entry
			// holds, and when. An entry recorded before Kobo reading state
			// synced has none, so its first report only sets the baseline:
			// the Kobo counted minutes Shisho never saw.
			`ALTER TABLE kobo_sync_entries ADD COLUMN reading_minutes INTEGER`,
			`ALTER TABLE kobo_sync_entries ADD COLUMN reading_minutes_at TIMESTAMP`,
			// When the Kobo last changed the status it reports, so a status it
			// keeps reporting unchanged doesn't undo one changed in Shisho
			// since, such as a pause.
			`ALTER TABLE kobo_sync_entries ADD COLUMN reading_status_at TIMESTAMP`,
			// What the Kobo last heard of the entry's reading state, so a sync
			// sends it again only once another reader changes it, and what a
			// sync sent it, in which sync point and on which page, which the
			// Kobo has heard once it syncs from that point.
			`ALTER TABLE kobo_sync_entries ADD COLUMN reading_state_version TEXT`,
			`ALTER TABLE kobo_sync_entries ADD COLUMN reading_state_sent_version TEXT`,
			`ALTER TABLE kobo_sync_entries ADD COLUMN reading_state_sent_in TEXT`,
			`ALTER TABLE kobo_sync_entries ADD COLUMN reading_state_sent_cursor INTEGER`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`ALTER TABLE kobo_sync_entries DROP COLUMN reading_state_sent_cursor`,
			`ALTER TABLE kobo_sync_entries DROP COLUMN reading_state_sent_in`,
			`ALTER TABLE kobo_sync_entries DROP COLUMN reading_state_sent_version`,
			`ALTER TABLE kobo_sync_entries DROP COLUMN reading_state_version`,
			`ALTER TABLE kobo_sync_entries DROP COLUMN reading_status_at`,
			`ALTER TABLE kobo_sync_entries DROP COLUMN reading_minutes_at`,
			`ALTER TABLE kobo_sync_entries DROP COLUMN reading_minutes`,
			`ALTER TABLE reading_positions DROP COLUMN chapter_percentage`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
