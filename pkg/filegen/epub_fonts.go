package filegen

import (
	"archive/zip"
	"bytes"
	"crypto/sha1" //nolint:gosec // the IDPF font obfuscation key is a SHA-1
	"encoding/hex"
	"encoding/xml"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/shishobooks/shisho/pkg/epub"
)

// Embedded fonts in an EPUB can be obfuscated: their first bytes XORed with
// a key made from the book's identifiers, as META-INF/encryption.xml
// records. A Generated EPUB can replace those identifiers, so its fonts are
// obfuscated again under the new ones, and the AZW3 conversion reads them
// in the clear.

// encryptionPath is where an EPUB records what it encrypts or obfuscates.
const encryptionPath = "META-INF/encryption.xml"

// The font obfuscation algorithms, which pkg/epub also knows, as the ones
// that leave a book readable, and how much of a font each obfuscates.
const (
	idpfObfuscation    = "http://www.idpf.org/2008/embedding"
	adobeObfuscation   = "http://ns.adobe.com/pdf/enc#RC"
	idpfObfuscatedLen  = 1040
	adobeObfuscatedLen = 1024
)

// encryptionDoc is the part of an encryption.xml that names obfuscated
// fonts.
type encryptionDoc struct {
	Data []struct {
		Method struct {
			Algorithm string `xml:"Algorithm,attr"`
		} `xml:"EncryptionMethod"`
		Cipher struct {
			Reference struct {
				URI string `xml:"URI,attr"`
			} `xml:"CipherReference"`
		} `xml:"CipherData"`
	} `xml:"EncryptedData"`
}

// obfuscatedFonts returns the entries an encryption.xml says are
// obfuscated fonts, with the algorithm of each. One that can't be read
// names none.
func obfuscatedFonts(data []byte) map[string]string {
	out := map[string]string{}
	var doc encryptionDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return out
	}
	for _, d := range doc.Data {
		if a := d.Method.Algorithm; a == idpfObfuscation || a == adobeObfuscation {
			out[encryptionEntry(d.Cipher.Reference.URI)] = a
		}
	}
	return out
}

// encryptionEntry returns the entry an encryption.xml URI names: a path
// from the container's root, possibly percent-encoded.
func encryptionEntry(uri string) string {
	if unescaped, err := url.PathUnescape(uri); err == nil {
		uri = unescaped
	}
	return path.Clean(strings.TrimPrefix(uri, "/"))
}

// obfuscationKey returns the key an algorithm makes from an identifier,
// and how many bytes of a font it obfuscates. The IDPF algorithm's key is
// the SHA-1 of the identifier with its whitespace removed; Adobe's is the
// 16 bytes of a UUID identifier.
func obfuscationKey(algorithm, id string) ([]byte, int, bool) {
	switch algorithm {
	case idpfObfuscation:
		key := sha1.Sum([]byte(strings.Map(func(r rune) rune { //nolint:gosec // the algorithm's key
			if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, id)))
		return key[:], idpfObfuscatedLen, true
	case adobeObfuscation:
		key, err := hex.DecodeString(strings.ReplaceAll(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id)), "urn:uuid:"), "-", ""))
		if err != nil || len(key) != 16 {
			return nil, 0, false
		}
		return key, adobeObfuscatedLen, true
	}
	return nil, 0, false
}

// xorFont returns data with its first n bytes XORed with key, which both
// obfuscates and undoes it.
func xorFont(data, key []byte, n int) []byte {
	out := bytes.Clone(data)
	for i := range min(n, len(out)) {
		out[i] ^= key[i%len(key)]
	}
	return out
}

// deobfuscateFont undoes a font's obfuscation. Which identifier keyed it
// isn't recorded, so each of ids is tried in turn, and the first that gives
// a font wins; it returns the font and that identifier, or false when none
// does.
func deobfuscateFont(data []byte, algorithm string, ids []string) ([]byte, string, bool) {
	for _, id := range ids {
		key, n, ok := obfuscationKey(algorithm, id)
		if !ok {
			continue
		}
		if out := xorFont(data, key, n); isFontData(out) {
			return out, id, true
		}
	}
	return nil, "", false
}

// isFontData reports whether data starts as a TrueType, OpenType, or WOFF
// font does.
func isFontData(data []byte) bool {
	for _, magic := range []string{"\x00\x01\x00\x00", "true", "OTTO", "wOFF", "wOF2"} {
		if bytes.HasPrefix(data, []byte(magic)) {
			return true
		}
	}
	return false
}

// epubPackageIdentifiers returns a parsed package's identifiers, its
// unique identifier first.
func epubPackageIdentifiers(pkg *epub.Package) []string {
	var ids []string
	for _, id := range pkg.Metadata.Identifier {
		ids = append(ids, id.Text)
	}
	return uniqueFirst(ids, func(i int) bool {
		return pkg.UniqueIdentifier != "" && pkg.Metadata.Identifier[i].ID == pkg.UniqueIdentifier
	})
}

