package main

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		token string
		kind  tokenKind
		norm  string
	}{
		// Repo paths.
		{"pkg/books/service.go", kindPath, "pkg/books/service.go"},
		{"pkg/books/", kindPath, "pkg/books"},
		{"types.go", kindPath, "types.go"},
		{"CONTEXT.md", kindPath, "CONTEXT.md"},
		{"./fixtures", kindPath, "fixtures"},
		{"pkg/books/service.go:42", kindPath, "pkg/books/service.go"},
		{"tref/chap", kindPath, "tref/chap"},

		// Not paths: routes, absolute, placeholders, globs, gitignored output.
		{"/api/books", kindSkip, ""},
		{"/config", kindSkip, ""},
		{"https://www.shishobooks.com", kindSkip, ""},
		{"pkg/plugins/*.go", kindSkip, ""},
		{"/docs/<id>", kindSkip, ""},
		{"pkg/{books,series}", kindSkip, ""},
		{"/share/:token", kindSkip, ""},
		{"tmp/library/", kindSkip, ""},
		{"app/types/generated/", kindSkip, ""},
		{"node_modules/foo/index.js", kindSkip, ""},
		{"build/api/api-air", kindSkip, ""},

		// Identifiers.
		{"CollectAffected", kindIdent, "CollectAffected"},
		{"searchService.CollectAffected", kindIdent, "searchService.CollectAffected"},
		{"useCan", kindIdent, "useCan"},
		{"useAuth()", kindIdent, "useAuth"},
		{"refetch()", kindIdent, "refetch"},
		{"useRequires(requirement, enabled)", kindIdent, "useRequires"},
		{"hasPermission(...)", kindIdent, "hasPermission"},
		{"t.Parallel()", kindIdent, "t.Parallel"},
		{"fileutils.CreateTemp", kindIdent, "fileutils.CreateTemp"},
		{"TestSchema_ForeignKeysHaveOnDelete", kindIdent, "TestSchema_ForeignKeysHaveOnDelete"},
		{"SHISHO_TEST_MODE", kindIdent, "SHISHO_TEST_MODE"},
		{"series_source", kindIdent, "series_source"},
		{"books.series_source", kindIdent, "books.series_source"},

		// Not identifiers.
		{"mdat", kindSkip, ""},
		{"Button", kindSkip, ""},
		{"API", kindSkip, ""},
		{"mise start", kindSkip, ""},
		{"lint:emdash", kindSkip, ""},
		{"-run", kindSkip, ""},
		{"X-Forwarded-Prefix", kindSkip, ""},
		{"json:\"-\"", kindSkip, ""},
		{"variant=\"link\"", kindSkip, ""},
		{"SHISHO_TEST_MODE=true", kindSkip, ""},
		{"{Entity}Response", kindSkip, ""},
		{"Index*", kindSkip, ""},
		{"LOWER(name)", kindSkip, ""},
		{"COLLATE NOCASE", kindSkip, ""},
		{"1.2.3", kindSkip, ""},
		{"?v=", kindSkip, ""},
		{"", kindSkip, ""},
	}
	for _, tc := range cases {
		kind, norm := classify(tc.token)
		assert.Equal(t, tc.kind, kind, "kind of %q", tc.token)
		if tc.kind != kindSkip {
			assert.Equal(t, tc.norm, norm, "normalized %q", tc.token)
		}
	}
}

func TestIsInstructionDoc(t *testing.T) {
	t.Parallel()
	assert.True(t, isInstructionDoc("AGENTS.md"))
	assert.True(t, isInstructionDoc("pkg/plugins/AGENTS.md"))
	assert.True(t, isInstructionDoc("CODING_STANDARDS.md"))
	assert.True(t, isInstructionDoc("docs/agents/backend/scanner.md"))
	assert.False(t, isInstructionDoc("docs/adr/0001-per-resource-alias-tables.md"))
	assert.False(t, isInstructionDoc("website/docs/metadata.md"))
	assert.False(t, isInstructionDoc("tools/checkdocrefs/testdata/AGENTS.md"))
	assert.False(t, isInstructionDoc("README.md"))
}

func fixtureRepo(t *testing.T) *Repo {
	t.Helper()
	fsys := fstest.MapFS{
		"AGENTS.md":                     {Data: []byte("# Root\n")},
		"pkg/AGENTS.md":                 {Data: []byte("# Backend\n\n## Shared services\n\n**Request binding must use structs**, never a slice.\n\n- **Config.** Validation lives here.\n")},
		"pkg/books/service.go":          {Data: []byte("package books\n\nfunc (s *searchService) CollectAffected() {}\nvar series_source = 1\nfunc h(c echo.Context) { c.JSON(200, nil) }\n")},
		"pkg/books/types.go":            {Data: []byte("package books\n")},
		"pkg/fileutils/temp.go":         {Data: []byte("package fileutils\n\nfunc CreateTemp() {}\n")},
		"pkg/migrations/schema_test.go": {Data: []byte("func TestSchema_ForeignKeysHaveOnDelete(t *testing.T) { t.Parallel() }\n")},
		"app/hooks/useCan.ts":           {Data: []byte("export function useCan() {}\nconst x = process.env.SHISHO_TEST_MODE\n")},
		"app/AGENTS.md":                 {Data: []byte("# Frontend\n")},
		"app/components/Thing.tsx":      {Data: []byte("export const Thing = 1\n")},
		"website/docs/notes.md":         {Data: []byte("GhostOnlyInMarkdown\n")},
	}
	paths := make([]string, 0, len(fsys))
	for p := range fsys {
		paths = append(paths, p)
	}
	repo, err := NewRepo(fsys, paths)
	require.NoError(t, err)
	return repo
}

