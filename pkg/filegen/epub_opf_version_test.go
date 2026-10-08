package filegen

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the parts of OPF generation that depend on the package
// version: how the cover is found and added, and how creator and identifier
// refinements (role, file-as, scheme) are written.

// versionedEPUB describes a source EPUB whose OPF lives at opfPath. files are
// extra entries keyed by their full zip path.
type versionedEPUB struct {
	opfPath string
	opf     string
	files   map[string]string
}

func writeVersionedEPUB(t *testing.T, path string, src versionedEPUB) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	mw, err := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	require.NoError(t, err)
	_, err = mw.Write([]byte("application/epub+zip"))
	require.NoError(t, err)

	entries := []struct{ name, content string }{
		{"META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="` + src.opfPath + `" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`},
		{src.opfPath, src.opf},
	}
	for name, content := range src.files {
		entries = append(entries, struct{ name, content string }{name, content})
	}
	for _, e := range entries {
		fw, err := w.Create(e.name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(e.content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
}

// generateVersioned writes src, generates from it, and returns the output
// path. When cover is non-nil it is stored as the file's Shisho cover.
func generateVersioned(t *testing.T, src versionedEPUB, book *models.Book, file *models.File, cover []byte, coverMime string) string {
	t.Helper()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.epub")
	writeVersionedEPUB(t, srcPath, src)

	if file == nil {
		file = &models.File{FileType: models.FileTypeEPUB}
	}
	file.Filepath = srcPath
	if cover != nil {
		ext := ".jpg"
		if coverMime == "image/png" {
			ext = ".png"
		}
		coverFilename := "source.epub.cover" + ext
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, coverFilename), cover, 0600))
		file.CoverImageFilename = &coverFilename
		file.CoverMimeType = &coverMime
	}

	destPath := filepath.Join(tmpDir, "dest.epub")
	require.NoError(t, (&EPUBGenerator{}).Generate(context.Background(), srcPath, destPath, book, file))
	return destPath
}

func zipEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()

	r, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer r.Close()

	out := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		out[f.Name] = data
	}
	return out
}

// opfElement is a namespace-resolved element from the generated OPF, read
// with a plain decoder so the assertions do not depend on opfPackage.
type opfElement struct {
	Name  xml.Name
	Attrs []xml.Attr
	Text  string
}

func (e opfElement) attr(space, local string) (string, bool) {
	for _, a := range e.Attrs {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// opfElements returns every element under <metadata> and <manifest>, keyed
// by the parent's local name.
func opfElements(t *testing.T, raw []byte) map[string][]opfElement {
	t.Helper()

	out := map[string][]opfElement{}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var stack []string
	var current *opfElement
	var currentParent string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch tok := tok.(type) {
		case xml.StartElement:
			if len(stack) == 2 && (stack[1] == "metadata" || stack[1] == "manifest") {
				var attrs []xml.Attr
				for _, a := range tok.Attr {
					if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
						continue
					}
					attrs = append(attrs, a)
				}
				current = &opfElement{Name: tok.Name, Attrs: attrs}
				currentParent = stack[1]
			}
			stack = append(stack, tok.Name.Local)
		case xml.CharData:
			if current != nil {
				current.Text += string(tok)
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
			if current != nil && len(stack) == 2 {
				current.Text = strings.TrimSpace(current.Text)
				out[currentParent] = append(out[currentParent], *current)
				current = nil
			}
		}
	}
	return out
}

// refinesFor returns property -> meta element for the metas refining id.
func refinesFor(elements []opfElement, id string) map[string]opfElement {
	out := map[string]opfElement{}
	for _, e := range elements {
		if e.Name.Local != "meta" {
			continue
		}
		if refines, _ := e.attr("", "refines"); refines == "#"+id {
			property, _ := e.attr("", "property")
			out[property] = e
		}
	}
	return out
}

func findByText(elements []opfElement, local, text string) (opfElement, bool) {
	for _, e := range elements {
		if e.Name.Local == local && e.Text == text {
			return e, true
		}
	}
	return opfElement{}, false
}

// assertValidXML checks the generated OPF with xmllint, which reports
// namespace errors (unbound prefixes, attributes repeated under one
// namespace) that encoding/xml tolerates.
func assertValidXML(t *testing.T, raw []byte) {
	t.Helper()

	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Log("xmllint not installed; skipping XML validation")
		return
	}
	cmd := exec.CommandContext(t.Context(), "xmllint", "--noout", "-")
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "xmllint rejected the generated OPF:\n%s\n%s", out, raw)
}

// assertNoDuplicateAttrs checks each start tag's raw attribute names, since
// encoding/xml tolerates duplicate attributes on decode.
func assertNoDuplicateAttrs(t *testing.T, raw []byte) {
	t.Helper()

	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		seen := map[string]bool{}
		for _, attr := range start.Attr {
			key := attr.Name.Space + ":" + attr.Name.Local
			assert.False(t, seen[key], "<%s> repeats attribute %q:\n%s", start.Name.Local, key, raw)
			seen[key] = true
		}
	}
}

