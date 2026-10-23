package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/ziputil"
	"golang.org/x/net/html"
)

// Strip is a KePub ready to be written as plain EPUB, without the markup
// Kobo's reader uses: the koboSpan wrappers, the book-columns and
// book-inner divs, the kobostylehacks style, kobo.js, and the scripted
// property kobo.js gave the content documents.
type Strip struct {
	entries      []Entry
	opfPath      string
	ncxPath      string
	contentFiles map[string]bool
	// koboJS holds the entries Kobo's script tags load, which are left out.
	koboJS map[string]bool
	// ownScripts holds the content documents that run a script of their
	// own or hold a form, so they stay scripted.
	ownScripts map[string]bool
	// targets holds, for each content document, the span ids a link
	// targets.
	targets map[string]map[string]bool
	// retarget maps each content document's targeted span ids to the id
	// links go to instead.
	retarget map[string]map[string]string
}

// PrepareStrip reads what writing the KePub made of entries as EPUB needs
// to know about the whole book, before any of it is written: which span ids
// links target, and where those links go once the spans are removed. A link
// can come before the document it targets, so the targeted documents are
// stripped once here to learn the ids, and again while writing.
func PrepareStrip(ctx context.Context, entries []Entry) (*Strip, error) {
	s := &Strip{
		entries:      entries,
		contentFiles: map[string]bool{},
		// The KePub writer puts kobo.js at the root of the zip.
		koboJS:     map[string]bool{koboJSPath: true},
		ownScripts: map[string]bool{},
		targets:    map[string]map[string]bool{},
		retarget:   map[string]map[string]string{},
	}
	var opf Entry
	for _, e := range entries {
		if filepath.Ext(e.Name) == ".opf" {
			opf = e
			break
		}
	}
	if opf.Name == "" {
		return nil, errors.New("no OPF file found")
	}
	s.opfPath = opf.Name
	opfData, err := opf.content()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read OPF")
	}
	for _, item := range extractManifestItems(opfData) {
		entry := manifestEntryPath(s.opfPath, item.Href)
		switch {
		case isContentFile(item.MediaType):
			s.contentFiles[entry] = true
		case item.MediaType == "application/x-dtbncx+xml":
			s.ncxPath = entry
		}
	}
	s.collectTargets(s.opfPath, opfData)

	for _, e := range entries {
		if !s.contentFiles[e.Name] && e.Name != s.ncxPath {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "conversion cancelled")
		}
		data, err := e.content()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read %s", e.Name)
		}
		s.collectTargets(e.Name, data)
		if !s.contentFiles[e.Name] {
			continue
		}
		if formPattern.Match(data) {
			s.ownScripts[e.Name] = true
		}
		for _, tag := range scriptTagPattern.FindAll(data, -1) {
			src := html.UnescapeString(extractAttribute(string(tag), "src"))
			if !isKoboScriptSrc(src) {
				s.ownScripts[e.Name] = true
			} else if u, err := url.Parse(src); err == nil && u.Scheme == "" && u.Host == "" {
				s.koboJS[path.Join(path.Dir(e.Name), u.Path)] = true
			}
		}
	}

	for _, e := range entries {
		targets := s.targets[e.Name]
		if targets == nil || !s.contentFiles[e.Name] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "conversion cancelled")
		}
		data, err := e.content()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read %s", e.Name)
		}
		_, retarget, err := stripDocument(data, targets, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to strip %s", e.Name)
		}
		s.retarget[e.Name] = retarget
	}
	return s, nil
}

// collectTargets records the span ids the links in the document at name
// target.
func (s *Strip) collectTargets(name string, data []byte) {
	for _, m := range linkAttrPattern.FindAllSubmatch(data, -1) {
		if doc, id, ok := resolveLink(name, html.UnescapeString(string(m[2])+string(m[3]))); ok {
			if s.targets[doc] == nil {
				s.targets[doc] = map[string]bool{}
			}
			s.targets[doc][id] = true
		}
	}
}