func findingStrings(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.String())
	}
	return out
}

func TestCheckDoc_Paths(t *testing.T) {
	t.Parallel()
	repo := fixtureRepo(t)
	doc := "# Doc\n" +
		"See `pkg/books/service.go` and `pkg/books/` and `pkg/books/gone.go`.\n" +
		"Relative: `components/Thing.tsx`, bare `types.go`, suffix `books/service.go`.\n" +
		"Missing bare `nothere.go`, missing dir `pkg/ghost/`.\n" +
		"Skipped: `/api/books`, `tmp/data.sqlite`, `image/jpeg`, `pkg/*.go`.\n" +
		"```\n`pkg/in/fence.go`\n```\n" +
		"Link [svc](../pkg/books/service.go) and [bad](./missing.md) and [web](https://x.y/z).\n"
	got := findingStrings(CheckDoc(repo, "app/AGENTS.md", []byte(doc), nil))
	assert.Equal(t, []string{
		"app/AGENTS.md:2: pkg/books/gone.go: path does not exist",
		"app/AGENTS.md:4: nothere.go: path does not exist",
		"app/AGENTS.md:4: pkg/ghost/: path does not exist",
		"app/AGENTS.md:9: ./missing.md: link target does not exist",
	}, got)
}

func TestCheckDoc_Identifiers(t *testing.T) {
	t.Parallel()
	repo := fixtureRepo(t)
	doc := "Use `searchService.CollectAffected`, `useCan`, `useCan(requirement)`, `fileutils.CreateTemp`.\n" +
		"Tests: `TestSchema_ForeignKeysHaveOnDelete`, `t.Parallel()`. Env `SHISHO_TEST_MODE`. Column `books.series_source`.\n" +
		"Stale: `CollectEverything`, `otherutils.CreateTemp`, `useCannot()`, `GhostOnlyInMarkdown`, `OLD_ENV_VAR`.\n" +
		"Allowed: `NeverExisted`.\n"
	got := findingStrings(CheckDoc(repo, "AGENTS.md", []byte(doc), map[string]bool{"NeverExisted": true}))
	assert.Equal(t, []string{
		"AGENTS.md:3: CollectEverything: identifier not found in source",
		"AGENTS.md:3: otherutils.CreateTemp: identifier not found in source (otherutils)",
		"AGENTS.md:3: useCannot(): identifier not found in source",
		"AGENTS.md:3: GhostOnlyInMarkdown: identifier not found in source",
		"AGENTS.md:3: OLD_ENV_VAR: identifier not found in source",
	}, got)
}

func TestCheckDoc_SectionRefs(t *testing.T) {
	t.Parallel()
	repo := fixtureRepo(t)
	doc := "See \"Shared services\" in `pkg/AGENTS.md`.\n" +
		"Rules are under \"shared Services\" in pkg/AGENTS.md.\n" +
		"See \"Request binding must use structs\" in `pkg/AGENTS.md` and \"Config\" in `pkg/AGENTS.md`.\n" +
		"See \"API Conventions\" in `pkg/AGENTS.md`.\n" +
		"See \"Anything\" in `pkg/NOPE.md`.\n"
	got := findingStrings(CheckDoc(repo, "AGENTS.md", []byte(doc), nil))
	assert.Equal(t, []string{
		"AGENTS.md:4: \"API Conventions\": no such heading or bold lead-in in pkg/AGENTS.md",
		"AGENTS.md:5: pkg/NOPE.md: path does not exist",
		"AGENTS.md:5: \"Anything\": section file pkg/NOPE.md does not exist",
	}, got)
}

func TestCheckDoc_AllowlistAppliesToPaths(t *testing.T) {
	t.Parallel()
	repo := fixtureRepo(t)
	got := CheckDoc(repo, "AGENTS.md", []byte("Old `pkg/legacy.go`.\n"), map[string]bool{"pkg/legacy.go": true})
	assert.Empty(t, got)
}

func TestParseAllowlist(t *testing.T) {
	t.Parallel()
	allow := ParseAllowlist([]byte("# comment\n\nLOWER  # trailing comment\n  recreateTable\n"))
	assert.Equal(t, map[string]bool{"LOWER": true, "recreateTable": true}, allow)
}

func TestCheckDoc_PathEdgeCases(t *testing.T) {
	t.Parallel()
	repo := fixtureRepo(t)
	repo.ignored = func(p string) bool { return p == "demo/corpus" }
	doc := "Method `c.JSON` is code, not a path. EPUB-internal `../Images/cover.jpg`.\n" +
		"Ignored checkout `demo/corpus/`.\n" +
		"Qualified `pkg/fileutils.CreateTemp` and stale `pkg/fileutils.RemoveTemp` and `pkg/nopkg.Thing`.\n"
	got := findingStrings(CheckDoc(repo, "pkg/AGENTS.md", []byte(doc), nil))
	assert.Equal(t, []string{
		"pkg/AGENTS.md:3: pkg/fileutils.RemoveTemp: identifier not found in source",
		"pkg/AGENTS.md:3: pkg/nopkg.Thing: path does not exist",
	}, got)
}
