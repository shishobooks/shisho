package plugins

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plugin reference lists the extensions Shisho parses itself, which a
// file parser is not used for. That set can grow in any release, so the list
// must name exactly what the code does.
func TestManifestReferenceListsBuiltInExtensions(t *testing.T) {
	t.Parallel()

	doc, err := os.ReadFile("../../website/docs/plugins/manifest-hooks-reference.md")
	require.NoError(t, err)
	const prefix = "Shisho parses these extensions itself: "
	var line string
	for _, l := range strings.Split(string(doc), "\n") {
		if strings.HasPrefix(l, prefix) {
			line = l
			break
		}
	}
	require.NotEmpty(t, line, "the reference has a line starting %q", prefix)

	sentence, _, _ := strings.Cut(strings.TrimPrefix(line, prefix), ".")
	var listed []string
	for _, m := range regexp.MustCompile("`([a-z0-9]+)`").FindAllStringSubmatch(sentence, -1) {
		listed = append(listed, m[1])
	}
	slices.Sort(listed)
	assert.Equal(t, models.BuiltInFileExtensions(), listed)
}
