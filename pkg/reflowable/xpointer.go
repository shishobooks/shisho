package reflowable

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// KOReader's crengine writes an XPointer as a path through its own DOM of the
// whole book: /body/DocFragment[N]/body/... for the Nth spine item, linear or
// not, then each element by name, with its 1-based index among same-named
// siblings when there's more than one, ending in text()[i].offset.
//
// Its DOM differs from the browser's in the text, which these mirror, as the
// KOReader harness showed:
//   - Whitespace-only text right after a block element, or first in one, is
//     dropped. Between inline elements, it stays.
//   - Within a text node, each run of whitespace is one space, except in pre.
//   - A comment or CDATA ends a text node, and CDATA is one of its own.
//   - Offsets count code points.

// blockElements are the elements crengine lays out as blocks by default,
// after which whitespace-only text is dropped.
var blockElements = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "body": true, "caption": true,
	"center": true, "col": true, "colgroup": true, "dd": true, "details": true, "dialog": true, "dir": true,
	"div": true, "dl": true, "dt": true, "fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "header": true,
	"hgroup": true, "hr": true, "html": true, "legend": true, "li": true, "main": true, "menu": true,
	"nav": true, "ol": true, "p": true, "pre": true, "section": true, "summary": true, "table": true,
	"tbody": true, "td": true, "tfoot": true, "th": true, "thead": true, "tr": true, "ul": true,
}

func isXMLSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }

// preformatted reports whether n is in a pre, where crengine keeps
// whitespace as it is.
func preformatted(n *node) bool {
	for p := n.parent; p != nil; p = p.parent {
		if p.name == "pre" {
			return true
		}
	}
	return false
}

// keptChildren is n's children as crengine's DOM holds them, without the
// whitespace-only text it drops.
func keptChildren(n *node) []*node {
	var kept []*node
	for _, c := range n.children {
		if c.isText() && !preformatted(c) && whitespaceOnly(c.text) {
			previous := n
			if len(kept) > 0 {
				previous = kept[len(kept)-1]
			}
			if blockElements[previous.name] {
				continue
			}
		}
		kept = append(kept, c)
	}
	return kept
}

func whitespaceOnly(text []rune) bool {
	for _, r := range text {
		if !isXMLSpace(r) {
			return false
		}
	}
	return true
}

// collapsedOffset is how far into t crengine counts raw code points in:
// each run of whitespace is one character.
func collapsedOffset(t *node, raw int) int {
	if preformatted(t) {
		return raw
	}
	collapsed := 0
	space := false
	for _, r := range t.text[:raw] {
		if isXMLSpace(r) {
			if !space {
				collapsed++
			}
			space = true
			continue
		}
		space = false
		collapsed++
	}
	return collapsed
}

// rawOffset is the code point in t that crengine's offset collapsed names:
// the one that makes that character, or the end. ok is false for an offset
// past the end.
func rawOffset(t *node, collapsed int) (int, bool) {
	if preformatted(t) {
		return min(collapsed, len(t.text)), collapsed <= len(t.text)
	}
	count := 0
	space := false
	for i, r := range t.text {
		if isXMLSpace(r) {
			if space {
				continue
			}
			space = true
		} else {
			space = false
		}
		if count == collapsed {
			return i, true
		}
		count++
	}
	return len(t.text), collapsed == count
}

// xpointerStep is the step from parent to child in crengine's DOM.
func xpointerStep(parent, child *node) string {
	name, kind := child.name, func(n *node) bool { return n.name == child.name }
	if child.isText() {
		name, kind = "text()", (*node).isText
	}
	index, count := 0, 0
	for _, sibling := range keptChildren(parent) {
		if kind(sibling) {
			count++
			if sibling == child {
				index = count
			}
		}
	}
	if count > 1 {
		return name + "[" + strconv.Itoa(index) + "]"
	}
	return name
}

