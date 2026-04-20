// Package sortspec parses, validates, builds SQL for, and resolves
// multi-level book sort specifications (e.g. "author:asc,series:asc").
package sortspec

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

// Direction is "asc" or "desc".
type Direction string

const (
	DirAsc  Direction = "asc"
	DirDesc Direction = "desc"
)

// MaxLevels is the hard cap on how many levels a spec may contain.
const MaxLevels = 10

// SortLevel is one field+direction pair in a spec.
type SortLevel struct {
	Field     string
	Direction Direction
}

// BuiltinDefault is the fallback sort applied when neither an explicit
// URL sort nor a stored per-(user, library) preference exists. It is
// the same default the React frontend's app/libraries/sortSpec.ts
// exposes as `BUILTIN_DEFAULT_SORT` — keep both in sync.
//
// Returns a fresh slice each call so callers can mutate the result
// without surprising other consumers.
//
// The books service (pkg/books/service.go ListBooksWithTotal) applies
// this default when Sort is nil and no series filter is active, so all
// surfaces that hit the books service — the /books REST endpoint, OPDS
// feeds, the eReader browser, and the React gallery — share the same
// "newest first" fallback. OPDS and the eReader browser still resolve
// to this default explicitly in their handlers so a stored preference
// takes priority over the builtin; the books service default is the
// safety net for callers that pass Sort=nil with no stored preference
// (e.g. third-party API consumers).
func BuiltinDefault() []SortLevel {
	return []SortLevel{
		{Field: FieldDateAdded, Direction: DirDesc},
	}
}

// Parse reads a serialized spec string (e.g. "author:asc,series:desc") into
// a slice of SortLevel. It rejects unknown fields, bad directions, duplicates,
// empty pairs, stray whitespace, and specs longer than MaxLevels.
func Parse(s string) ([]SortLevel, error) {
	if s == "" {
		return nil, errors.New("sort spec is empty")
	}
	// Whitespace is not allowed anywhere — this is a machine-readable URL
	// param, not human prose. Rejecting early keeps the grammar strict.
	if strings.ContainsAny(s, " \t\n\r") {
		return nil, errors.New("sort spec must not contain whitespace")
	}

	parts := strings.Split(s, ",")
	if len(parts) > MaxLevels {
		return nil, errors.Errorf("sort spec has %d levels, max is %d", len(parts), MaxLevels)
	}

	seen := make(map[string]struct{}, len(parts))
	levels := make([]SortLevel, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			return nil, errors.New("sort spec contains an empty pair")
		}

		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			return nil, errors.Errorf("sort level %q missing direction", part)
		}

		field, dir := kv[0], kv[1]
		if !IsValidField(field) {
			return nil, errors.Errorf("unknown sort field %q", field)
		}
		if dir != string(DirAsc) && dir != string(DirDesc) {
			return nil, errors.Errorf("invalid direction %q (want asc or desc)", dir)
		}
		if _, dup := seen[field]; dup {
			return nil, errors.Errorf("duplicate sort field %q", field)
		}
		seen[field] = struct{}{}

		levels = append(levels, SortLevel{Field: field, Direction: Direction(dir)})
	}

	return levels, nil
}

// Serialize renders a level slice back into the URL-param form.
// The zero/nil slice serializes to the empty string.
func Serialize(levels []SortLevel) string {
	if len(levels) == 0 {
		return ""
	}
	parts := make([]string, len(levels))
	for i, l := range levels {
		parts[i] = fmt.Sprintf("%s:%s", l.Field, l.Direction)
	}
	return strings.Join(parts, ",")
}
