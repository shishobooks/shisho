package filegen

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// epub3FidelityOPF is an EPUB 3 package that uses the attributes opfPackage
// does not model by name: manifest properties (nav, cover-image), itemref
// linear and properties, spine page-progression-direction, the package prefix
// and xml:lang, and a unique-identifier pointing at a dc:identifier id.
const epub3FidelityOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id" prefix="rendition: http://www.idpf.org/vocab/rendition/#" xml:lang="en">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="pub-id">urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10</dc:identifier>
    <dc:title id="t1">Source Title</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="cover" href="cover.jpg" media-type="image/jpeg" properties="cover-image"/>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="notes" href="notes.xhtml" media-type="application/xhtml+xml" properties="scripted"/>
  </manifest>
  <spine page-progression-direction="ltr">
    <itemref idref="chapter1" properties="page-spread-right"/>
    <itemref idref="notes" linear="no"/>
  </spine>
</package>`

const epub3FidelityNav = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Contents</title></head>
<body>
  <nav epub:type="toc"><ol><li><a href="chapter1.xhtml">Chapter One</a></li></ol></nav>
</body>
</html>`

func createEPUBWithOPF(t *testing.T, path, opf string, extra map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	mw, err := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	require.NoError(t, err)
	_, err = mw.Write([]byte("application/epub+zip"))
	require.NoError(t, err)

	files := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"content.opf": opf,
	}
	for name, content := range extra {
		files[name] = content
	}
	for _, name := range []string{"META-INF/container.xml", "content.opf"} {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(files[name]))
		require.NoError(t, err)
		delete(files, name)
	}
	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
}

// fidelityPackage reads the generated OPF with its own struct so the
// assertions do not depend on what opfPackage happens to model.
type fidelityPackage struct {
	UniqueIdentifier string `xml:"unique-identifier,attr"`
	Prefix           string `xml:"prefix,attr"`
	Lang             string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Identifiers      []struct {
		ID   string `xml:"id,attr"`
		Text string `xml:",chardata"`
	} `xml:"metadata>identifier"`
	Titles []struct {
		Text string `xml:",chardata"`
		Lang string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	} `xml:"metadata>title"`
	Items []struct {
		ID         string `xml:"id,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine struct {
		PageProgressionDirection string `xml:"page-progression-direction,attr"`
		Itemrefs                 []struct {
			IDRef      string `xml:"idref,attr"`
			Linear     string `xml:"linear,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

func generateFidelityEPUB(t *testing.T, file *models.File) (string, fidelityPackage) {
	t.Helper()
	return generateFromOPF(t, epub3FidelityOPF, file)
}

func generateFromOPF(t *testing.T, opf string, file *models.File) (string, fidelityPackage) {
	t.Helper()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.epub")
	createEPUBWithOPF(t, srcPath, opf, map[string]string{
		"nav.xhtml":      epub3FidelityNav,
		"chapter1.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>One</p></body></html>`,
		"notes.xhtml":    `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>Notes</p></body></html>`,
		"cover.jpg":      "not-really-a-jpeg",
	})
	destPath := filepath.Join(tmpDir, "dest.epub")

	book := &models.Book{
		Title: "Regenerated Title",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "Author"}},
		},
	}
	require.NoError(t, (&EPUBGenerator{}).Generate(context.Background(), srcPath, destPath, book, file))

	raw := readFileFromEPUB(t, destPath, "content.opf")
	var pkg fidelityPackage
	require.NoError(t, xml.Unmarshal(raw, &pkg), "generated OPF must be well-formed:\n%s", raw)
	return destPath, pkg
}

func TestEPUBGenerator_PreservesManifestAndSpineAttributes(t *testing.T) {
	t.Parallel()

	destPath, pkg := generateFidelityEPUB(t, &models.File{FileType: models.FileTypeEPUB})

	props := map[string]string{}
	for _, item := range pkg.Items {
		props[item.ID] = item.Properties
	}
	assert.Equal(t, "nav", props["nav"])
	assert.Equal(t, "cover-image", props["cover"])
	assert.Equal(t, "scripted", props["notes"])

	require.Len(t, pkg.Spine.Itemrefs, 2)
	assert.Equal(t, "ltr", pkg.Spine.PageProgressionDirection)
	assert.Equal(t, "page-spread-right", pkg.Spine.Itemrefs[0].Properties)
	assert.Equal(t, "no", pkg.Spine.Itemrefs[1].Linear)

	assert.Equal(t, "rendition: http://www.idpf.org/vocab/rendition/#", pkg.Prefix)
	assert.Equal(t, "en", pkg.Lang)

	// The nav document is found through properties="nav", so chapters parse
	// from the generated file only when that attribute survives.
	metadata, err := epub.Parse(destPath)
	require.NoError(t, err)
	require.NotEmpty(t, metadata.Chapters)
	assert.Equal(t, "Chapter One", metadata.Chapters[0].Title)
}