// assertEPUBCheck runs epubcheck on the generated file when it is installed.
func assertEPUBCheck(t *testing.T, path string) {
	t.Helper()

	if _, err := exec.LookPath("epubcheck"); err != nil {
		return
	}
	out, err := exec.CommandContext(t.Context(), "epubcheck", path).CombinedOutput()
	// The fixtures are minimal packages, so only check that epubcheck does
	// not flag the attributes this generator writes.
	if err != nil {
		for _, line := range strings.Split(string(out), "\n") {
			for _, needle := range []string{"opf:role", "opf:file-as", "opf:scheme", "\"role\"", "\"file-as\"", "\"scheme\"", "cover-image"} {
				assert.NotContains(t, line, needle, "epubcheck output:\n%s", out)
			}
		}
	}
}

func coverBook() *models.Book {
	return &models.Book{
		Title:   "Cover Book",
		Authors: []*models.Author{{SortOrder: 0, Person: &models.Person{Name: "Author", SortName: "Author"}}},
	}
}

const coverChapter = `<html xmlns="http://www.w3.org/1999/xhtml"><head><title>One</title></head><body><p>One</p></body></html>`

func TestEPUBGenerator_CoverSwap(t *testing.T) {
	t.Parallel()

	newCover := []byte("shisho cover bytes")

	t.Run("EPUB 3 cover marked only with cover-image", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "OEBPS/content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="img" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{
				"OEBPS/images/cover.jpg": "original cover",
				"OEBPS/chapter1.xhtml":   coverChapter,
			},
		}, coverBook(), nil, newCover, "image/png")

		entries := zipEntries(t, dest)
		assert.Equal(t, newCover, entries["OEBPS/images/cover.jpg"], "the cover-image item's file is replaced")

		els := opfElements(t, entries["OEBPS/content.opf"])
		require.Len(t, els["manifest"], 2, "no second cover item is added")
		mediaType, _ := els["manifest"][0].attr("", "media-type")
		assert.Equal(t, "image/png", mediaType)
		props, _ := els["manifest"][0].attr("", "properties")
		assert.Equal(t, "cover-image", props)

		metadata, err := epub.Parse(dest)
		require.NoError(t, err)
		assert.Equal(t, newCover, metadata.CoverData)
	})

	t.Run("meta name=cover wins over cover-image", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
    <meta name="cover" content="legacy"/>
  </metadata>
  <manifest>
    <item id="legacy" href="legacy.jpg" media-type="image/jpeg"/>
    <item id="modern" href="modern.jpg" media-type="image/jpeg" properties="cover-image"/>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{
				"legacy.jpg":     "legacy cover",
				"modern.jpg":     "modern cover",
				"chapter1.xhtml": coverChapter,
			},
		}, coverBook(), nil, newCover, "image/jpeg")

		entries := zipEntries(t, dest)
		assert.Equal(t, newCover, entries["legacy.jpg"])
		assert.Equal(t, []byte("modern cover"), entries["modern.jpg"])
	})

	t.Run("EPUB 2 cover through meta name=cover", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="bookid">abc</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
    <meta name="cover" content="cov"/>
  </metadata>
  <manifest>
    <item id="cov" href="cover.jpeg" media-type="image/jpeg"/>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{
				"cover.jpeg":     "original cover",
				"chapter1.xhtml": coverChapter,
			},
		}, coverBook(), nil, newCover, "image/jpeg")

		entries := zipEntries(t, dest)
		assert.Equal(t, newCover, entries["cover.jpeg"])
	})

	t.Run("EPUB 3 without a cover gets a cover-image item", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "OEBPS/content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{"OEBPS/chapter1.xhtml": coverChapter},
		}, coverBook(), nil, newCover, "image/png")

		entries := zipEntries(t, dest)
		raw := entries["OEBPS/content.opf"]
		els := opfElements(t, raw)
		require.Len(t, els["manifest"], 2)
		added := els["manifest"][1]
		props, _ := added.attr("", "properties")
		assert.Equal(t, "cover-image", props)
		mediaType, _ := added.attr("", "media-type")
		assert.Equal(t, "image/png", mediaType)
		href, _ := added.attr("", "href")
		assert.Equal(t, newCover, entries["OEBPS/"+href], "the cover file is added next to the OPF")
		for _, e := range els["metadata"] {
			name, _ := e.attr("", "name")
			assert.NotEqual(t, "cover", name, "EPUB 3 marks the cover with properties, not meta name=cover")
		}
		assertValidXML(t, raw)

		metadata, err := epub.Parse(dest)
		require.NoError(t, err)
		assert.Equal(t, newCover, metadata.CoverData)
	})

	t.Run("EPUB 2 without a cover gets meta name=cover", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="bookid">abc</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{"chapter1.xhtml": coverChapter},
		}, coverBook(), nil, newCover, "image/jpeg")

		entries := zipEntries(t, dest)
		raw := entries["content.opf"]
		els := opfElements(t, raw)
		require.Len(t, els["manifest"], 2)
		added := els["manifest"][1]
		_, hasProps := added.attr("", "properties")
		assert.False(t, hasProps, "properties is not an EPUB 2 manifest attribute")
		addedID, _ := added.attr("", "id")
		href, _ := added.attr("", "href")
		assert.Equal(t, newCover, entries[href])

		var coverMeta string
		for _, e := range els["metadata"] {
			if name, _ := e.attr("", "name"); name == "cover" {
				coverMeta, _ = e.attr("", "content")
			}
		}
		assert.Equal(t, addedID, coverMeta)
		assertValidXML(t, raw)

		metadata, err := epub.Parse(dest)
		require.NoError(t, err)
		assert.Equal(t, newCover, metadata.CoverData)
	})

	t.Run("meta name=cover pointing at a non-image item is repointed", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="bookid">abc</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
    <meta name="cover" content="cover-image"/>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="cover-image" href="cover.jpg" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{"chapter1.xhtml": coverChapter, "cover.jpg": "not a cover"},
		}, coverBook(), nil, newCover, "image/jpeg")

		entries := zipEntries(t, dest)
		assert.Equal(t, []byte("not a cover"), entries["cover.jpg"], "the XHTML item's file is not overwritten")
		els := opfElements(t, entries["content.opf"])
		require.Len(t, els["manifest"], 3)
		addedID, _ := els["manifest"][2].attr("", "id")
		assert.NotEqual(t, "cover-image", addedID, "the added item's id does not collide")
		href, _ := els["manifest"][2].attr("", "href")
		assert.Equal(t, newCover, entries[href])

		var covers []string
		for _, e := range els["metadata"] {
			if name, _ := e.attr("", "name"); name == "cover" {
				content, _ := e.attr("", "content")
				covers = append(covers, content)
			}
		}
		assert.Equal(t, []string{addedID}, covers)
	})

	t.Run("image item with a conventional cover id", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="bookid">abc</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="Cover-Image" href="images/cover.jpg" media-type="image/jpeg"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{"chapter1.xhtml": coverChapter, "images/cover.jpg": "original cover"},
		}, coverBook(), nil, newCover, "image/jpeg")

		entries := zipEntries(t, dest)
		assert.Equal(t, newCover, entries["images/cover.jpg"], "the parser's id fallback is swapped too")
		assert.Len(t, opfElements(t, entries["content.opf"])["manifest"], 2, "no second cover is added")
	})

	t.Run("the last meta name=cover wins, as in the parser", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="bookid">abc</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
    <meta name="cover" content="first"/>
    <meta name="cover" content="second"/>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="first" href="first.jpg" media-type="image/jpeg"/>
    <item id="second" href="second.jpg" media-type="image/jpeg"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{"chapter1.xhtml": coverChapter, "first.jpg": "first", "second.jpg": "second"},
		}, coverBook(), nil, newCover, "image/jpeg")

		entries := zipEntries(t, dest)
		assert.Equal(t, newCover, entries["second.jpg"])
		assert.Equal(t, []byte("first"), entries["first.jpg"])

		metadata, err := epub.Parse(dest)
		require.NoError(t, err)
		assert.Equal(t, newCover, metadata.CoverData)
	})

	t.Run("no Shisho cover leaves a coverless package alone", func(t *testing.T) {
		t.Parallel()

		dest := generateVersioned(t, versionedEPUB{
			opfPath: "content.opf",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
			files: map[string]string{"chapter1.xhtml": coverChapter},
		}, coverBook(), nil, nil, "")

		entries := zipEntries(t, dest)
		assert.Len(t, entries, 4, "mimetype, container, OPF, and chapter only")
		assert.Len(t, opfElements(t, entries["content.opf"])["manifest"], 1)
	})
}