// XPointer is the XPointer crengine writes for p.
func (b *Book) XPointer(p Place) string {
	it, offset := b.resolve(p)
	if it == nil {
		return ""
	}
	fragment := "DocFragment"
	if len(b.items) > 1 {
		fragment += "[" + strconv.Itoa(p.Spine+1) + "]"
	}
	prefix := "/body/" + fragment + "/body"
	if it.body == nil {
		return prefix
	}

	// The text node crengine has at offset: the one holding it, or past
	// dropped whitespace, the next, or at the end, the last.
	var target, last *node
	var visit func(n *node)
	visit = func(n *node) {
		for _, c := range keptChildren(n) {
			if target != nil {
				return
			}
			if !c.isText() {
				visit(c)
				continue
			}
			last = c
			if offset < c.end {
				target = c
			}
		}
	}
	visit(it.body)
	if target == nil {
		target = last
	}
	if target == nil {
		return prefix
	}
	raw := max(0, min(offset-target.start, len(target.text)))
	steps := []string{xpointerStep(target.parent, target) + "." + strconv.Itoa(collapsedOffset(target, raw))}
	for el := target.parent; el != it.body; el = el.parent {
		steps = append([]string{xpointerStep(el.parent, el)}, steps...)
	}
	return prefix + "/" + strings.Join(steps, "/")
}

// xpointerPart is one parsed step: a name, its 1-based index, or 0 for
// none, and an offset after it.
type xpointerPart struct {
	name      string
	index     int
	offset    int
	hasOffset bool
}

func parseXPointer(xp string) ([]xpointerPart, error) {
	if !strings.HasPrefix(xp, "/") {
		return nil, errors.Errorf("%q isn't an XPointer", xp)
	}
	var parts []xpointerPart
	for _, step := range strings.Split(xp[1:], "/") {
		part := xpointerPart{}
		// The offset follows the last step, after a dot that isn't in the
		// step's name.
		if dot := strings.LastIndex(step, "."); dot > strings.LastIndex(step, "]") && dot > strings.LastIndex(step, ")") {
			offset, err := strconv.Atoi(step[dot+1:])
			if err != nil {
				return nil, errors.Errorf("bad offset in %q", xp)
			}
			part.offset, part.hasOffset = offset, true
			step = step[:dot]
		}
		if open := strings.LastIndex(step, "["); open > 0 && strings.HasSuffix(step, "]") {
			index, err := strconv.Atoi(step[open+1 : len(step)-1])
			if err != nil || index < 1 {
				return nil, errors.Errorf("bad index in %q", xp)
			}
			part.index = index
			step = step[:open]
		}
		if step == "" {
			return nil, errors.Errorf("empty step in %q", xp)
		}
		part.name = step
		parts = append(parts, part)
	}
	return parts, nil
}

// FromXPointer places an XPointer crengine wrote. A path that doesn't
// resolve all the way, as in another edition of the book, lands at the start
// of the deepest element it reaches, and an offset past its text at the
// text's end, as an estimate.
func (b *Book) FromXPointer(xp string) (Place, error) {
	parts, err := parseXPointer(xp)
	if err != nil {
		return Place{}, err
	}
	if len(parts) < 2 || parts[0].name != "body" || parts[1].name != "DocFragment" {
		return Place{}, errors.Wrapf(ErrNotFound, "%s", xp)
	}
	spine := max(parts[1].index, 1) - 1
	if spine >= len(b.items) {
		return Place{}, errors.Wrapf(ErrNotFound, "%s", xp)
	}
	it := b.items[spine]
	if it.body == nil {
		return b.place(spine, 0, false), nil
	}
	current := it.body
	steps := parts[2:]
	if len(steps) > 0 && steps[0].name == "body" {
		steps = steps[1:]
	} else if len(steps) > 0 {
		return b.place(spine, 0, false), nil
	}
	for i, step := range steps {
		want := max(step.index, 1)
		var found *node
		count := 0
		for _, c := range keptChildren(current) {
			if step.name == "text()" && c.isText() || !c.isText() && c.name == step.name {
				count++
				if count == want {
					found = c
					break
				}
			}
		}
		if found == nil {
			return b.place(spine, current.start, false), nil
		}
		if found.isText() {
			if i != len(steps)-1 {
				return b.place(spine, found.start, false), nil
			}
			raw, ok := rawOffset(found, step.offset)
			return b.place(spine, found.start+raw, ok), nil
		}
		current = found
	}
	return b.place(spine, current.start, true), nil
}