// opfPackageIdentifiers returns the identifiers of a package the EPUB
// generator rewrites, its unique identifier first, and the unique
// identifier, or "" when the package has none.
func opfPackageIdentifiers(pkg *opfPackage) ([]string, string) {
	var ids []string
	unique := ""
	for _, id := range pkg.Metadata.Identifiers {
		ids = append(ids, id.Text)
		if pkg.UniqueIdentifier != "" && id.ID == pkg.UniqueIdentifier {
			unique = strings.TrimSpace(id.Text)
		}
	}
	return uniqueFirst(ids, func(i int) bool {
		return pkg.UniqueIdentifier != "" && pkg.Metadata.Identifiers[i].ID == pkg.UniqueIdentifier
	}), unique
}

// uniqueFirst returns ids trimmed, with the one isUnique picks moved first.
func uniqueFirst(ids []string, isUnique func(int) bool) []string {
	out := make([]string, 0, len(ids))
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if isUnique(i) {
			out = append([]string{id}, out...)
		} else {
			out = append(out, id)
		}
	}
	return out
}

// rekeyFonts returns the obfuscated fonts of src whose key the Generated
// EPUB's identifiers no longer make, obfuscated again under them, and the
// encryption.xml that goes with them, or nils when no font needs it. before
// and after are the package's identifiers before and after the generator
// rewrote them, and newUnique its unique identifier after, or "" for none.
// A font no identifier unlocks is left as it is, since nothing can read it
// either way.
//
// A font is obfuscated again with the IDPF algorithm, keyed on the new
// unique identifier, since an Adobe key needs a UUID the new identifiers
// may not have. With no unique identifier left, it is written in the clear
// and dropped from encryption.xml.
func rekeyFonts(src *zip.Reader, before, after []string, newUnique string) (map[string][]byte, []byte, error) {
	files := map[string]*zip.File{}
	for _, f := range src.File {
		files[f.Name] = f
	}
	encryption, ok := files[encryptionPath]
	if !ok {
		return nil, nil, nil
	}
	encData, err := readZipFile(encryption)
	if err != nil {
		return nil, nil, err
	}
	fonts := map[string][]byte{}
	// algorithms maps each rekeyed font to the algorithm it now has, ""
	// for none.
	algorithms := map[string]string{}
	for name, algorithm := range obfuscatedFonts(encData) {
		f, ok := files[name]
		if !ok {
			continue
		}
		data, err := readZipFile(f)
		if err != nil {
			return nil, nil, err
		}
		plain, id, ok := deobfuscateFont(data, algorithm, before)
		if !ok || keyStillMade(algorithm, id, after, newUnique) {
			continue
		}
		if newUnique == "" {
			fonts[name], algorithms[name] = plain, ""
			continue
		}
		key, n, _ := obfuscationKey(idpfObfuscation, newUnique)
		fonts[name], algorithms[name] = xorFont(plain, key, n), idpfObfuscation
	}
	if len(fonts) == 0 {
		return nil, nil, nil
	}
	return fonts, rewriteEncryption(encData, algorithms), nil
}

// keyStillMade reports whether the new identifiers still make the key id
// made: for the IDPF algorithm, id is still the unique identifier, and for
// Adobe's, still one of the identifiers.
func keyStillMade(algorithm, id string, after []string, newUnique string) bool {
	if algorithm == idpfObfuscation {
		return id == newUnique
	}
	for _, a := range after {
		if a == id {
			return true
		}
	}
	return false
}

var (
	encryptedData = regexp.MustCompile(`(?s)<(?:[\w.-]+:)?EncryptedData\b.*?</(?:[\w.-]+:)?EncryptedData\s*>`)
	cipherURI     = regexp.MustCompile(`\bURI\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	algorithmAttr = regexp.MustCompile(`(\bAlgorithm\s*=\s*["'])[^"']*(["'])`)
)

// rewriteEncryption returns an encryption.xml with the algorithm of each
// entry in algorithms changed to the one it maps to, or the entry dropped
// for "". The rest is kept as written.
func rewriteEncryption(data []byte, algorithms map[string]string) []byte {
	return encryptedData.ReplaceAllFunc(data, func(block []byte) []byte {
		m := cipherURI.FindSubmatch(block)
		if m == nil {
			return block
		}
		algorithm, ok := algorithms[encryptionEntry(string(m[1])+string(m[2]))]
		switch {
		case !ok:
			return block
		case algorithm == "":
			return nil
		}
		return algorithmAttr.ReplaceAll(block, []byte("${1}"+algorithm+"${2}"))
	})
}
