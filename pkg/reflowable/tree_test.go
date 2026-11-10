package reflowable

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseReadsTagsAsTheKePubConverterDoes checks that parse reads tags as
// pkg/kepub, which wrote the KePub being read, does: a bare param was once
// read as holding what followed it.
func TestParseReadsTagsAsTheKePubConverterDoes(t *testing.T) {
	t.Parallel()

	t.Run("a param without a closing slash holds nothing", func(t *testing.T) {
		t.Parallel()
		root, err := parse([]byte(`<body><object><param name="a" value="b">after</object></body>`))
		require.NoError(t, err)

		param := root.find(func(n *node) bool { return n.name == "param" })
		require.NotNil(t, param)
		assert.Empty(t, param.children)
		require.Len(t, param.parent.children, 2)
		assert.Equal(t, "after", string(param.parent.children[1].text))
	})

	t.Run("a self-closed title doesn't swallow what follows", func(t *testing.T) {
		t.Parallel()
		root, err := parse([]byte(`<html><head><title/></head><body><p>text</p></body></html>`))
		require.NoError(t, err)

		p := root.find(func(n *node) bool { return n.name == "p" })
		require.NotNil(t, p)
		require.Len(t, p.children, 1)
		assert.Equal(t, "text", string(p.children[0].text))
	})
}