const epub2AttrsOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title opf:file-as="Source, The">The Source</dc:title>
    <dc:creator opf:role="aut" opf:file-as="Doe, Jane">Jane Doe</dc:creator>
    <dc:creator opf:role="edt" opf:file-as="Itor, Ed">Ed Itor</dc:creator>
    <dc:contributor opf:role="bkp">calibre</dc:contributor>
    <dc:identifier id="bookid" opf:scheme="UUID">0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`

func attrsBook() *models.Book {
	return &models.Book{
		Title: "The Source",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "Jane Doe", SortName: "Doe, Jane"}},
			{SortOrder: 1, Person: &models.Person{Name: "New Person", SortName: "Person, New"}},
		},
	}
}

func attrsFile() *models.File {
	return &models.File{
		FileType: models.FileTypeEPUB,
		Identifiers: []*models.FileIdentifier{
			{Type: "uuid", Value: "0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10"},
			{Type: "goodreads", Value: "12345"},
		},
	}
}

func TestEPUBGenerator_EPUB2WritesOPFNamespacedAttributes(t *testing.T) {
	t.Parallel()

	dest := generateVersioned(t, versionedEPUB{
		opfPath: "content.opf",
		opf:     epub2AttrsOPF,
		files:   map[string]string{"chapter1.xhtml": coverChapter},
	}, attrsBook(), attrsFile(), nil, "")

	raw := zipEntries(t, dest)["content.opf"]
	assertValidXML(t, raw)
	assertNoDuplicateAttrs(t, raw)
	assertEPUBCheck(t, dest)

	els := opfElements(t, raw)["metadata"]
	for _, e := range els {
		for _, local := range []string{"role", "file-as", "scheme"} {
			_, plain := e.attr("", local)
			assert.False(t, plain, "<%s> has an unprefixed %s attribute:\n%s", e.Name.Local, local, raw)
		}
	}

	expect := []struct {
		local, text, role, fileAs string
	}{
		{"creator", "Jane Doe", "aut", "Doe, Jane"},
		{"creator", "New Person", "aut", "Person, New"},
		{"creator", "Ed Itor", "edt", "Itor, Ed"},
		{"contributor", "calibre", "bkp", ""},
	}
	for _, want := range expect {
		e, ok := findByText(els, want.local, want.text)
		require.True(t, ok, "missing %s %q:\n%s", want.local, want.text, raw)
		role, _ := e.attr(opfNamespace, "role")
		assert.Equal(t, want.role, role, "%s opf:role", want.text)
		fileAs, _ := e.attr(opfNamespace, "file-as")
		assert.Equal(t, want.fileAs, fileAs, "%s opf:file-as", want.text)
	}

	title, ok := findByText(els, "title", "The Source")
	require.True(t, ok)
	fileAs, _ := title.attr(opfNamespace, "file-as")
	assert.Equal(t, "Source, The", fileAs, "an unchanged title keeps its opf:file-as")

	uuid, ok := findByText(els, "identifier", "0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10")
	require.True(t, ok)
	scheme, _ := uuid.attr(opfNamespace, "scheme")
	assert.Equal(t, "UUID", scheme)
	goodreads, ok := findByText(els, "identifier", "12345")
	require.True(t, ok)
	scheme, _ = goodreads.attr(opfNamespace, "scheme")
	assert.Equal(t, "GOODREADS", scheme)

	metadata, err := epub.Parse(dest)
	require.NoError(t, err)
	var authors []string
	for _, a := range metadata.Authors {
		authors = append(authors, a.Name)
	}
	assert.Equal(t, []string{"Jane Doe", "New Person"}, authors)
}

const epub3AttrsOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="pub-id">urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:identifier id="old-isbn">9780316769488</dc:identifier>
    <meta refines="#old-isbn" property="identifier-type" scheme="onix:codelist5">15</meta>
    <dc:title opf:file-as="Source, The">The Source</dc:title>
    <dc:creator id="cre1">Jane Doe</dc:creator>
    <meta refines="#cre1" property="role" scheme="marc:relators">aut</meta>
    <meta refines="#cre1" property="file-as">Doe, Jane</meta>
    <meta refines="#cre1" property="alternate-script" xml:lang="ja">ジェーン・ドウ</meta>
    <dc:creator id="cre2">Old Author</dc:creator>
    <meta refines="#cre2" property="role" scheme="marc:relators">aut</meta>
    <meta refines="#cre2" property="file-as">Author, Old</meta>
    <dc:creator id="ill" opf:role="ill">Ill Ustrator</dc:creator>
    <dc:contributor id="bkp">calibre</dc:contributor>
    <meta refines="#bkp" property="role" scheme="marc:relators">bkp</meta>
    <dc:contributor id="trl">Trans Lator</dc:contributor>
    <meta refines="#trl" property="role" scheme="marc:relators">trl</meta>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`

