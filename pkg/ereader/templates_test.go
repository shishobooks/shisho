package ereader

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPaginationWithParams_ClampsCurrentPage pins the bounds-check on
// currentPage: a URL-edited ?page=9999 on a 3-page listing should
// render "Page 3 of 3" with Prev enabled and Next disabled, rather
// than advertising Prev/Next targets that don't exist.
func TestPaginationWithParams_ClampsCurrentPage(t *testing.T) {
	t.Parallel()

	got := paginationWithParams(9999, 3, "/base", "", "")
	assert.Contains(t, got, "Page 3 of 3", "currentPage clamped up to totalPages")
	assert.Contains(t, got, `href="/base?page=2"`, "Prev points at clamped-page - 1")
	assert.NotContains(t, got, "page=9998", "no stale overshoot URLs")
	assert.NotContains(t, got, `href="/base?page=4"`, "Next disabled at the last page")

	got = paginationWithParams(0, 3, "/base", "", "")
	assert.Contains(t, got, "Page 1 of 3", "currentPage clamped up to 1")
	assert.Contains(t, got, `href="/base?page=2"`, "Next enabled from page 1")

	got = paginationWithParams(-5, 3, "/base", "", "")
	assert.Contains(t, got, "Page 1 of 3", "negative currentPage clamped up to 1")
}

// TestPaginationWithParams_ThreadsExtraParams confirms search queries
// (and any other caller-supplied params) ride along on Prev/Next
// links. Empty values are dropped so a blank param doesn't leave
// "&q=" in the URL.
func TestPaginationWithParams_ThreadsExtraParams(t *testing.T) {
	t.Parallel()

	got := paginationWithParams(2, 3, "/s", "epub", "on", [2]string{"q", "brandon sanderson"})
	assert.Contains(t, got, "page=1")
	assert.Contains(t, got, "page=3")
	assert.Contains(t, got, "q=brandon+sanderson", "query is URL-encoded")
	assert.Contains(t, got, "types=epub")
	assert.Contains(t, got, "covers=on")

	got = paginationWithParams(2, 3, "/s", "", "", [2]string{"q", ""})
	assert.NotContains(t, got, "q=", "empty extra params are skipped")
}

// TestFilterBar_PreservesExtraParams is the behavioral pin for the M2
// fix: on the paginated search page, clicking a type filter must not
// drop the ?q= query. Before this fix, filterLink only preserved
// types/covers; now it threads through arbitrary extras via the same
// mechanism as paginationWithParams.
func TestFilterBar_PreservesExtraParams(t *testing.T) {
	t.Parallel()

	got := filterBar("/s", "", "", [2]string{"q", "sanderson"})

	// Every non-current filter link must carry ?q=sanderson...
	for _, label := range []string{"EPUB", "CBZ", "M4B", "PDF"} {
		idx := strings.Index(got, ">"+label+"<")
		if !assert.NotEqual(t, -1, idx, "label %q rendered", label) {
			continue
		}
		// Find the href of the link containing this label
		anchorStart := strings.LastIndex(got[:idx], `href="`)
		if !assert.NotEqual(t, -1, anchorStart, "label %q has an href", label) {
			continue
		}
		anchorEnd := strings.Index(got[anchorStart+6:], `"`)
		href := got[anchorStart+6 : anchorStart+6+anchorEnd]
		assert.Contains(t, href, "q=sanderson", "filter link for %q preserves the search query", label)
	}

	// And the covers=on link preserves it too.
	assert.Contains(t, got, "q=sanderson")
}

// TestFilterLink_UsesURLValuesEncoding pins the M3 fix: filterLink now
// uses url.Values.Encode() like paginationWithParams, so values with
// special characters (hypothetical future filter option) are
// percent-encoded instead of naively concatenated.
func TestFilterLink_UsesURLValuesEncoding(t *testing.T) {
	t.Parallel()

	// With no filters set, the link should have no query string at all
	// (previously: "?" was emitted bare when both were empty).
	got := filterLink("/s", "types", "all", "", "", "All")
	assert.NotContains(t, got, "?", `"all" link with no other params is clean`)

	// Preserves a search query through the encoding path.
	got = filterLink("/s", "types", "epub", "", "", "EPUB", [2]string{"q", "a&b"})
	assert.Contains(t, got, "q=a%26b", "ampersand in q is percent-encoded")
	assert.Contains(t, got, "types=epub")
}
