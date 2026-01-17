package epub

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNavDocument(t *testing.T) {
	navXML := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body>
<nav epub:type="toc">
  <ol>
    <li><a href="chapter1.xhtml">Chapter 1</a></li>
    <li>
      <a href="part2.xhtml">Part 2</a>
      <ol>
        <li><a href="chapter2.xhtml">Chapter 2</a></li>
        <li><a href="chapter3.xhtml#section1">Chapter 3</a></li>
      </ol>
    </li>
  </ol>
</nav>
</body>
</html>`

	chapters, err := parseNavDocument(strings.NewReader(navXML))
	require.NoError(t, err)
	require.Len(t, chapters, 2)

	// Chapter 1 - flat
	assert.Equal(t, "Chapter 1", chapters[0].Title)
	require.NotNil(t, chapters[0].Href)
	assert.Equal(t, "chapter1.xhtml", *chapters[0].Href)
	assert.Empty(t, chapters[0].Children)

	// Part 2 - nested
	assert.Equal(t, "Part 2", chapters[1].Title)
	require.NotNil(t, chapters[1].Href)
	assert.Equal(t, "part2.xhtml", *chapters[1].Href)
	require.Len(t, chapters[1].Children, 2)

	// Nested children
	assert.Equal(t, "Chapter 2", chapters[1].Children[0].Title)
	assert.Equal(t, "chapter2.xhtml", *chapters[1].Children[0].Href)
	assert.Equal(t, "Chapter 3", chapters[1].Children[1].Title)
	assert.Equal(t, "chapter3.xhtml#section1", *chapters[1].Children[1].Href)
}

func TestParseNavDocument_SpanWithoutLink(t *testing.T) {
	navXML := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body>
<nav epub:type="toc">
  <ol>
    <li><span>Part 1 (no link)</span>
      <ol>
        <li><a href="chapter1.xhtml">Chapter 1</a></li>
      </ol>
    </li>
  </ol>
</nav>
</body>
</html>`

	chapters, err := parseNavDocument(strings.NewReader(navXML))
	require.NoError(t, err)
	require.Len(t, chapters, 1)

	// Part 1 - span without href
	assert.Equal(t, "Part 1 (no link)", chapters[0].Title)
	assert.Nil(t, chapters[0].Href)
	require.Len(t, chapters[0].Children, 1)
}

func TestParseNCX(t *testing.T) {
	ncxXML := `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<navMap>
  <navPoint id="ch1" playOrder="1">
    <navLabel><text>Chapter 1</text></navLabel>
    <content src="chapter1.xhtml"/>
    <navPoint id="ch1-1" playOrder="2">
      <navLabel><text>Section 1.1</text></navLabel>
      <content src="chapter1.xhtml#s1"/>
    </navPoint>
  </navPoint>
  <navPoint id="ch2" playOrder="3">
    <navLabel><text>Chapter 2</text></navLabel>
    <content src="chapter2.xhtml"/>
  </navPoint>
</navMap>
</ncx>`

	chapters, err := parseNCX(strings.NewReader(ncxXML))
	require.NoError(t, err)
	require.Len(t, chapters, 2)

	// Chapter 1 with nested section
	assert.Equal(t, "Chapter 1", chapters[0].Title)
	require.NotNil(t, chapters[0].Href)
	assert.Equal(t, "chapter1.xhtml", *chapters[0].Href)
	require.Len(t, chapters[0].Children, 1)
	assert.Equal(t, "Section 1.1", chapters[0].Children[0].Title)
	assert.Equal(t, "chapter1.xhtml#s1", *chapters[0].Children[0].Href)

	// Chapter 2 - flat
	assert.Equal(t, "Chapter 2", chapters[1].Title)
	assert.Equal(t, "chapter2.xhtml", *chapters[1].Href)
	assert.Empty(t, chapters[1].Children)
}
