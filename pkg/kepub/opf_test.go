package kepub

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformOPF(t *testing.T) {
	t.Parallel()
	t.Run("adds cover-image property when meta cover exists", func(t *testing.T) {
		input := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata>
    <meta name="cover" content="cover-id"/>
  </metadata>
  <manifest>
    <item id="cover-id" href="cover.jpg" media-type="image/jpeg"/>
  </manifest>
</package>`
		var buf bytes.Buffer

		err := TransformOPF(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, `properties="cover-image"`)
	})

	t.Run("preserves existing cover-image property", func(t *testing.T) {
		input := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata></metadata>
  <manifest>
    <item id="cover" href="cover.jpg" media-type="image/jpeg" properties="cover-image"/>
  </manifest>
</package>`
		var buf bytes.Buffer

		err := TransformOPF(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		// Should still have exactly one cover-image property
		count := strings.Count(output, "cover-image")
		assert.Equal(t, 1, count)
	})

	t.Run("returns unchanged when no cover defined", func(t *testing.T) {
		input := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata></metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
</package>`
		var buf bytes.Buffer

		err := TransformOPF(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.NotContains(t, output, "cover-image")
	})

	t.Run("handles meta tag with content before name", func(t *testing.T) {
		input := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata>
    <meta content="my-cover" name="cover"/>
  </metadata>
  <manifest>
    <item id="my-cover" href="cover.png" media-type="image/png"/>
  </manifest>
</package>`
		var buf bytes.Buffer

		err := TransformOPF(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, `properties="cover-image"`)
	})

	t.Run("handles EPUB3 style properties", func(t *testing.T) {
		input := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata></metadata>
  <manifest>
    <item id="cover" properties="cover-image" href="cover.jpg" media-type="image/jpeg"/>
  </manifest>
</package>`
		var buf bytes.Buffer

		err := TransformOPF(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		// Should preserve the existing property
		assert.Contains(t, output, "cover-image")
		count := strings.Count(output, "cover-image")
		assert.Equal(t, 1, count)
	})

	t.Run("adds to existing properties attribute", func(t *testing.T) {
		input := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata>
    <meta name="cover" content="cover-id"/>
  </metadata>
  <manifest>
    <item id="cover-id" href="cover.svg" media-type="image/svg+xml" properties="svg"/>
  </manifest>
</package>`
		var buf bytes.Buffer

		err := TransformOPF(strings.NewReader(input), &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "svg")
		assert.Contains(t, output, "cover-image")
	})
}

func TestFindCoverID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard meta tag",
			input:    `<meta name="cover" content="cover-image"/>`,
			expected: "cover-image",
		},
		{
			name:     "meta with content before name",
			input:    `<meta content="my-cover" name="cover"/>`,
			expected: "my-cover",
		},
		{
			name:     "EPUB3 properties style",
			input:    `<item properties="cover-image" id="cover-img"/>`,
			expected: "cover-img",
		},
		{
			name:     "EPUB3 with id before properties",
			input:    `<item id="the-cover" properties="cover-image"/>`,
			expected: "the-cover",
		},
		{
			name:     "no cover defined",
			input:    `<metadata></metadata>`,
			expected: "",
		},
		{
			name:     "single quotes",
			input:    `<meta name='cover' content='cover-id'/>`,
			expected: "cover-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findCoverID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddCoverImageProperty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		content  string
		coverID  string
		contains string
	}{
		{
			name:     "adds properties to item without any",
			content:  `<item id="cover" href="cover.jpg" media-type="image/jpeg"/>`,
			coverID:  "cover",
			contains: `properties="cover-image"`,
		},
		{
			name:     "adds to existing properties",
			content:  `<item id="cover" href="cover.svg" media-type="image/svg+xml" properties="svg"/>`,
			coverID:  "cover",
			contains: "cover-image",
		},
		{
			name:     "does not modify if already has cover-image",
			content:  `<item id="cover" properties="cover-image" href="cover.jpg"/>`,
			coverID:  "cover",
			contains: "cover-image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addCoverImageProperty(tt.content, tt.coverID)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestTransformOPFBytes(t *testing.T) {
	t.Parallel()
	t.Run("transforms OPF correctly", func(t *testing.T) {
		input := []byte(`<?xml version="1.0"?>
<package>
  <metadata>
    <meta name="cover" content="cover"/>
  </metadata>
  <manifest>
    <item id="cover" href="cover.jpg" media-type="image/jpeg"/>
  </manifest>
</package>`)

		output, err := TransformOPFBytes(input)
		require.NoError(t, err)

		assert.Contains(t, string(output), `properties="cover-image"`)
	})
}
