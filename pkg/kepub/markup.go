package kepub

import (
	"bytes"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/net/html"
)

// The ids and class Kobo's markup is recognized by. The converter writes
// the markup from them, and the predicates below recognize it, both to
// leave out what a document already has and to strip it, so writing,
// detecting, and removing stay in step.
const (
	koboSpanClass    = "koboSpan"
	koboStyleHacksID = "kobostylehacks"
	bookColumnsID    = "book-columns"
	bookInnerID      = "book-inner"
)

// Kobo's markup that transformDocument adds.
const (
	koboStyleHacks     = `<style type="text/css" id="` + koboStyleHacksID + `">div#` + bookInnerID + ` { margin-top: 0; margin-bottom: 0; }</style>`
	bookWrappersStart  = `<div id="` + bookColumnsID + `"><div id="` + bookInnerID + `">`
	bookWrappersEnd    = `</div></div>`
	koboSpanStart      = `<span class="` + koboSpanClass + `" id="`
	koboSpanStartClose = `">`
	koboSpanEnd        = `</span>`
)

// koboScriptTag is the script tag that loads kobo.js from scriptPath.
func koboScriptTag(scriptPath string) string {
	return `<script type="text/javascript" src="` + html.EscapeString(scriptPath) + `"/>`
}

// IsKoboSpan reports whether a tag is one of the spans a KePub wraps text
// in.
func IsKoboSpan(name string, attrs map[string]string) bool {
	return name == "span" && slices.Contains(strings.Fields(attrs["class"]), koboSpanClass)
}

func isKoboScript(name string, attrs map[string]string) bool {
	return name == "script" && isKoboScriptSrc(attrs["src"])
}

// isKoboScriptSrc reports whether a script's unescaped src loads kobo.js.
func isKoboScriptSrc(src string) bool {
	return src != "" && path.Base(src) == koboJSPath
}

func isKoboStyleHacks(name string, attrs map[string]string) bool {
	return name == "style" && attrs["id"] == koboStyleHacksID
}

// IsBookWrapper reports whether a tag is either of the book-columns and
// book-inner divs a KePub wraps the body's content in.
func IsBookWrapper(name string, attrs map[string]string) bool {
	return name == "div" && (attrs["id"] == bookColumnsID || attrs["id"] == bookInnerID)
}

// voidElements are the HTML elements that never have an end tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "param": true, "source": true, "track": true, "wbr": true,
}

// IsVoidElement reports whether name is an HTML element that never has an
// end tag, so a start tag without the self-closing slash still holds
// nothing.
func IsVoidElement(name string) bool {
	return voidElements[name]
}

// EachToken calls visit with each token of a content document in turn,
// along with the tokenizer to read the token from, and returns the error
// that stopped it before the end, if any. Every content rewrite reads its
// document through it, as does anything that must read a document the way
// the rewrites do. It sets up the tokenizer as XHTML needs:
//
//   - CDATA sections are read as text. XHTML allows them anywhere, and
//     without AllowCDATA one holding a > ends as a comment at it, leaving
//     the rest as markup.
//   - A self-closed tag never starts raw text. Without NextIsNotRawText, a
//     self-closed script or title swallows what follows as its text.
func EachToken(data []byte, visit func(z *html.Tokenizer, tt html.TokenType)) error {
	z := html.NewTokenizer(bytes.NewReader(data))
	z.AllowCDATA(true)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				return nil
			}
			return errors.WithStack(z.Err())
		}
		if tt == html.SelfClosingTagToken {
			z.NextIsNotRawText()
		}
		visit(z, tt)
	}
}

// TokenAttrs returns the current tag token's name and attributes.
func TokenAttrs(z *html.Tokenizer) (string, map[string]string) {
	name, more := z.TagName()
	attrs := map[string]string{}
	for more {
		var key, val []byte
		key, val, more = z.TagAttr()
		if _, ok := attrs[string(key)]; !ok {
			attrs[string(key)] = string(val)
		}
	}
	return string(name), attrs
}
