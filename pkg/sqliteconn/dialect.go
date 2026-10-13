package sqliteconn

import (
	"strings"

	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/schema"
)

// NewDialect returns the bun dialect Shisho uses for SQLite. It drops NUL
// bytes from string values instead of failing the query.
//
// bun's own dialect rejects any string containing a NUL byte. Shisho stores
// text read from book files, and M4B tags, PDF Info strings, and plugin
// results can carry stray NULs that are never part of the text. Rejecting
// them would fail a whole scan or save over one unprintable byte.
func NewDialect() schema.Dialect {
	return &dialect{Dialect: sqlitedialect.New()}
}

type dialect struct {
	*sqlitedialect.Dialect
}

func (d *dialect) AppendString(b []byte, s string) []byte {
	return d.Dialect.AppendString(b, strings.ReplaceAll(s, "\x00", ""))
}
