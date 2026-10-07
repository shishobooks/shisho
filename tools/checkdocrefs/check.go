package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// tokenKind is what a backticked token looks like.
type tokenKind int

const (
	kindSkip tokenKind = iota
	kindPath
	kindIdent
)

// Finding is one stale reference.
type Finding struct {
	Path   string
	Line   int
	Token  string
	Reason string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.Path, f.Line, f.Token, f.Reason)
}

// checkedExts are the extensions that make a token a repo path to look up.
var checkedExts = map[string]bool{
	"go": true, "ts": true, "tsx": true, "js": true, "jsx": true, "mjs": true, "cjs": true,
	"md": true, "mdx": true, "yml": true, "yaml": true, "toml": true, "sh": true,
	"json": true, "sql": true, "css": true, "html": true,
}

// dataExts mark a dotted token as a file name that is not a repo path (a
// file inside an archive, a library file, a domain), so it is neither looked
// up as a path nor as an identifier.
var dataExts = map[string]bool{
	"xml": true, "opf": true, "ncx": true, "xhtml": true, "epub": true, "kepub": true,
	"cbz": true, "cbr": true, "m4b": true, "m4a": true, "mp3": true, "mp4": true, "pdf": true,
	"jpg": true, "jpeg": true, "png": true, "webp": true, "gif": true, "svg": true, "ico": true,
	"sqlite": true, "db": true, "log": true, "txt": true, "env": true, "lock": true, "zip": true,
	"out": true, "com": true, "org": true, "io": true, "dev": true, "local": true,
}

// ignoredPrefixes are gitignored output directories that docs name on purpose.
var ignoredPrefixes = []string{
	"tmp/", "build/", "node_modules/", "app/types/generated/", "website/build/",
	"website/.docusaurus/", ".gotest-timings", ".git/",
}

// routePrefixes are URL paths served by the app, not repo paths.
var routePrefixes = []string{"api/", "opds", "kobo", "ereader", "e/", "health"}

var (
	identRe     = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
	callRe      = regexp.MustCompile(`^([A-Za-z_$][A-Za-z0-9_$.]*)\(.*\)$`)
	lineSuffix  = regexp.MustCompile(`:\d+(-\d+)?$`)
	innerUpper  = regexp.MustCompile(`[a-z0-9][A-Z]`)
	allCaps     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	snakeLower  = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)+$`)
	codeSpanRe  = regexp.MustCompile("`([^`]+)`")
	linkRe      = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	sectionRe   = regexp.MustCompile("[\"“]([^\"“”]{2,100})[\"”](?:\\s+section)?\\s+(?:in|of)\\s+(?:the\\s+)?`?([A-Za-z0-9_./-]+\\.md)`?")
	headingRe   = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)
	boldLeadRe  = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+\.\s+)?\*\*(.+?)\*\*`)
	errNotFound = errors.New("not found")
)

// classify decides whether a backticked token is a repo path, a code
// identifier, or neither, and returns the form to look up.
func classify(token string) (tokenKind, string) {
	tok := strings.TrimSpace(token)
	if tok == "" {
		return kindSkip, ""
	}
	if strings.ContainsAny(tok, "<>{}*\"'`=?&|!") || strings.Contains(tok, "...") && !callRe.MatchString(tok) {
		return kindSkip, ""
	}
	if strings.Contains(tok, "://") || strings.HasPrefix(tok, "~") || strings.HasPrefix(tok, "$") || strings.HasPrefix(tok, "-") {
		return kindSkip, ""
	}
	if m := callRe.FindStringSubmatch(tok); m != nil {
		callee := m[1]
		if !dottedIdent(callee) || allCaps.MatchString(callee) && !strings.Contains(callee, "_") {
			return kindSkip, ""
		}
		return kindIdent, callee
	}
	if strings.ContainsAny(tok, " \t()[],;") {
		return kindSkip, ""
	}
	if looksLikePath(tok) {
		return classifyPath(tok)
	}
	if strings.Contains(tok, ".") {
		segs := strings.Split(tok, ".")
		if dataExts[segs[len(segs)-1]] || !dottedIdent(tok) {
			return kindSkip, ""
		}
		return kindIdent, tok
	}
	if !identRe.MatchString(tok) {
		return kindSkip, ""
	}
	switch {
	case allCaps.MatchString(tok):
		if strings.Contains(tok, "_") {
			return kindIdent, tok
		}
		return kindSkip, ""
	case innerUpper.MatchString(tok), snakeLower.MatchString(tok):
		return kindIdent, tok
	}
	return kindSkip, ""
}

