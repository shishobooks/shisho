// Package wordcount counts the words in a book's text, for the word count of
// a reflowable ebook.
//
// A word is a run of characters between spaces or dashes that holds a letter
// or digit, so a contraction or hyphenated word counts once and punctuation
// alone counts never. Chinese and Japanese don't put spaces between words, so
// each of their characters counts as one, as word processors count them.
package wordcount

import (
	"context"
	"io"
	"io/fs"
	"unicode"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/kepub"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// maxDocumentBytes bounds how much of one document is read, so a crafted
// file can't make the count hold an unbounded document in memory. A whole
// long book in one document is a few megabytes.
const maxDocumentBytes = 64 << 20

// Retryable reports whether a count that failed with err might succeed if
// tried again on the same file: the file couldn't be read, as when a share
// drops out or a permission is fixed, or the count was canceled. Any other
// failure is the content's, and trying again changes nothing.
func Retryable(err error) bool {
	var pathErr *fs.PathError
	return errors.As(err, &pathErr) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

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
	var skip skipState
	// The KePub converter's tokenizer setup reads XHTML as a reader does:
	// CDATA is text, and a self-closed script or title ends where it opens.
	err = kepub.EachToken(data, func(z *html.Tokenizer, tt html.TokenType) {
		switch tt {
		case html.TextToken:
			if !skip.skipping() {
				c.add(string(z.Text()))
			}
		case html.StartTagToken:
			name, _ := z.TagName()
			skip.start(string(name))
			if !inline[string(name)] {
				c.boundary()
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			skip.end(string(name))
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

// skipState tracks whether text is in an element a reader doesn't read: the
// head, code run by the page, or the pronunciation ruby prints over its base
// text. HTML lets a page leave out the end tags of head, rt, and rp, so a
// head also ends at the first element that can't be in it, and an rt or rp
// at the next one, its ruby's end, or the next block.
type skipState struct {
	// raw counts the open elements whose content the tokenizer reads as raw
	// text, which it always ends at their end tag.
	raw        int
	head       bool
	annotation bool
}

// rawSkipped are the skipped elements other than head, rt, and rp.
var rawSkipped = map[string]bool{
	"title":    true,
	"script":   true,
	"style":    true,
	"template": true,
	"noscript": true,
}

func (s *skipState) skipping() bool { return s.raw > 0 || s.head || s.annotation }

func (s *skipState) start(name string) {
	if s.head && !headContent[name] {
		s.head = false
	}
	switch name {
	case "head":
		s.head = true
	case "rt", "rp":
		s.annotation = true
	default:
		if rawSkipped[name] {
			s.raw++
		}
		if block[name] {
			s.annotation = false
		}
	}
}

func (s *skipState) end(name string) {
	switch name {
	case "head":
		s.head = false
	case "rt", "rp", "ruby":
		s.annotation = false
	default:
		if rawSkipped[name] && s.raw > 0 {
			s.raw--
		}
		if block[name] {
			s.annotation = false
		}
	}
}

// headContent are the elements a head holds; any other starts the body.
var headContent = map[string]bool{
	"head": true, "title": true, "meta": true, "link": true, "style": true,
	"script": true, "base": true, "noscript": true, "template": true,
}

// block are the elements that can't sit in a ruby, so one starting or
// ending closes an rt or rp left open.
var block = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"body": true, "dd": true, "div": true, "dl": true, "dt": true,
	"figcaption": true, "figure": true, "footer": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "header": true, "hr": true,
	"li": true, "main": true, "nav": true, "ol": true, "p": true, "pre": true,
	"section": true, "table": true, "td": true, "th": true, "tr": true, "ul": true,
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