// Write writes the EPUB to w front to back. The mimetype entry goes first;
// the content documents, the OPF, and the NCX are stripped, other replaced
// entries are compressed, kobo.js is left out, and every other entry is
// copied without recompressing.
func (s *Strip) Write(ctx context.Context, w io.Writer) error {
	convert := func(ctx context.Context, e Entry) (*ziputil.Entry, error) {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "conversion cancelled")
		}
		switch {
		case s.contentFiles[e.Name]:
			data, err := e.content()
			if err != nil {
				return nil, errors.Wrapf(err, "failed to strip content file: %s", e.Name)
			}
			out, _, err := stripDocument(data, s.targets[e.Name], s.linkRewriter(e.Name))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to strip content file: %s", e.Name)
			}
			return ziputil.Compress(e.Name, zip.Deflate, out)
		case e.Name == s.opfPath:
			data, err := e.content()
			if err != nil {
				return nil, errors.Wrap(err, "failed to strip OPF file")
			}
			return ziputil.Compress(e.Name, zip.Deflate, s.rewriteLinkAttrs(e.Name, s.stripOPF(data)))
		case e.Name == s.ncxPath:
			data, err := e.content()
			if err != nil {
				return nil, errors.Wrap(err, "failed to strip NCX file")
			}
			return ziputil.Compress(e.Name, zip.Deflate, s.rewriteLinkAttrs(e.Name, data))
		case e.File == nil:
			return ziputil.Compress(e.Name, zip.Deflate, e.Content)
		default:
			return nil, nil
		}
	}

	// The mimetype entry is written first, so any other is dropped, and so
	// is kobo.js.
	var body []Entry
	for _, e := range s.entries {
		if e.Name != "mimetype" && !s.koboJS[e.Name] {
			body = append(body, e)
		}
	}
	lead := []leadEntry{{name: "mimetype", method: zip.Store, content: []byte("application/epub+zip")}}
	return writeZip(ctx, w, lead, body, convert)
}

// linkRewriter returns the function that points a link in the document at
// from away from a removed span: to the id its span's nearest remaining
// element has, or to the document itself when the span was never found.
// Every other link is returned as it was.
func (s *Strip) linkRewriter(from string) func(string) string {
	return func(ref string) string {
		doc, id, ok := resolveLink(from, ref)
		if !ok {
			return ref
		}
		base := ref[:strings.IndexByte(ref, '#')]
		if to, ok := s.retarget[doc][id]; ok {
			return base + "#" + to
		}
		return base
	}
}

// rewriteLinkAttrs runs linkRewriter over every href and src attribute in
// an XML document that isn't parsed, such as the NCX or the OPF's guide.
// The values there are escaped, so each is unescaped to resolve it and
// escaped again.
func (s *Strip) rewriteLinkAttrs(from string, data []byte) []byte {
	rewrite := s.linkRewriter(from)
	return linkAttrPattern.ReplaceAllFunc(data, func(attr []byte) []byte {
		m := linkAttrPattern.FindSubmatch(attr)
		quote, value := `"`, string(m[2])
		if m[2] == nil {
			quote, value = `'`, string(m[3])
		}
		unescaped := html.UnescapeString(value)
		rewritten := rewrite(unescaped)
		if rewritten == unescaped {
			return attr
		}
		return []byte(string(m[1]) + quote + html.EscapeString(rewritten) + quote)
	})
}

// stripOPF drops the kobo.js manifest item and the scripted property of
// content documents that run no script of their own.
func (s *Strip) stripOPF(data []byte) []byte {
	return []byte(manifestItemElementPattern.ReplaceAllStringFunc(string(data), func(item string) string {
		entry := manifestEntryPath(s.opfPath, html.UnescapeString(extractAttribute(item, "href")))
		if s.koboJS[entry] {
			return ""
		}
		if s.contentFiles[entry] && !s.ownScripts[entry] {
			return removeItemProperty(item, "scripted")
		}
		return item
	}))
}

var (
	// koboSpanIDPattern matches the id Kobo's writer gives a span.
	koboSpanIDPattern = regexp.MustCompile(`^kobo\.\d+\.\d+$`)
	// linkAttrPattern matches an href or src attribute, its value in group
	// 2 when double-quoted and group 3 when single-quoted.
	linkAttrPattern = regexp.MustCompile(`(\s(?:href|src)\s*=\s*)(?:"([^"]*)"|'([^']*)')`)
	// scriptTagPattern matches a script element's start tag.
	scriptTagPattern = regexp.MustCompile(`<script\b[^>]*>`)
	// formPattern matches the start tag of a form, which needs the
	// scripted property as a script does.
	formPattern = regexp.MustCompile(`<form\b`)
	// manifestItemElementPattern matches a whole manifest item element,
	// self-closed or not.
	manifestItemElementPattern = regexp.MustCompile(`[ \t]*<item\b[^>]*/>|[ \t]*<item\b[^>]*>\s*</item>`)
)

