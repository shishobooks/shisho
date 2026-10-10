package testutils

import (
	"archive/zip"
	"html"
	"os"

	"github.com/pkg/errors"
)

// writeMinimalEPUB writes a small valid EPUB at path so E2E tests can download
// a generated file. It carries only a title, an identifier, and one chapter,
// styled with a white body and black text the way Project Gutenberg EPUBs
// are, so the reader e2e test can check that themes override book colors.
func writeMinimalEPUB(path, title string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = errors.WithStack(cerr)
		}
	}()

	zw := zip.NewWriter(f)
	// The mimetype entry must come first and be stored uncompressed.
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := w.Write([]byte("application/epub+zip")); err != nil {
		return errors.WithStack(err)
	}

	escaped := html.EscapeString(title)
	entries := []struct{ name, body string }{
		{"META-INF/container.xml", `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`},
		{"OEBPS/content.opf", `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>` + escaped + `</dc:title>
    <dc:identifier id="bookid">urn:uuid:e2e-test-book</dc:identifier>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
  </spine>
</package>`},
		{"OEBPS/chapter1.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>` + escaped + `</title>
<style>body { color: black; background-color: white; } a { color: blue; }</style></head>
<body><p>Test chapter. <a href="#end">A link.</a></p><p id="end">The end.</p></body>
</html>`},
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			return errors.WithStack(err)
		}
	}
	return errors.WithStack(zw.Close())
}
