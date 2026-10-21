package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
)

// ParseKePub reads a KePub's metadata the way Parse reads an EPUB's, and
// returns mediafile.ErrDRMProtected for a KePub whose content is encrypted.
func ParseKePub(path string) (*mediafile.ParsedMetadata, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	protected, err := isDRMProtected(&zr.Reader)
	zr.Close()
	if err != nil {
		return nil, err
	}
	if protected {
		return nil, errors.WithStack(mediafile.ErrDRMProtected)
	}
	return Parse(path)
}

// fontObfuscationAlgorithms are the encryption.xml algorithms that only
// obfuscate embedded fonts (IDPF and Adobe). A book that uses them alone is
// readable.
var fontObfuscationAlgorithms = map[string]bool{
	"http://www.idpf.org/2008/embedding": true,
	"http://ns.adobe.com/pdf/enc#RC":     true,
}

// isDRMProtected reports whether an EPUB's content is encrypted: Adobe DRM
// leaves a rights.xml, other DRM lists the content it encrypts in
// encryption.xml, and the Kobo store encrypts content documents with no
// record in the book at all, so a content document that is not markup
// gives it away.
func isDRMProtected(zr *zip.Reader) (bool, error) {
	for _, f := range zr.File {
		switch {
		case strings.EqualFold(f.Name, "META-INF/rights.xml"):
			return true, nil
		case strings.EqualFold(f.Name, "META-INF/encryption.xml"):
			encrypted, err := encryptsContent(f)
			if err != nil || encrypted {
				return encrypted, err
			}
		case isContentDocument(f.Name):
			markup, err := startsWithMarkup(f)
			if err != nil || !markup {
				return !markup, err
			}
		}
	}
	return false, nil
}

// encryptsContent reports whether an encryption.xml encrypts anything with
// an algorithm other than font obfuscation.
func encryptsContent(f *zip.File) (bool, error) {
	r, err := f.Open()
	if err != nil {
		return false, errors.WithStack(err)
	}
	defer r.Close()
	var doc struct {
		Data []struct {
			Method struct {
				Algorithm string `xml:"Algorithm,attr"`
			} `xml:"EncryptionMethod"`
		} `xml:"EncryptedData"`
	}
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		// An encryption.xml Shisho cannot read says nothing it can trust,
		// so the content documents decide.
		return false, nil //nolint:nilerr // see above
	}
	for _, d := range doc.Data {
		if !fontObfuscationAlgorithms[d.Method.Algorithm] {
			return true, nil
		}
	}
	return false, nil
}

func isContentDocument(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".xhtml", ".html", ".htm":
		return true
	}
	return false
}

// startsWithMarkup reports whether a content document's first character,
// after any byte order mark and white space, opens a tag. An empty document
// counts: encryption never leaves one empty.
func startsWithMarkup(f *zip.File) (bool, error) {
	r, err := f.Open()
	if err != nil {
		return false, errors.WithStack(err)
	}
	defer r.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, errors.WithStack(err)
	}
	head = bytes.TrimPrefix(head[:n], []byte("\xef\xbb\xbf"))
	head = bytes.TrimLeft(head, " \t\r\n")
	return len(head) == 0 || head[0] == '<', nil
}