func TestEPUBGenerator_EPUB3WritesRefinesInsteadOfAttributes(t *testing.T) {
	t.Parallel()

	dest := generateVersioned(t, versionedEPUB{
		opfPath: "content.opf",
		opf:     epub3AttrsOPF,
		files:   map[string]string{"chapter1.xhtml": coverChapter},
	}, attrsBook(), attrsFile(), nil, "")

	raw := zipEntries(t, dest)["content.opf"]
	assertValidXML(t, raw)
	assertNoDuplicateAttrs(t, raw)
	assertEPUBCheck(t, dest)

	els := opfElements(t, raw)["metadata"]
	for _, e := range els {
		if e.Name.Space != "http://purl.org/dc/elements/1.1/" {
			continue
		}
		for _, local := range []string{"role", "file-as", "scheme"} {
			for _, space := range []string{"", opfNamespace} {
				_, has := e.attr(space, local)
				assert.False(t, has, "EPUB 3 <dc:%s> carries a %s attribute:\n%s", e.Name.Local, local, raw)
			}
		}
	}

	// ids referenced by refines must exist.
	ids := map[string]bool{}
	for _, e := range els {
		if id, ok := e.attr("", "id"); ok {
			ids[id] = true
		}
	}
	for _, e := range els {
		if refines, ok := e.attr("", "refines"); ok {
			assert.True(t, ids[strings.TrimPrefix(refines, "#")], "meta refines %s, which no element has:\n%s", refines, raw)
		}
	}

	// An unchanged title's opf:file-as becomes a refinement.
	title, ok := findByText(els, "title", "The Source")
	require.True(t, ok)
	titleID, _ := title.attr("", "id")
	require.NotEmpty(t, titleID)
	assert.Equal(t, "Source, The", refinesFor(els, titleID)["file-as"].Text)

	// The kept author keeps its id and its refinements, including the
	// ones Shisho does not model.
	jane, ok := findByText(els, "creator", "Jane Doe")
	require.True(t, ok)
	janeID, _ := jane.attr("", "id")
	assert.Equal(t, "cre1", janeID)
	janeRefines := refinesFor(els, janeID)
	assert.Equal(t, "aut", janeRefines["role"].Text)
	relators, _ := janeRefines["role"].attr("", "scheme")
	assert.Equal(t, "marc:relators", relators)
	assert.Equal(t, "Doe, Jane", janeRefines["file-as"].Text)
	assert.Equal(t, "ジェーン・ドウ", janeRefines["alternate-script"].Text)

	// A new author gets an id and refinements.
	newPerson, ok := findByText(els, "creator", "New Person")
	require.True(t, ok)
	newID, _ := newPerson.attr("", "id")
	require.NotEmpty(t, newID)
	newRefines := refinesFor(els, newID)
	assert.Equal(t, "aut", newRefines["role"].Text)
	assert.Equal(t, "Person, New", newRefines["file-as"].Text)

	// The dropped author and its refinements are gone.
	_, ok = findByText(els, "creator", "Old Author")
	assert.False(t, ok)

	// Non-author creators survive, with their role moved into refines.
	ill, ok := findByText(els, "creator", "Ill Ustrator")
	require.True(t, ok, "an EPUB 3 illustrator whose role is an attribute is kept:\n%s", raw)
	illID, _ := ill.attr("", "id")
	assert.Equal(t, "ill", refinesFor(els, illID)["role"].Text)
	bkp, ok := findByText(els, "contributor", "calibre")
	require.True(t, ok)
	bkpID, _ := bkp.attr("", "id")
	assert.Equal(t, "bkp", refinesFor(els, bkpID)["role"].Text)
	trl, ok := findByText(els, "contributor", "Trans Lator")
	require.True(t, ok, "every contributor survives, not just the last:\n%s", raw)
	trlID, _ := trl.attr("", "id")
	assert.Equal(t, "trl", refinesFor(els, trlID)["role"].Text)

	// Identifier types move into refines too.
	goodreads, ok := findByText(els, "identifier", "12345")
	require.True(t, ok)
	goodreadsID, _ := goodreads.attr("", "id")
	require.NotEmpty(t, goodreadsID)
	assert.Equal(t, "GOODREADS", refinesFor(els, goodreadsID)["identifier-type"].Text)

	// Shisho reads its own output back the same way.
	metadata, err := epub.Parse(dest)
	require.NoError(t, err)
	var authors []string
	for _, a := range metadata.Authors {
		authors = append(authors, a.Name)
	}
	assert.Equal(t, []string{"Jane Doe", "New Person"}, authors)
	types := map[string]string{}
	for _, id := range metadata.Identifiers {
		types[id.Type] = id.Value
	}
	assert.Equal(t, "12345", types["goodreads"])
}