func dottedIdent(s string) bool {
	for seg := range strings.SplitSeq(s, ".") {
		if !identRe.MatchString(seg) {
			return false
		}
	}
	return true
}

func looksLikePath(tok string) bool {
	if strings.Contains(tok, "/") {
		return true
	}
	return checkedExts[ext(lineSuffix.ReplaceAllString(tok, ""))]
}

func ext(p string) string {
	base := path.Base(p)
	i := strings.LastIndex(base, ".")
	if i <= 0 {
		return ""
	}
	return base[i+1:]
}

func classifyPath(tok string) (tokenKind, string) {
	if strings.HasPrefix(tok, "/") || strings.Contains(tok, ":") && !lineSuffix.MatchString(tok) {
		return kindSkip, ""
	}
	p := lineSuffix.ReplaceAllString(tok, "")
	if i := strings.Index(p, "#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimPrefix(p, "./")
	for _, pre := range routePrefixes {
		if strings.HasPrefix(p, pre) && strings.HasSuffix(pre, "/") {
			return kindSkip, ""
		}
	}
	for _, pre := range ignoredPrefixes {
		if strings.HasPrefix(p+"/", pre) || strings.HasPrefix(p, pre) {
			return kindSkip, ""
		}
	}
	p = strings.TrimSuffix(p, "/")
	if p == "" || p == "." {
		return kindSkip, ""
	}
	return kindPath, p
}

// Repo indexes the tracked tree: which files and directories exist and which
// words appear in source (every tracked file except Markdown and this tool).
type Repo struct {
	fsys     fs.FS
	files    map[string]bool
	dirs     map[string]bool
	words    map[string]bool
	headings map[string]map[string]bool
	// ignored reports whether a missing path is gitignored, so build output
	// and CI checkouts a doc names on purpose are not flagged. Nil means none.
	ignored func(p string) bool
}

// NewRepo indexes paths (relative to fsys) that exist on disk.
func NewRepo(fsys fs.FS, paths []string) (*Repo, error) {
	r := &Repo{
		fsys:     fsys,
		files:    map[string]bool{},
		dirs:     map[string]bool{".": true},
		words:    map[string]bool{},
		headings: map[string]map[string]bool{},
	}
	for _, p := range paths {
		info, err := fs.Stat(fsys, p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // Deleted in the working tree.
			}
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if info.IsDir() {
			continue // Submodule or similar.
		}
		r.files[p] = true
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			r.dirs[d] = true
		}
		if isMarkdown(p) || strings.HasPrefix(p, "tools/checkdocrefs/") || info.Size() > 4<<20 {
			continue
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			continue // Binary.
		}
		addWords(r.words, data)
	}
	return r, nil
}

func isMarkdown(p string) bool {
	e := ext(p)
	return e == "md" || e == "mdx"
}

func addWords(set map[string]bool, data []byte) {
	start := -1
	for i := 0; i <= len(data); i++ {
		isWord := i < len(data) && (data[i] == '_' || data[i] >= '0' && data[i] <= '9' ||
			data[i] >= 'a' && data[i] <= 'z' || data[i] >= 'A' && data[i] <= 'Z')
		switch {
		case isWord && start < 0:
			start = i
		case !isWord && start >= 0:
			set[string(data[start:i])] = true
			start = -1
		}
	}
}

func (r *Repo) exists(p string) bool {
	return r.files[p] || r.dirs[p]
}

// resolve finds p as written, relative to the doc's directory, or (for a
// file name with a checked extension) as the tail of any tracked path.
func (r *Repo) resolve(docDir, p string) (string, bool) {
	if r.exists(p) {
		return p, true
	}
	if rel := path.Join(docDir, p); r.exists(rel) {
		return rel, true
	}
	if checkedExts[ext(p)] {
		suffix := "/" + p
		for f := range r.files {
			if strings.HasSuffix(f, suffix) {
				return f, true
			}
		}
	}
	return "", false
}

// rooted reports whether an extension-less slash path starts at a directory
// the repo or the doc's directory has, so MIME types and MP4 atom paths
// (`image/jpeg`, `tref/chap`) are not mistaken for repo paths.
func (r *Repo) rooted(docDir, p string) bool {
	first, _, _ := strings.Cut(p, "/")
	if first == "." || first == ".." {
		return false
	}
	return r.exists(first) || r.exists(path.Join(docDir, first))
}