// resolveLink returns the document and Kobo span id a link in the document
// at from targets, and false for a link to anything else.
func resolveLink(from, ref string) (doc, id string, ok bool) {
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "" || u.Host != "" || !koboSpanIDPattern.MatchString(u.Fragment) {
		return "", "", false
	}
	doc = from
	if u.Path != "" {
		doc = path.Join(path.Dir(from), u.Path)
	}
	return doc, u.Fragment, true
}

// stripDocument removes Kobo's markup from a content document. Each span
// id in targets that it removes maps, in the returned map, to the id of
// the nearest element that remains: the span itself when it keeps other
// attributes, and otherwise its nearest remaining ancestor. That element
// gets an id when it has none. rewriteLink, when set, is applied to the
// href and src of every tag that stays.
//
// It reads the document as a stream of HTML tokens rather than parsing it
// into a tree, and copies every token and attribute it doesn't change as it
// was. Go's HTML parser would restructure markup that is valid XHTML but
// not valid HTML, such as a div inside a p, and would drop a pre's leading
// newline.
func stripDocument(data []byte, targets map[string]bool, rewriteLink func(string) string) ([]byte, map[string]string, error) {
	ids, err := documentIDs(data)
	if err != nil {
		return nil, nil, err
	}

	// open is an element whose end tag is still to come.
	type open struct {
		name   string
		action tagAction
		// kept is the element's start tag in out, when it stays.
		kept *keptTag
	}
	var stack []*open
	openCount := map[string]int{}
	var needID []*keptTag
	retarget := map[string]string{}
	dropping := 0
	var out bytes.Buffer
	out.Grow(len(data))

	err = eachToken(data, func(z *html.Tokenizer, tt html.TokenType) {
		raw := z.Raw()
		//nolint:exhaustive // Every other token is copied as it was.
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, attrs := tokenAttrs(z)
			koboSpan := isKoboSpan(name, attrs)
			action := actionDrop
			if dropping == 0 {
				action, raw = kepubTagAction(name, attrs, raw)
			}
			var kept *keptTag
			if action == actionKeep {
				if rewriteLink != nil {
					raw, _ = editTag(raw, func(attr, value string) (string, bool) {
						if attr == "href" || attr == "src" {
							return rewriteLink(value), true
						}
						return value, true
					})
				}
				kept = &keptTag{start: out.Len(), id: attrs["id"]}
				if koboSpan && koboSpanIDPattern.MatchString(kept.id) {
					kept.id = ""
				}
				out.Write(raw)
				kept.end = out.Len()
			}
			if id := attrs["id"]; koboSpan && dropping == 0 && targets[id] {
				to := kept
				// The nearest element that stays is the innermost open one.
				for i := len(stack) - 1; to == nil && i >= 0; i-- {
					to = stack[i].kept
				}
				if to != nil {
					if to.id == "" {
						to.id = uniqueID(ids, "shisho-"+strings.ReplaceAll(strings.TrimPrefix(id, "kobo."), ".", "-"))
						needID = append(needID, to)
					}
					retarget[id] = to.id
				}
			}
			if tt == html.SelfClosingTagToken || voidElements[name] {
				return
			}
			if action == actionDrop {
				dropping++
			}
			stack = append(stack, &open{name: name, action: action, kept: kept})
			openCount[name]++
		case html.EndTagToken:
			name, _ := z.TagName()
			if openCount[string(name)] == 0 {
				// An end tag with no start tag is copied as it was.
				if dropping == 0 {
					out.Write(raw)
				}
				return
			}
			i := len(stack) - 1
			for stack[i].name != string(name) {
				i--
			}
			closed := stack[i]
			for _, o := range stack[i:] {
				openCount[o.name]--
				if o.action == actionDrop {
					dropping--
				}
			}
			stack = stack[:i]
			if closed.action == actionKeep {
				out.Write(raw)
			}
		default:
			if dropping == 0 {
				out.Write(raw)
			}
		}
	})
	if err != nil {
		return nil, nil, err
	}

	if len(needID) == 0 {
		return out.Bytes(), retarget, nil
	}
	// Give each element that needs one its new id, in place of an empty one.
	result := out.Bytes()
	slices.SortFunc(needID, func(a, b *keptTag) int { return a.start - b.start })
	var b bytes.Buffer
	b.Grow(len(result) + len(needID)*24)
	last := 0
	for _, t := range needID {
		b.Write(result[last:t.start])
		tag, _ := editTag(result[t.start:t.end], func(attr, value string) (string, bool) {
			return value, attr != "id"
		})
		b.Write(insertAttr(tag, ` id="`+html.EscapeString(t.id)+`"`))
		last = t.end
	}
	b.Write(result[last:])
	return b.Bytes(), retarget, nil
}

