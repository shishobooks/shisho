package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"net/url"
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
	defer zr.Close()
	protected, err := declaresDRM(&zr.Reader)
	if err != nil {
		return nil, err
	}
	if !protected {
		metadata, result, err := parseArchive(&zr.Reader)
		if err != nil {
			return nil, err
		}
		protected, err = spineEncrypted(&zr.Reader, result)
		if err != nil {
			return nil, err
		}
		if !protected {
			return metadata, nil
		}
	}
	return nil, errors.WithStack(mediafile.ErrDRMProtected)
}

// fontObfuscationAlgorithms are the encryption.xml algorithms that only
// obfuscate embedded fonts (IDPF and Adobe). A book that uses them alone is
// readable.
var fontObfuscationAlgorithms = map[string]bool{
	"http://www.idpf.org/2008/embedding": true,
	"http://ns.adobe.com/pdf/enc#RC":     true,
}

// declaresDRM reports whether an EPUB says its content is encrypted:
// Adobe DRM leaves a rights.xml, and other DRM lists the content it encrypts
// in encryption.xml.
func declaresDRM(zr *zip.Reader) (bool, error) {
	for _, f := range zr.File {
		switch {
		case strings.EqualFold(f.Name, "META-INF/rights.xml"):
			return true, nil
		case strings.EqualFold(f.Name, "META-INF/encryption.xml"):
			encrypted, err := encryptsContent(f)
			if err != nil || encrypted {
				return encrypted, err
			}
		}
	}
	return false, nil
}

// spineEncrypted reports whether every content document in the spine is
// something other than markup. The Kobo store encrypts a KePub's content
// documents with no record in the book, so that is what gives it away. A
// readable book can have one odd document, so one is never enough.
func spineEncrypted(zr *zip.Reader, result *ParseOPFResult) (bool, error) {
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}
	hrefs := make(map[string]string, len(result.Package.Manifest.Item))
	for _, item := range result.Package.Manifest.Item {
		hrefs[item.ID] = item.Href
	}
	checked := 0
	for _, ref := range result.Package.Spine.Itemref {
		href := hrefs[ref.Idref]
		if unescaped, err := url.PathUnescape(href); err == nil {
			href = unescaped
		}
		f := files[path.Clean(result.BasePath+href)]
		if f == nil {
			continue
		}
		markup, err := startsWithMarkup(f)
		if err != nil || markup {
			return false, err
		}
		checked++
	}
	return checked > 0, nil
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

// byteOrderMarks are the UTF-8, UTF-16, and UTF-32 byte order marks, longest
// first so UTF-32 LE is not read as UTF-16 LE.
var byteOrderMarks = [][]byte{
	{0x00, 0x00, 0xfe, 0xff}, {0xff, 0xfe, 0x00, 0x00},
	{0xef, 0xbb, 0xbf}, {0xfe, 0xff}, {0xff, 0xfe},
}

// startsWithMarkup reports whether a content document's first character,
// after any byte order mark and white space, opens a tag. NUL bytes are
// skipped, so UTF-16 and UTF-32 text reads as its ASCII. An empty document
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
	head = head[:n]
	for _, bom := range byteOrderMarks {
		if bytes.HasPrefix(head, bom) {
			head = head[len(bom):]
			break
		}
	}
	head = bytes.TrimLeft(head, " \t\r\n\x00")
	return len(head) == 0 || head[0] == '<', nil
}