func (r *Repo) sectionNames(p string) (map[string]bool, error) {
	if h, ok := r.headings[p]; ok {
		return h, nil
	}
	if !r.files[p] {
		return nil, errNotFound
	}
	data, err := fs.ReadFile(r.fsys, p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	h := map[string]bool{}
	inFence := false
	for line := range strings.SplitSeq(string(data), "\n") {
		if isFence(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			h[normSection(m[1])] = true
		}
		if m := boldLeadRe.FindStringSubmatch(line); m != nil {
			h[normSection(m[1])] = true
		}
	}
	r.headings[p] = h
	return h, nil
}

func normSection(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.TrimRight(s, ".: ")
}

func hasSection(names map[string]bool, want string) bool {
	want = normSection(want)
	if names[want] {
		return true
	}
	for n := range names {
		if strings.HasPrefix(n, want+" (") || strings.HasPrefix(n, want+":") {
			return true
		}
	}
	return false
}

func isFence(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// isInstructionDoc reports whether p is an agent instruction file. ADRs are
// left out: they record decisions as of their date and name code that later
// changed on purpose.
func isInstructionDoc(p string) bool {
	if strings.HasPrefix(p, "testdata/") || strings.Contains(p, "/testdata/") {
		return false
	}
	return path.Base(p) == "AGENTS.md" || p == "CODING_STANDARDS.md" ||
		strings.HasPrefix(p, "docs/agents/") && ext(p) == "md"
}

// ParseAllowlist reads one token per line; `#` starts a comment.
func ParseAllowlist(data []byte) map[string]bool {
	allow := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line != "" {
			allow[line] = true
		}
	}
	return allow
}

// CheckDoc returns the stale references in one instruction doc.
func CheckDoc(r *Repo, docPath string, content []byte, allow map[string]bool) []Finding {
	docDir := path.Dir(docPath)
	var out []Finding
	add := func(line int, token, reason string) {
		out = append(out, Finding{Path: docPath, Line: line, Token: token, Reason: reason})
	}
	inFence := false
	for i, line := range strings.Split(string(content), "\n") {
		n := i + 1
		if isFence(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range codeSpanRe.FindAllStringSubmatch(line, -1) {
			tok := m[1]
			if allow[strings.TrimSpace(tok)] {
				continue
			}
			kind, norm := classify(tok)
			if allow[norm] {
				continue
			}
			switch kind {
			case kindPath:
				if !checkedExts[ext(norm)] && !r.rooted(docDir, norm) {
					continue
				}
				if reason := r.checkPath(docDir, norm); reason != "" {
					add(n, tok, reason)
				}
			case kindIdent:
				if reason := r.checkIdent(norm); reason != "" {
					add(n, tok, reason)
				}
			case kindSkip:
			}
		}
		for _, m := range sectionRe.FindAllStringSubmatch(line, -1) {
			name, file := m[1], m[2]
			if allow[name] {
				continue
			}
			target, ok := r.resolve(docDir, strings.TrimPrefix(file, "./"))
			if !ok {
				add(n, strconv.Quote(name), "section file "+file+" does not exist")
				continue
			}
			names, err := r.sectionNames(target)
			if err != nil {
				add(n, strconv.Quote(name), err.Error())
				continue
			}
			if !hasSection(names, name) {
				add(n, strconv.Quote(name), "no such heading or bold lead-in in "+target)
			}
		}
		for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") ||
				strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "/") || allow[target] {
				continue
			}
			p, _, _ := strings.Cut(target, "#")
			if !r.exists(path.Join(docDir, p)) {
				add(n, target, "link target does not exist")
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Line < out[b].Line })
	return out
}

// checkPath returns why p is stale, or "" when it resolves. A Go-style
// qualified name (`pkg/sqliteconn.NewConnector`) checks the package
// directory and then the identifier.
func (r *Repo) checkPath(docDir, p string) string {
	if _, ok := r.resolve(docDir, p); ok {
		return ""
	}
	if r.ignored != nil && r.ignored(p) {
		return ""
	}
	base := path.Base(p)
	if pkg, ident, ok := strings.Cut(base, "."); ok && !checkedExts[ext(base)] && dottedIdent(base) {
		if _, found := r.resolve(docDir, path.Join(path.Dir(p), pkg)); found {
			return r.checkIdent(ident)
		}
	}
	return "path does not exist"
}

func (r *Repo) checkIdent(norm string) string {
	segs := strings.Split(norm, ".")
	var missing []string
	for _, s := range segs {
		if !r.words[s] {
			missing = append(missing, s)
		}
	}
	switch {
	case len(missing) == 0:
		return ""
	case len(segs) == 1:
		return "identifier not found in source"
	default:
		return "identifier not found in source (" + strings.Join(missing, ", ") + ")"
	}
}