// keptTag is the start tag of an element that stays, in the output.
type keptTag struct {
	start, end int
	// id is the element's id, or empty when it has none.
	id string
}

// tagAction is what stripping does with an element.
type tagAction int

const (
	// actionKeep copies the element.
	actionKeep tagAction = iota
	// actionUnwrap drops the element's tags and keeps its content.
	actionUnwrap
	// actionDrop drops the element and its content.
	actionDrop
)

// kepubTagAction returns what stripping does with an element of Kobo's
// markup, and for a span that keeps other attributes, its start tag
// without the Kobo ones.
func kepubTagAction(name string, attrs map[string]string, raw []byte) (tagAction, []byte) {
	switch {
	case isKoboScript(name, attrs), isKoboStyleHacks(name, attrs):
		return actionDrop, raw
	case isBookWrapper(name, attrs):
		return actionUnwrap, raw
	case isKoboSpan(name, attrs):
		tag, remaining := editTag(raw, func(attr, value string) (string, bool) {
			switch attr {
			case "id":
				return value, !koboSpanIDPattern.MatchString(value)
			case "class":
				kept := slices.DeleteFunc(strings.Fields(value), func(c string) bool { return c == koboSpanClass })
				return strings.Join(kept, " "), len(kept) > 0
			}
			return value, true
		})
		if remaining == 0 {
			return actionUnwrap, raw
		}
		return actionKeep, tag
	}
	return actionKeep, raw
}

// attrPattern matches one attribute of a start tag with the space before
// it, as the HTML tokenizer reads it: the name in group 1, and the value,
// if any, in group 2 when double-quoted, 3 when single-quoted, or 4 when
// unquoted.
var attrPattern = regexp.MustCompile(`^\s+([^\s"'>/=]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+)))?`)

// editTag runs edit over each attribute of the start tag raw, passing the
// lowercased name and unescaped value, and returns the tag with each
// attribute edit drops left out and each value it changes rewritten, along
// with how many attributes remain. Every other byte is kept as it was.
func editTag(raw []byte, edit func(name, value string) (string, bool)) ([]byte, int) {
	pos := 1
	for pos < len(raw) && !strings.ContainsRune(" \t\n\r\f/>", rune(raw[pos])) {
		pos++
	}
	var out []byte
	out = append(out, raw[:pos]...)
	remaining := 0
	for {
		m := attrPattern.FindSubmatchIndex(raw[pos:])
		if m == nil {
			break
		}
		attr := raw[pos : pos+m[1]]
		name := strings.ToLower(string(raw[pos+m[2] : pos+m[3]]))
		value := ""
		for g := 4; g <= 8; g += 2 {
			if m[g] >= 0 {
				value = html.UnescapeString(string(raw[pos+m[g] : pos+m[g+1]]))
			}
		}
		pos += m[1]
		edited, keep := edit(name, value)
		if !keep {
			continue
		}
		remaining++
		if edited == value {
			out = append(out, attr...)
			continue
		}
		quote := `"`
		if m[6] >= 0 {
			quote = `'`
		}
		out = append(out, ' ')
		out = append(out, raw[pos-m[1]+m[2]:pos-m[1]+m[3]]...)
		out = append(out, "="+quote+html.EscapeString(edited)+quote...)
	}
	return append(out, raw[pos:]...), remaining
}

// insertAttr adds attr, with its leading space, to the end of the start tag
// raw.
func insertAttr(raw []byte, attr string) []byte {
	end := len(raw) - 1
	if bytes.HasSuffix(raw, []byte("/>")) {
		end--
	}
	for end > 0 && strings.ContainsRune(" \t\n\r\f", rune(raw[end-1])) {
		end--
	}
	out := append([]byte{}, raw[:end]...)
	out = append(out, attr...)
	return append(out, raw[end:]...)
}

// documentIDs returns every id in a document, so a new one can't clash.
func documentIDs(data []byte) (map[string]bool, error) {
	ids := map[string]bool{}
	err := eachToken(data, func(z *html.Tokenizer, tt html.TokenType) {
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			return
		}
		if _, attrs := tokenAttrs(z); attrs["id"] != "" {
			ids[attrs["id"]] = true
		}
	})
	return ids, err
}

// uniqueID returns base, or base with a number added when ids has it, and
// adds what it returns to ids.
func uniqueID(ids map[string]bool, base string) string {
	id := base
	for i := 2; ids[id]; i++ {
		id = base + "-" + strconv.Itoa(i)
	}
	ids[id] = true
	return id
}