func TestEPUBGenerator_GeneratedOPFHasNoMangledNamespaces(t *testing.T) {
	t.Parallel()

	destPath, _ := generateFidelityEPUB(t, &models.File{FileType: models.FileTypeEPUB})
	raw := readFileFromEPUB(t, destPath, "content.opf")

	// Namespace declarations read from the source must not be re-emitted as
	// ordinary attributes, which encoding/xml would print as "_xmlns:..." or
	// as a duplicate xmlns attribute. encoding/xml tolerates duplicate
	// attributes on decode, so check each start tag's raw attribute names.
	assert.NotContains(t, string(raw), "_xmlns")
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

func TestEPUBGenerator_KeepsUniqueIdentifierWhenReplacingIdentifiers(t *testing.T) {
	t.Parallel()

	_, pkg := generateFidelityEPUB(t, &models.File{
		FileType: models.FileTypeEPUB,
		Identifiers: []*models.FileIdentifier{
			{Type: "isbn_13", Value: "9780316769488"},
		},
	})

	require.Equal(t, "pub-id", pkg.UniqueIdentifier)
	values := map[string]string{}
	for _, id := range pkg.Identifiers {
		values[id.Text] = id.ID
	}
	assert.Equal(t, "pub-id", values["urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10"],
		"the identifier referenced by unique-identifier must survive with its id")
	assert.Contains(t, values, "9780316769488")
}

func TestEPUBGenerator_DoesNotDuplicateUniqueIdentifierValue(t *testing.T) {
	t.Parallel()

	_, pkg := generateFidelityEPUB(t, &models.File{
		FileType: models.FileTypeEPUB,
		Identifiers: []*models.FileIdentifier{
			{Type: "uuid", Value: "urn:uuid:0b5e5b3c-1f4e-4a8e-9c1d-3f7c2a9e8b10"},
		},
	})

	require.Len(t, pkg.Identifiers, 1)
	assert.Equal(t, "pub-id", pkg.Identifiers[0].ID)
}

// isbnUniqueOPF points unique-identifier at an ISBN, as many publisher EPUBs
// do. The title carries its own xml:lang.
const isbnUniqueOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id" xml:lang="ja">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="pub-id">urn:isbn:9780316769488</dc:identifier>
    <dc:title xml:lang="ja">Source Title</dc:title>
    <dc:language>ja</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
  </spine>
</package>`

func TestEPUBGenerator_CorrectedISBNReplacesStaleUniqueIdentifier(t *testing.T) {
	t.Parallel()

	_, pkg := generateFromOPF(t, isbnUniqueOPF, &models.File{
		FileType: models.FileTypeEPUB,
		Identifiers: []*models.FileIdentifier{
			{Type: "isbn_13", Value: "9780306406157"},
			{Type: "asin", Value: "B08N5WRWNW"},
		},
	})

	require.Equal(t, "pub-id", pkg.UniqueIdentifier)
	values := map[string]string{}
	for _, id := range pkg.Identifiers {
		values[id.Text] = id.ID
	}
	require.Len(t, values, 2, "the stale ISBN is gone")
	assert.Equal(t, "pub-id", values["9780306406157"], "the user's ISBN takes over the unique identifier")
	// The ASIN gets an id of its own so its EPUB 3 identifier-type
	// refinement has something to point at.
	assert.NotEmpty(t, values["B08N5WRWNW"])
	assert.NotEqual(t, "pub-id", values["B08N5WRWNW"])
}

func TestEPUBGenerator_SameISBNWithURNPrefixIsNotDuplicated(t *testing.T) {
	t.Parallel()

	_, pkg := generateFromOPF(t, isbnUniqueOPF, &models.File{
		FileType:    models.FileTypeEPUB,
		Identifiers: []*models.FileIdentifier{{Type: "isbn_13", Value: "9780316769488"}},
	})

	require.Len(t, pkg.Identifiers, 1)
	assert.Equal(t, "pub-id", pkg.Identifiers[0].ID)
}

func TestEPUBGenerator_RetitleDropsStaleTitleAttributes(t *testing.T) {
	t.Parallel()

	lang := "en"
	_, pkg := generateFromOPF(t, isbnUniqueOPF, &models.File{
		FileType: models.FileTypeEPUB,
		Language: &lang,
	})

	require.NotEmpty(t, pkg.Titles)
	assert.Equal(t, "Regenerated Title", pkg.Titles[0].Text)
	assert.Empty(t, pkg.Titles[0].Lang, "the source title's xml:lang does not describe the new title")
	assert.Equal(t, "en", pkg.Lang, "the package language follows the file's language")
}