// TestKepubEPUBGenerator_AddedCoverIsMarkedCoverImage checks the KePub step
// marks a cover the EPUB generator added to an EPUB 2 package, exactly once.
func TestKepubEPUBGenerator_AddedCoverIsMarkedCoverImage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.epub")
	writeVersionedEPUB(t, srcPath, versionedEPUB{
		opfPath: "content.opf",
		opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="bookid">abc</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
		files: map[string]string{"chapter1.xhtml": coverChapter},
	})
	coverFilename := "source.epub.cover.jpg"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, coverFilename), []byte("shisho cover"), 0600))
	mimeType := "image/jpeg"
	file := &models.File{
		FileType:           models.FileTypeEPUB,
		Filepath:           srcPath,
		CoverImageFilename: &coverFilename,
		CoverMimeType:      &mimeType,
	}

	destPath := filepath.Join(tmpDir, "dest.kepub.epub")
	require.NoError(t, NewKepubEPUBGenerator().Generate(context.Background(), srcPath, destPath, coverBook(), file))

	entries := zipEntries(t, destPath)
	els := opfElements(t, entries["content.opf"])
	var coverID string
	for _, e := range els["metadata"] {
		if name, _ := e.attr("", "name"); name == "cover" {
			coverID, _ = e.attr("", "content")
		}
	}
	require.NotEmpty(t, coverID)
	for _, item := range els["manifest"] {
		if id, _ := item.attr("", "id"); id == coverID {
			props, _ := item.attr("", "properties")
			marks := 0
			for _, prop := range strings.Fields(props) {
				if prop == "cover-image" {
					marks++
				}
			}
			assert.Equal(t, 1, marks, "KePub marks the added cover exactly once:\n%s", entries["content.opf"])
			return
		}
	}
	t.Fatalf("no manifest item %q:\n%s", coverID, entries["content.opf"])
}

