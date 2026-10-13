package cache

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Every cache GET /cache lists has a limit in each settings shape and a
// limiter, under the same id, so a new cache cannot be half wired.
func TestSettingsShapesCoverEveryCache(t *testing.T) {
	t.Parallel()
	var ids []string
	for _, e := range (&Handler{}).entries() {
		ids = append(ids, e.id)
	}
	sort.Strings(ids)
	assert.Equal(t, ids, keys((&SettingsResponse{}).byID()))
	assert.Equal(t, ids, keys((&savedSettings{}).byID()))
	assert.Equal(t, ids, keys((&UpdateSettingsPayload{}).byID()))
	assert.Equal(t, ids, keys(Limiters{}.byID()))
}
