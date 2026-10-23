// Package filenames makes text safe to use in the names of downloaded
// files. Organized names on disk follow pkg/fileutils' own rules instead.
package filenames

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// unsafeFilenameChars are dropped from every filename Clean makes:
//   - / \ : * ? " < > | are invalid in Windows, macOS, or Linux filenames.
//   - [ ] { } are shell glob characters, and # starts a URL fragment and a
//     shell comment.
//   - $ and ` expand even inside double quotes in a shell, and % starts a URL
//     escape and a Windows cmd variable.
//
// Other punctuation, such as & ! ( ) ' and commas, is kept: it is safe once
// the name is quoted, and titles use it.
const unsafeFilenameChars = `/\:*?"<>|[]{}#$%` + "`"

// letterSpellings spells out Latin letters that have no decomposition, so
// transliterateLatin cannot reach a base letter for them. A capital spells
// out in title case (Æsir to Aesir, Þór to Thor), except the capital sharp s,
// which appears only in all-capital text.
var letterSpellings = map[rune]string{
	'ß': "ss", 'ẞ': "SS",
	'æ': "ae", 'Æ': "Ae",
	'œ': "oe", 'Œ': "Oe",
	'ø': "o", 'Ø': "O",
	'ł': "l", 'Ł': "L",
	'đ': "d", 'Đ': "D",
	'ð': "d", 'Ð': "D",
	'þ': "th", 'Þ': "Th",
	'ı': "i",
}

// isHiddenFormatRune reports the invisible format characters a filename
// drops: the bidirectional controls, which can make a name display in a
// misleading order, and the byte order mark. Joiners stay, because some
// scripts need them.
func isHiddenFormatRune(r rune) bool {
	switch {
	case r == '\u200E', r == '\u200F', r == '\uFEFF':
		return true
	case r >= '\u202A' && r <= '\u202E':
		return true
	case r >= '\u2066' && r <= '\u2069':
		return true
	}
	return false
}

// transliterateLatin turns accented Latin letters into plain ones (é to e,
// ñ to n, ß to ss). It decomposes the text, drops the combining marks that
// follow a Latin letter, and recomposes, so the marks of other scripts,
// which Devanagari, Thai, and Japanese kana depend on, are kept.
func transliterateLatin(s string) string {
	var b strings.Builder
	for _, r := range s {
		if spelled, ok := letterSpellings[r]; ok {
			b.WriteString(spelled)
		} else {
			b.WriteRune(r)
		}
	}

	decomposed := norm.NFD.String(b.String())
	b.Reset()
	latinBase := false
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			if !latinBase {
				b.WriteRune(r)
			}
			continue
		}
		latinBase = unicode.Is(unicode.Latin, r)
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// collapseSpaces trims s and collapses each run of whitespace to one space.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Clean makes s safe as part of any download filename. It
// transliterates accented Latin letters, turns every whitespace character
// into a space, drops control characters, hidden format characters, and
// unsafeFilenameChars, and collapses the spaces. Letters, digits, and marks
// of every other script are kept.
func Clean(s string) string {
	var b strings.Builder
	for _, r := range transliterateLatin(s) {
		switch {
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		case unicode.IsControl(r), isHiddenFormatRune(r), strings.ContainsRune(unsafeFilenameChars, r):
		default:
			b.WriteRune(r)
		}
	}
	return collapseSpaces(b.String())
}

// koboUnsafeChars contains characters that cause problems with Kobo e-readers.
// Based on https://github.com/kobolabs/epub-spec which states:
// "File names containing non-alphanumeric characters are not fully supported,
// and their use may lead to undefined behaviour."
// Additionally, colons are known to prevent Kobo from seeing kepub files:
// https://github.com/seblucas/cops/issues/263
var koboUnsafeChars = regexp.MustCompile(`[^a-zA-Z0-9 \-_.,()']`)

// koboApostrophes turns typographic apostrophes into plain ones, which the
// Kobo-safe set keeps, so "Harry’s" does not become "Harrys".
var koboApostrophes = strings.NewReplacer("\u2018", "'", "\u2019", "'")

// CleanASCII makes s safe as part of a filename that must be ASCII: a
// KePub's, and the plain name sent beside every other download's UTF-8
// one. After Clean, it keeps only the Kobo-safe ASCII characters, so
// accented Latin text stays readable and other scripts are dropped.
func CleanASCII(s string) string {
	s = koboApostrophes.Replace(Clean(s))
	return collapseSpaces(koboUnsafeChars.ReplaceAllString(s, ""))
}
