package reflowable

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// The web reader's foliate writes CFIs as epubcfi.js does, which these
// mirror: a step to the spine's itemref, then steps from the content
// document's root. An element is an even step; the text between elements is
// an odd step, and its offset counts UTF-16 code units through all of that
// text, even when a comment or CDATA splits it into several nodes. An
// element with an id carries it as an assertion.

// cfiEscaper escapes what an id assertion can't hold as it is.
var cfiEscaper = strings.NewReplacer("^", "^^", "[", "^[", "]", "^]", "(", "^(", ")", "^)", ",", "^,", ";", "^;", "=", "^=")

// elementStep is the CFI step from n's parent to n.
func elementStep(n *node) string {
	index := 0
	for i, sibling := range n.parent.elements() {
		if sibling == n {
			index = (i + 1) * 2
			break
		}
	}
	step := "/" + strconv.Itoa(index)
	if id := n.id(); id != "" {
		step += "[" + cfiEscaper.Replace(id) + "]"
	}
	return step
}

// CFI is the CFI foliate writes for p, collapsed to a point.
func (b *Book) CFI(p Place) string {
	it, offset := b.resolve(p)
	if it == nil {
		return ""
	}
	spine := b.spineSteps + b.itemSteps[p.Spine]
	if it.body == nil {
		return "epubcfi(" + spine + ")"
	}

	// The text node holding offset, or at the end, the last one.
	var target, last *node
	it.body.walk(func(n *node) {
		if !n.isText() {
			return
		}
		last = n
		if target == nil && n.start <= offset && offset < n.end {
			target = n
		}
	})
	if target == nil {
		target = last
	}
	var steps []string
	if target == nil {
		for el := it.body; el != it.html; el = el.parent {
			steps = append([]string{elementStep(el)}, steps...)
		}
		return "epubcfi(" + spine + "!" + strings.Join(steps, "") + ")"
	}

	// The text step: which stretch of text between elements, and how far
	// into it, counting the nodes before target in the same stretch.
	parent := target.parent
	elementsBefore, units := 0, 0
	for _, sibling := range parent.children {
		if sibling == target {
			break
		}
		if sibling.isText() {
			units += utf16Len(sibling.text)
		} else {
			elementsBefore++
			units = 0
		}
	}
	units += utf16Len(target.text[:offset-target.start])
	steps = append(steps, "/"+strconv.Itoa(elementsBefore*2+1)+":"+strconv.Itoa(units))
	for el := parent; el != it.html; el = el.parent {
		steps = append([]string{elementStep(el)}, steps...)
	}
	return "epubcfi(" + spine + "!" + strings.Join(steps, "") + ")"
}

// cfiStep is one step of a parsed CFI.
type cfiStep struct {
	index  int
	id     string
	offset int
	// hasOffset is set for a step with a character offset.
	hasOffset bool
}

// parseCFI reads a CFI into its paths, one before each indirection, as
// epubcfi.js parses it. A range is read as its start.
func parseCFI(cfi string) ([][]cfiStep, error) {
	cfi = strings.TrimSpace(cfi)
	if !strings.HasPrefix(cfi, "epubcfi(") || !strings.HasSuffix(cfi, ")") {
		return nil, errors.Errorf("%q isn't a CFI", cfi)
	}
	// A range's parent path, then its start, are together where it starts.
	pieces := splitUnescaped(cfi[len("epubcfi("):len(cfi)-1], ',')
	start := pieces[0]
	if len(pieces) > 1 {
		start += pieces[1]
	}

	var paths [][]cfiStep
	for _, part := range splitUnescaped(start, '!') {
		steps, err := parseCFISteps(part)
		if err != nil {
			return nil, err
		}
		paths = append(paths, steps)
	}
	return paths, nil
}

