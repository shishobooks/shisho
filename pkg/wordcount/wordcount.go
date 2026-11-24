// Package wordcount counts the words in a book's text, for the word count of
// a reflowable ebook.
//
// A word is a run of characters between spaces or dashes that holds a letter
// or digit, so a contraction or hyphenated word counts once and punctuation
// alone counts never. Chinese and Japanese don't put spaces between words, so
// each of their characters counts as one, as word processors count them.
package wordcount

import (
	"io"
	"unicode"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/kepub"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// maxDocumentBytes bounds how much of one document is read, so a crafted
// file can't make the count hold an unbounded document in memory. The
// longest books are tens of megabytes in all.
const maxDocumentBytes = 256 << 20

// Text counts the words in plain text.
func Text(s string) int {
	var c counter
	c.add(s)
	c.boundary()
	return c.n
}

// HTML counts the words in an HTML or XHTML document's text. The head,
// scripts, styles, and ruby annotations aren't text a reader reads, so
// they're skipped. Inline elements such as b and span don't split a word;
// every other element does, as a paragraph or line break would.
//
// The text is read as UTF-8 unless a byte order mark says it's UTF-16. A
// meta charset isn't trusted: books are UTF-8 far more often than their
// markup says.
func HTML(r io.Reader) (int, error) {
	decoded, err := charset.NewReader(r, "text/html; charset=utf-8")
	if err != nil {
		return 0, errors.WithStack(err)
	}
	data, err := io.ReadAll(io.LimitReader(decoded, maxDocumentBytes+1))
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if len(data) > maxDocumentBytes {
		return 0, errors.Errorf("document is larger than %d bytes", maxDocumentBytes)
	}

	var c counter
	skipping := 0
	// The KePub converter's tokenizer setup reads XHTML as a reader does:
	// CDATA is text, and a self-closed script or title ends where it opens.
	err = kepub.EachToken(data, func(z *html.Tokenizer, tt html.TokenType) {
		switch tt {
		case html.TextToken:
			if skipping == 0 {
				c.add(string(z.Text()))
			}
		case html.StartTagToken:
			name, _ := z.TagName()
			if skipped[string(name)] {
				skipping++
			}
			if !inline[string(name)] {
				c.boundary()
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if skipped[string(name)] && skipping > 0 {
				skipping--
			}
			if !inline[string(name)] {
				c.boundary()
			}
		case html.SelfClosingTagToken:
			name, _ := z.TagName()
			if !inline[string(name)] {
				c.boundary()
			}
		case html.ErrorToken, html.CommentToken, html.DoctypeToken:
		}
	})
	if err != nil {
		return 0, err
	}
	c.boundary()
	return c.n, nil
}

// skipped are the elements whose text isn't read: the head and title, code
// run by the page, and the pronunciation ruby prints over its base text.
var skipped = map[string]bool{
	"head":     true,
	"title":    true,
	"script":   true,
	"style":    true,
	"template": true,
	"noscript": true,
	"rt":       true,
	"rp":       true,
}

// inline are the elements that can sit inside a word without splitting it.
var inline = map[string]bool{
	"a": true, "abbr": true, "b": true, "bdi": true, "bdo": true, "big": true,
	"cite": true, "code": true, "data": true, "del": true, "dfn": true,
	"em": true, "font": true, "i": true, "ins": true, "kbd": true,
	"mark": true, "nobr": true, "q": true, "rb": true, "ruby": true, "s": true,
	"samp": true, "small": true, "span": true, "strike": true,
	"strong": true, "sub": true, "sup": true, "time": true, "tt": true,
	"u": true, "var": true, "wbr": true,
}

// counter counts words across the pieces of one text.
type counter struct {
	n int
	// inWord is whether the last character continued a word, and
	// hasLetter whether that word has held a letter or digit yet.
	inWord    bool
	hasLetter bool
}

func (c *counter) add(s string) {
	for _, r := range s {
		switch {
		case countsAlone(r):
			c.boundary()
			c.n++
		case separates(r):
			c.boundary()
		default:
			c.inWord = true
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				c.hasLetter = true
			}
		}
	}
}

// boundary ends the word in progress, if any.
func (c *counter) boundary() {
	if c.inWord && c.hasLetter {
		c.n++
	}
	c.inWord, c.hasLetter = false, false
}

// separates reports whether r falls between words: a space, a zero-width
// space, or a dash other than a hyphen.
func separates(r rune) bool {
	switch r {
	case '\u200b':
		return true
	case '-', '\u2010', '\u2011':
		return false
	}
	return unicode.IsSpace(r) || unicode.Is(unicode.Pd, r)
}

// countsAlone reports whether r is a word by itself: a Chinese character,
// or Japanese kana, including the long vowel mark written with katakana.
func countsAlone(r rune) bool {
	return r == '\u30fc' || unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}
