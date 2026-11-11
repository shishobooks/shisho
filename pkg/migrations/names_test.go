package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Bun tracks applied migrations by the timestamp part of the file name, so
// two migrations sharing one would let a database that ran one skip the
// other. Give each new migration a timestamp no other migration uses.
func TestMigrationNamesAreUnique(t *testing.T) {
	t.Parallel()
	seen := map[string]string{}
	for _, migration := range Migrations.Sorted() {
		if other, ok := seen[migration.Name]; ok {
			assert.Failf(t, "duplicate migration name", "%s is used by both %s and %s", migration.Name, other, migration.Comment)
			continue
		}
		seen[migration.Name] = migration.Comment
	}
}