// TestEPUBGenerator_EPUB3NonMARCRoleIsNotDuplicated covers a role refinement
// in another vocabulary (ONIX code A01 is "By (author)"). The generator
// cannot read it as a MARC relator, so it must not keep the creator as a
// non-author next to the book's author of the same name.
func TestEPUBGenerator_EPUB3NonMARCRoleIsNotDuplicated(t *testing.T) {
	t.Parallel()

	dest := generateVersioned(t, versionedEPUB{
		opfPath: "content.opf",
		opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:creator id="c1">Jane Writer</dc:creator>
    <meta refines="#c1" property="role" scheme="onix:codelist17">A01</meta>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="chapter1"/></spine>
</package>`,
		files: map[string]string{"chapter1.xhtml": coverChapter},
	}, &models.Book{
		Title:   "Source",
		Authors: []*models.Author{{SortOrder: 0, Person: &models.Person{Name: "Jane Writer", SortName: "Writer, Jane"}}},
	}, nil, nil, "")

	raw := zipEntries(t, dest)["content.opf"]
	assertValidXML(t, raw)
	els := opfElements(t, raw)["metadata"]

	var creators []opfElement
	for _, e := range els {
		if e.Name.Local == "creator" && e.Text == "Jane Writer" {
			creators = append(creators, e)
		}
	}
	require.Len(t, creators, 1, "the author is written once:\n%s", raw)
	id, _ := creators[0].attr("", "id")
	assert.Equal(t, "c1", id, "the source element is reused")
	role := refinesFor(els, id)["role"]
	assert.Equal(t, "aut", role.Text)
	scheme, _ := role.attr("", "scheme")
	assert.Equal(t, "marc:relators", scheme)
}

// tocOPF builds a package with a nav document (EPUB 3 only) and an NCX
// referenced from the spine, both describing tocTree.
func tocOPF(version string) string {
	nav := ""
	if version == "3.0" {
		nav = `
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="` + version + `" unique-identifier="pub-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:title>Source</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>` + nav + `
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="part1" href="text/part1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx"><itemref idref="part1"/></spine>
</package>`
}

// tocNav is the EPUB 3 navigation document. The heading span, the landmarks
// nav, and the untitled list item are markup the parser does not turn into
// chapters, so rewriting the table of contents must leave them as written.
const tocNav = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Contents</title></head>
<body>
  <nav epub:type="toc" id="toc">
    <h1>Contents</h1>
    <ol>
      <li class="part"><a href="text/part1.xhtml">Part One</a>
        <ol>
          <li><a href="text/ch1.xhtml">Chapter 1</a></li>
          <li><a href="text/ch2.xhtml">Chapter 2</a></li>
        </ol>
      </li>
      <li class="part"><span>Part Two</span>
        <ol>
          <li><a href="text/ch3.xhtml#start">Chapter 3</a></li>
        </ol>
      </li>
      <li><a href="text/blank.xhtml"></a></li>
      <li><a href="text/after.xhtml">Afterword</a></li>
    </ol>
  </nav>
  <nav epub:type="landmarks" hidden="">
    <ol><li><a epub:type="bodymatter" href="text/part1.xhtml">Start</a></li></ol>
  </nav>
</body>
</html>`

const tocNCX = `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <head><meta name="dtb:uid" content="urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10"/></head>
  <docTitle><text>Source</text></docTitle>
  <navMap>
    <navPoint id="np1" playOrder="1">
      <navLabel><text>Part One</text></navLabel>
      <content src="text/part1.xhtml"/>
      <navPoint id="np2" playOrder="2">
        <navLabel><text>Chapter 1</text></navLabel>
        <content src="text/ch1.xhtml"/>
      </navPoint>
      <navPoint id="np3" playOrder="3">
        <navLabel><text>Chapter 2</text></navLabel>
        <content src="text/ch2.xhtml"/>
      </navPoint>
    </navPoint>
    <navPoint id="np4" playOrder="4">
      <navLabel><text>Part Two</text></navLabel>
      <content src="text/part2.xhtml"/>
      <navPoint id="np5" playOrder="5">
        <navLabel><text>Chapter 3</text></navLabel>
        <content src="text/ch3.xhtml#start"/>
      </navPoint>
    </navPoint>
    <navPoint id="np6" playOrder="6">
      <navLabel><text>Afterword</text></navLabel>
      <content src="text/after.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`

func tocEPUB(version string) versionedEPUB {
	files := map[string]string{
		"OEBPS/toc.ncx":          tocNCX,
		"OEBPS/text/part1.xhtml": coverChapter,
	}
	if version == "3.0" {
		files["OEBPS/nav.xhtml"] = tocNav
	}
	return versionedEPUB{opfPath: "OEBPS/content.opf", opf: tocOPF(version), files: files}
}

// tocChapter is a stored chapter in a test tree.
type tocChapter struct {
	title    string
	href     string // empty for a heading without a link
	children []tocChapter
}

// storedChapters returns tree in the shape RetrieveFileWithRelations loads:
// every chapter of the file in one flat list, ordered by sort order, each
// with its direct children attached.
func storedChapters(tree []tocChapter) []*models.Chapter {
	var all []*models.Chapter
	nextID := 1
	var add func(nodes []tocChapter, parentID *int) []*models.Chapter
	add = func(nodes []tocChapter, parentID *int) []*models.Chapter {
		level := make([]*models.Chapter, 0, len(nodes))
		for i, n := range nodes {
			ch := &models.Chapter{ID: nextID, ParentID: parentID, SortOrder: i, Title: n.title}
			nextID++
			if n.href != "" {
				href := n.href
				ch.Href = &href
			}
			all = append(all, ch)
			level = append(level, ch)
			ch.Children = add(n.children, &ch.ID)
		}
		return level
	}
	add(tree, nil)
	sort.SliceStable(all, func(i, j int) bool { return all[i].SortOrder < all[j].SortOrder })
	return all
}

// navTree reads a parsed chapter tree as tocChapters for comparison.
func navTree(chapters []mediafile.ParsedChapter) []tocChapter {
	var out []tocChapter
	for _, ch := range chapters {
		n := tocChapter{title: ch.Title, children: navTree(ch.Children)}
		if ch.Href != nil {
			n.href = *ch.Href
		}
		out = append(out, n)
	}
	return out
}

// ncxTree parses the generated NCX directly, since the parser only reads it
// when a package has no usable nav document.
func ncxTree(t *testing.T, raw []byte) []tocChapter {
	t.Helper()
	var ncx epub.NCX
	require.NoError(t, xml.Unmarshal(raw, &ncx))
	var walk func(points []epub.NCXNavPoint) []tocChapter
	walk = func(points []epub.NCXNavPoint) []tocChapter {
		var out []tocChapter
		for _, p := range points {
			out = append(out, tocChapter{title: p.NavLabel.Text, href: p.Content.Src, children: walk(p.Children)})
		}
		return out
	}
	return walk(ncx.NavMap.NavPoints)
}

// originalTOC is what a scan stores for tocEPUB: the nav and the NCX agree,
// except that the nav's Part Two is a heading without a link.
func originalTOC(version string) []tocChapter {
	partTwoHref := "text/part2.xhtml"
	if version == "3.0" {
		partTwoHref = ""
	}
	return []tocChapter{
		{title: "Part One", href: "text/part1.xhtml", children: []tocChapter{
			{title: "Chapter 1", href: "text/ch1.xhtml"},
			{title: "Chapter 2", href: "text/ch2.xhtml"},
		}},
		{title: "Part Two", href: partTwoHref, children: []tocChapter{
			{title: "Chapter 3", href: "text/ch3.xhtml#start"},
		}},
		{title: "Afterword", href: "text/after.xhtml"},
	}
}

// editedTOC renames Chapter 1, deletes Chapter 2, Part Two's only child,
// and the Afterword.
func editedTOC(version string) []tocChapter {
	toc := originalTOC(version)
	return []tocChapter{
		{title: toc[0].title, href: toc[0].href, children: []tocChapter{
			{title: "The <Beginning> & End", href: "text/ch1.xhtml"},
		}},
		{title: toc[1].title, href: toc[1].href},
	}
}

func TestEPUBGenerator_WritesChapterEdits(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"2.0", "3.0"} {
		t.Run("EPUB "+version, func(t *testing.T) {
			t.Parallel()

			file := &models.File{FileType: models.FileTypeEPUB, Chapters: storedChapters(editedTOC(version))}
			dest := generateVersioned(t, tocEPUB(version), coverBook(), file, nil, "")
			entries := zipEntries(t, dest)

			metadata, err := epub.Parse(dest)
			require.NoError(t, err)
			assert.Equal(t, editedTOC(version), navTree(metadata.Chapters), "the parsed table of contents carries the edits")

			assert.Equal(t, editedTOC("2.0"), ncxTree(t, entries["OEBPS/toc.ncx"]), "the NCX carries the edits")
			assertValidXML(t, entries["OEBPS/toc.ncx"])

			if version == "3.0" {
				nav := string(entries["OEBPS/nav.xhtml"])
				assertValidXML(t, entries["OEBPS/nav.xhtml"])
				assert.Contains(t, nav, `<h1>Contents</h1>`, "markup outside the entries is kept")
				assert.Contains(t, nav, `<li><a href="text/blank.xhtml"></a></li>`, "an entry the parser skips is kept")
				assert.Contains(t, nav, `<nav epub:type="landmarks" hidden="">`, "other nav elements are kept")
				assert.Contains(t, nav, `<li class="part"><span>Part Two</span>`, "kept entries keep their attributes")
				assert.Contains(t, nav, `The &lt;Beginning&gt; &amp; End`, "the new title is escaped")
				assert.NotContains(t, nav, "Chapter 3", "a deleted entry is removed")
				assert.Equal(t, 3, strings.Count(nav, "<ol>"), "a list left without entries is removed")
			}
			assertEPUBCheck(t, dest)
		})
	}
}

func TestEPUBGenerator_UneditedChaptersKeepTOCBytes(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"2.0", "3.0"} {
		t.Run("EPUB "+version, func(t *testing.T) {
			t.Parallel()

			src := tocEPUB(version)
			cases := map[string][]*models.Chapter{
				"chapters as scanned": storedChapters(originalTOC(version)),
				"no chapters":         nil,
				// Adding and reordering chapters are not supported, so a
				// tree that is not the original with entries renamed or
				// deleted leaves the table of contents as written.
				"added chapter": storedChapters(append(originalTOC(version), tocChapter{title: "New", href: "text/new.xhtml"})),
				"reordered chapters": storedChapters([]tocChapter{
					originalTOC(version)[2], originalTOC(version)[0], originalTOC(version)[1],
				}),
			}
			for name, chapters := range cases {
				file := &models.File{FileType: models.FileTypeEPUB, Chapters: chapters}
				entries := zipEntries(t, generateVersioned(t, src, coverBook(), file, nil, ""))
				assert.Equal(t, src.files["OEBPS/toc.ncx"], string(entries["OEBPS/toc.ncx"]), name)
				assert.Equal(t, src.files["OEBPS/nav.xhtml"], string(entries["OEBPS/nav.xhtml"]), name)
			}
		})
	}
}

func TestKepubEPUBGenerator_WritesChapterEdits(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.epub")
	writeVersionedEPUB(t, srcPath, tocEPUB("3.0"))
	file := &models.File{FileType: models.FileTypeEPUB, Filepath: srcPath, Chapters: storedChapters(editedTOC("3.0"))}

	destPath := filepath.Join(tmpDir, "dest.kepub.epub")
	require.NoError(t, NewKepubEPUBGenerator().Generate(context.Background(), srcPath, destPath, coverBook(), file))

	// KePub wraps the body and every label in Kobo spans, so the nav is
	// checked as text rather than parsed.
	entries := zipEntries(t, destPath)
	nav := string(entries["OEBPS/nav.xhtml"])
	assert.Contains(t, nav, "The &lt;Beginning&gt; &amp; End")
	for _, deleted := range []string{"Chapter 1", "Chapter 2", "Chapter 3", "Afterword"} {
		assert.NotContains(t, nav, deleted)
	}
	assert.Equal(t, editedTOC("2.0"), ncxTree(t, entries["OEBPS/toc.ncx"]))
}

// TestEPUBGenerator_DeletesTheEditedEntryAmongSameHrefs covers entries that
// point at the same file: deleting the first must not keep it under the
// second one's title.
func TestEPUBGenerator_DeletesTheEditedEntryAmongSameHrefs(t *testing.T) {
	t.Parallel()

	src := tocEPUB("2.0")
	src.files["OEBPS/toc.ncx"] = `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="a" playOrder="1">
      <navLabel><text>Opening</text></navLabel>
      <content src="text/ch1.xhtml"/>
      <navPoint id="a1" playOrder="2"><navLabel><text>Scene</text></navLabel><content src="text/ch1.xhtml#s"/></navPoint>
    </navPoint>
    <navPoint id="b" playOrder="3">
      <navLabel><text>Opening, continued</text></navLabel>
      <content src="text/ch1.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`
	file := &models.File{FileType: models.FileTypeEPUB, Chapters: storedChapters([]tocChapter{
		{title: "Opening, continued", href: "text/ch1.xhtml"},
	})}
	entries := zipEntries(t, generateVersioned(t, src, coverBook(), file, nil, ""))

	ncx := string(entries["OEBPS/toc.ncx"])
	assert.Equal(t, []tocChapter{{title: "Opening, continued", href: "text/ch1.xhtml"}}, ncxTree(t, entries["OEBPS/toc.ncx"]))
	assert.Contains(t, ncx, `<navPoint id="b" playOrder="3">`, "the second entry is the one kept")
}