// splitUnescaped splits s at each sep that isn't escaped or in an assertion.
func splitUnescaped(s string, sep rune) []string {
	var out []string
	var current strings.Builder
	escape, inBracket := false, false
	for _, r := range s {
		switch {
		case escape:
			escape = false
		case r == '^':
			escape = true
		case r == '[':
			inBracket = true
		case r == ']':
			inBracket = false
		case r == sep && !inBracket:
			out = append(out, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	return append(out, current.String())
}

// parseCFISteps reads one path: steps of /N, each with an optional [id]
// assertion and :offset, and ignored temporal and spatial offsets.
func parseCFISteps(path string) ([]cfiStep, error) {
	var steps []cfiStep
	runes := []rune(path)
	readNumber := func(i int) (int, int) {
		j := i
		for j < len(runes) && runes[j] >= '0' && runes[j] <= '9' {
			j++
		}
		n, _ := strconv.Atoi(string(runes[i:j]))
		return n, j
	}
	for i := 0; i < len(runes); {
		switch runes[i] {
		case '/':
			n, j := readNumber(i + 1)
			if j == i+1 {
				return nil, errors.Errorf("a CFI step has no index in %q", path)
			}
			steps = append(steps, cfiStep{index: n})
			i = j
		case ':':
			if len(steps) == 0 {
				return nil, errors.Errorf("a CFI offset has no step in %q", path)
			}
			n, j := readNumber(i + 1)
			steps[len(steps)-1].offset, steps[len(steps)-1].hasOffset = n, true
			i = j
		case '[':
			// An assertion: an id after an element step, or text to check,
			// with optional parameters after a semicolon.
			var value strings.Builder
			j := i + 1
			for ; j < len(runes) && runes[j] != ']'; j++ {
				if runes[j] == '^' && j+1 < len(runes) {
					j++
				}
				value.WriteRune(runes[j])
			}
			id, _, _ := strings.Cut(value.String(), ";")
			if len(steps) > 0 && steps[len(steps)-1].index%2 == 0 && !steps[len(steps)-1].hasOffset {
				steps[len(steps)-1].id = id
			}
			i = j + 1
		default:
			// Temporal (~) and spatial (@) offsets mean nothing in text.
			i++
		}
	}
	if len(steps) == 0 {
		return nil, errors.Errorf("a CFI has no steps in %q", path)
	}
	return steps, nil
}

// FromCFI places a CFI foliate wrote. A path that doesn't resolve all the
// way, as in another edition of the book, lands at the start of the deepest
// element it reaches, as an estimate.
func (b *Book) FromCFI(cfi string) (Place, error) {
	paths, err := parseCFI(cfi)
	if err != nil {
		return Place{}, err
	}
	spine := b.spineIndex(paths[0])
	if spine < 0 {
		return Place{}, errors.Wrapf(ErrNotFound, "%s", cfi)
	}
	it := b.items[spine]
	if len(paths) < 2 || it.html == nil {
		return b.place(spine, 0, it.html != nil), nil
	}
	steps := paths[1]

	// An id assertion on the last step finds its element directly.
	if last := steps[len(steps)-1]; last.id != "" {
		if el := it.html.find(func(n *node) bool { return n.id() == last.id }); el != nil {
			return b.place(spine, el.start, true), nil
		}
	}

	current := it.html
	for i, step := range steps {
		chunks := chunkChildren(current)
		if step.index < 0 || step.index >= len(chunks) {
			return b.place(spine, current.start, false), nil
		}
		chunk := chunks[step.index]
		switch {
		case chunk.element != nil:
			if i == len(steps)-1 {
				return b.place(spine, chunk.element.start, true), nil
			}
			current = chunk.element
		case i < len(steps)-1:
			// Text has nothing under it to step to.
			return b.place(spine, chunk.at, false), nil
		default:
			// A point in the text: its offset counts through every node of
			// the stretch.
			units := step.offset
			for _, t := range chunk.text {
				length := utf16Len(t.text)
				if units <= length {
					return b.place(spine, t.start+runesIn(t.text, units), true), nil
				}
				units -= length
			}
			return b.place(spine, chunk.end, !step.hasOffset || units == 0), nil
		}
	}
	return b.place(spine, current.start, true), nil
}

// runesIn is how many code points of text the first units UTF-16 code units
// cover.
func runesIn(text []rune, units int) int {
	for i, r := range text {
		if units <= 0 {
			return i
		}
		units--
		if r >= 0x10000 {
			units--
		}
	}
	return len(text)
}

// chunk is what one CFI step from an element reaches: a child element, or
// the stretch of text between two, which may be empty.
type chunk struct {
	element *node
	text    []*node
	// at is where the chunk starts in the document's text, and end where it
	// ends.
	at, end int
}

// chunkChildren is n's children as CFI steps number them: index 0 before
// everything, then text and elements alternating, starting and ending with
// text, then one after everything.
func chunkChildren(n *node) []chunk {
	chunks := []chunk{{at: n.start, end: n.start}}
	text := chunk{at: n.start, end: n.start}
	for _, c := range n.children {
		if c.isText() {
			text.text = append(text.text, c)
			text.end = c.end
			continue
		}
		chunks = append(chunks, text, chunk{element: c, at: c.start, end: c.end})
		text = chunk{at: c.end, end: c.end}
	}
	return append(chunks, text, chunk{at: n.end, end: n.end})
}

// spineIndex is the spine item the spine path of a CFI names, or -1.
func (b *Book) spineIndex(steps []cfiStep) int {
	if last := steps[len(steps)-1]; last.id != "" {
		for i, itemref := range b.itemrefs {
			if itemref.id() == last.id {
				return i
			}
		}
	}
	current := b.opf
	for _, step := range steps {
		elements := current.elements()
		i := step.index/2 - 1
		if step.index%2 != 0 || i < 0 || i >= len(elements) {
			return -1
		}
		current = elements[i]
	}
	for i, itemref := range b.itemrefs {
		if itemref == current {
			return i
		}
	}
	return -1
}
