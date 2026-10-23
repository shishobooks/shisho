// Package httputil provides small HTTP helpers shared across handlers.
package httputil

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/shishobooks/shisho/pkg/filenames"
)

// SetAttachmentFilename writes a Content-Disposition: attachment header that
// works across the spectrum of HTTP clients per RFC 6266 / RFC 5987.
//
// For ASCII-only filenames, only the legacy quoted form is emitted. When the
// filename contains anything outside printable ASCII, both a sanitized ASCII
// fallback (filename=) and a percent-encoded UTF-8 form (filename*=) are
// emitted; per RFC 6266 §4.3 modern clients prefer filename* while legacy
// clients ignore the unknown parameter and use the ASCII fallback.
//
// This matters for OPDS clients like KOReader's "Use server filenames" mode,
// which parses the header to name the downloaded file on the device.
func SetAttachmentFilename(w http.ResponseWriter, filename string) {
	SetAttachmentFilenames(w, filename, "")
}

// SetAttachmentFilenames is SetAttachmentFilename with the ASCII fallback
// given, as a generated download builds it from the same parts as its name
// (downloadcache.DownloadFilenames). An empty ascii derives it from
// filename.
func SetAttachmentFilenames(w http.ResponseWriter, filename, ascii string) {
	var fallback string
	switch {
	case !needsExtendedForm(filename):
		// A name already in printable ASCII is sent as it is.
		fallback = guardFallback(strings.TrimSpace(filename))
	case ascii != "" && !needsExtendedForm(ascii):
		fallback = guardFallback(ascii)
	default:
		fallback = asciiFallback(filename)
	}
	header := `attachment; filename="` + escapeQuoted(fallback) + `"`
	if needsExtendedForm(filename) {
		header += "; filename*=UTF-8''" + percentEncode(filename)
	}
	w.Header().Set("Content-Disposition", header)
}

// asciiFallback makes the ASCII name for filename with the rules every ASCII
// download name follows (filenames.CleanASCII): accented letters become
// plain ones and everything else outside the safe set is dropped. Invalid
// UTF-8 decodes to U+FFFD, which is dropped too. The name is cleaned
// without its extension, and the separators a dropped part leaves at the
// end are trimmed, so "Author - 吾輩.epub" becomes "Author.epub".
func asciiFallback(s string) string {
	stem, ext := splitExtension(s)
	stem = strings.Trim(filenames.CleanASCII(stem), " -_.,")
	return guardFallback(stem + filenames.CleanASCII(ext))
}

// splitExtension splits a filename's extension off, keeping a KePub's
// ".kepub.epub" whole.
func splitExtension(s string) (stem, ext string) {
	if strings.HasSuffix(strings.ToLower(s), kepubExtension) {
		return s[:len(s)-len(kepubExtension)], s[len(s)-len(kepubExtension):]
	}
	ext = filepath.Ext(s)
	return strings.TrimSuffix(s, ext), ext
}

// kepubExtension is a KePub's two-part extension.
const kepubExtension = ".kepub.epub"

// guardFallback returns "download" for an empty name and prepends it to a
// name that would start with a dot and so save as a hidden file.
func guardFallback(s string) string {
	if s == "" {
		return "download"
	}
	if strings.HasPrefix(s, ".") {
		return "download" + s
	}
	return s
}

// escapeQuoted escapes backslashes and double quotes for use inside an
// HTTP quoted-string (RFC 7230 §3.2.6).
func escapeQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// needsExtendedForm reports whether the filename contains any byte that
// can't be represented losslessly in a printable-ASCII quoted-string.
// Iterates runes, so invalid UTF-8 bytes (decoded as U+FFFD) also trigger
// the extended form, matching the byte-level encoding done by percentEncode.
func needsExtendedForm(s string) bool {
	for _, r := range s {
		if r < 0x20 || r > 0x7E {
			return true
		}
	}
	return false
}

// percentEncode encodes a UTF-8 string per RFC 5987 attr-char rules: keep
// alphanumerics and !#$&+-.^_`|~, percent-encode every other byte uppercase.
func percentEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isAttrChar(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isAttrChar(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '!', '#', '$', '&', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}
