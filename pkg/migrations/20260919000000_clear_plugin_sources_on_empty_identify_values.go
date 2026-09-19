package migrations

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// clearPluginSourcesOnEmptyIdentifyValues heals rows left behind by Identify
// clears made before ADR 0006. Those clears nulled the value but kept the
// plugin source, and a plugin-sourced empty slot outranks embedded metadata,
// so an ordinary Scan could never repopulate the field.
//
// Only plugin sources are cleared. The Edit form stores a cleared value as
// NULL plus 'manual' on purpose, as a protected empty slot, and the scanner
// never writes an empty value, so a plugin source on a NULL value can only
// come from one of those Identify clears. Language, Abridged, and Name are
// not listed because their clears always nulled the source.
func clearPluginSourcesOnEmptyIdentifyValues(ctx context.Context, db *bun.DB) error {
	targets := []struct {
		table, value, source string
	}{
		{"books", "subtitle", "subtitle_source"},
		{"books", "description", "description_source"},
		{"files", "publisher_id", "publisher_source"},
		{"files", "url", "url_source"},
		{"files", "release_date", "release_date_source"},
	}
	for _, target := range targets {
		// Identifiers come from the fixed list above, never from input.
		query := fmt.Sprintf(`
			UPDATE %[1]s
			SET %[3]s = NULL
			WHERE %[2]s IS NULL
				AND (%[3]s = 'plugin' OR %[3]s LIKE 'plugin:%%')
		`, target.table, target.value, target.source)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return errors.Wrapf(err, "failed to clear %s.%s", target.table, target.source)
		}
	}
	return nil
}

func init() {
	up := clearPluginSourcesOnEmptyIdentifyValues

	// This data migration is intentionally irreversible. The cleared sources
	// described no value, and which plugin they named cannot be recovered.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}
