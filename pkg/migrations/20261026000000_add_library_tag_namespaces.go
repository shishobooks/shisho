package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Tag namespaces are a library's list of tag name prefixes ("Circle" groups
// "Circle: Foo"), held as a JSON array of strings. Existing libraries get an
// empty list, which leaves tags shown as they were.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `ALTER TABLE libraries ADD COLUMN tag_namespaces TEXT NOT NULL DEFAULT '[]'`)
		return errors.Wrap(err, "failed to add libraries.tag_namespaces")
	}
	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `ALTER TABLE libraries DROP COLUMN tag_namespaces`)
		return errors.Wrap(err, "failed to drop libraries.tag_namespaces")
	}
	Migrations.MustRegister(up, down)
}
