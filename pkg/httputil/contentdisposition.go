// Package httputil provides small HTTP helpers shared across handlers.
package httputil

import (
	"fmt"
	"net/http"
	"strings"
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
	fallback := asciiFallback(filename)
	header := `attachment; filename="` + escapeQuoted(fallback) + `"`
	if needsExtendedForm(filename) {
		header += "; filename*=UTF-8''" + percentEncode(filename)
	}
	w.Header().Set("Content-Disposition", header)
}

// asciiFallback strips runes outside printable ASCII and tidies the result so
// it is safe to drop into a quoted-string. Invalid UTF-8 decodes to U+FFFD
// which is also out of range and gets stripped. Returns "download" if nothing
// usable remains, and prepends "download" to results that would start with a
// dot (e.g. "тест.epub" -> ".epub" -> "download.epub").
func asciiFallback(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r <= 0x7E {
			b.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(b.String())
	for strings.Contains(cleaned, "  ") {
		cleaned = strings.ReplaceAll(cleaned, "  ", " ")
	}
	if cleaned == "" {
		return "download"
	}
	if strings.HasPrefix(cleaned, ".") {
		return "download" + cleaned
	}
	return cleaned
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
