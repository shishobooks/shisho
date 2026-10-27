package tags

import (
	"fmt"
	"maps"
	"strings"
	"unicode/utf8"

	"github.com/shishobooks/shisho/pkg/sortspec"
)

// A tag is in a namespace when its name starts with the namespace and a
// colon, compared case-insensitively for ASCII letters only (as SQLite's LIKE
// compares), and something other than spaces follows. "Circle: Foo" and
// "Circle:Foo" are both the tag "Foo" in "Circle"; "Circle:" alone is a plain
// tag. splitTagNamespace in app/utils/tagNamespaces.ts applies the same rule
// when the frontend splits tag names for display, so change both together.

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// namespaceMatch returns the SQL condition, over the tag alias t, that the
// tag is in namespace.
func namespaceMatch(namespace string) (string, []any) {
	return `(t.name LIKE ? ESCAPE '\' AND ltrim(substr(t.name, ?), ' ') <> '')`,
		[]any{likeEscaper.Replace(namespace) + ":%", valueStart(namespace)}
}

// valueStart is the 1-based character position where a namespaced tag's
// name begins, just past the colon. SQLite's substr counts characters.
func valueStart(namespace string) int {
	return utf8.RuneCountInString(namespace) + 2
}

// namespaceSortFields sorts tags in one namespace by their names without the
// prefix, so "Circle:Bar" sorts with "Circle: Baz" rather than after every
// "Circle: " tag.
func namespaceSortFields(namespace string) sortspec.ResourceFields {
	fields := maps.Clone(tagSortFields)
	fields[sortspec.FieldName] = fmt.Sprintf("ltrim(substr(t.name, %d), ' ')", valueStart(namespace))
	return fields
}
