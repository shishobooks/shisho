package filegen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mobiTOC = []testgen.MOBITOCEntry{
	{Title: "Part One", Children: []testgen.MOBITOCEntry{
		{Title: "Chapter 1"},
		{Title: "Chapter 2", Children: []testgen.MOBITOCEntry{{Title: "Scene 2.1"}}},
	}},
	{Title: "Part Two", Children: []testgen.MOBITOCEntry{{Title: "Chapter 3"}}},
	{Title: "Epilogue"},
}

// generateMOBIChapters builds a MOBI file with mobiTOC, lets edit change
// the chapters a scan stores for it, generates a download, and returns the
// source and the download.
func generateMOBIChapters(t *testing.T, opts testgen.MOBIOptions, fileType string, edit func([]tocChapter) []tocChapter) ([]byte, []byte) {
	t.Helper()
	dir := t.TempDir()
	srcPath := testgen.GenerateMOBI(t, dir, "book."+fileType, opts)
	parsed, err := mobi.Parse(srcPath)
	require.NoError(t, err)

	file := &models.File{FileType: fileType, Filepath: srcPath, Chapters: storedChapters(edit(navTree(parsed.Chapters)))}
	destPath := filepath.Join(dir, "out."+fileType)
	gen := &MOBIGenerator{fileType: fileType}
	require.NoError(t, gen.Generate(context.Background(), srcPath, destPath, &models.Book{Title: "Book"}, file))

	src, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	out, err := os.ReadFile(destPath)
	require.NoError(t, err)
	return src, out
}

// mobiTOCTitles returns each book's table of contents as tocChapter titles
// only, KF8 first.
func mobiTOCTitles(t *testing.T, data []byte) [][]tocChapter {
	t.Helper()
	tocs, err := mobi.ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	var walk func([]*mobi.TOCEntry) []tocChapter
	walk = func(entries []*mobi.TOCEntry) []tocChapter {
		var out []tocChapter
		for _, e := range entries {
			out = append(out, tocChapter{title: e.Title, children: walk(e.Children)})
		}
		return out
	}
	out := make([][]tocChapter, 0, len(tocs))
	for _, toc := range tocs {
		out = append(out, walk(toc.Entries))
	}
	return out
}

func withoutHrefs(tree []tocChapter) []tocChapter {
	for i := range tree {
		tree[i].href = ""
		tree[i].children = withoutHrefs(tree[i].children)
	}
	return tree
}

// renameAndDelete renames Chapter 1 and deletes Chapter 2 with its scene.
func renameAndDelete(tree []tocChapter) []tocChapter {
	tree[0].children[0].title = "The First Chapter"
	tree[0].children = tree[0].children[:1]
	return tree
}

var editedMOBITOC = []tocChapter{
	{title: "Part One", children: []tocChapter{{title: "The First Chapter"}}},
	{title: "Part Two", children: []tocChapter{{title: "Chapter 3"}}},
	{title: "Epilogue"},
}

func TestMOBIGenerator_WritesChapterEdits(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		kind     testgen.MOBIKind
		fileType string
		books    int
	}{
		"MOBI6": {testgen.MOBIKindMOBI6, models.FileTypeMOBI, 1},
		"AZW3":  {testgen.MOBIKindKF8, models.FileTypeAZW3, 1},
		"combo": {testgen.MOBIKindCombo, models.FileTypeMOBI, 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, out := generateMOBIChapters(t, testgen.MOBIOptions{Kind: tc.kind, Title: "Book", TOC: mobiTOC}, tc.fileType, renameAndDelete)
			tocs := mobiTOCTitles(t, out)
			require.Len(t, tocs, tc.books)
			for _, toc := range tocs {
				assert.Equal(t, editedMOBITOC, toc)
			}

			// A scan of the download stores the edited chapters.
			path := filepath.Join(t.TempDir(), "download."+tc.fileType)
			require.NoError(t, os.WriteFile(path, out, 0o600))
			parsed, err := mobi.Parse(path)
			require.NoError(t, err)
			assert.Equal(t, editedMOBITOC, withoutHrefs(navTree(parsed.Chapters)))
		})
	}
}

func TestMOBIGenerator_CarriesChapterEditsToFlatMOBI6Half(t *testing.T) {
	t.Parallel()

	// Calibre's combo files hold a nested KF8 index and a flat MOBI6 one.
	// Deleting a chapter deletes the entries nested under it in both.
	opts := testgen.MOBIOptions{Kind: testgen.MOBIKindCombo, Title: "Book", TOC: mobiTOC, FlatMOBI6TOC: true}
	_, out := generateMOBIChapters(t, opts, models.FileTypeMOBI, renameAndDelete)
	tocs := mobiTOCTitles(t, out)
	require.Len(t, tocs, 2)
	assert.Equal(t, editedMOBITOC, tocs[0])
	assert.Equal(t, []tocChapter{
		{title: "Part One"}, {title: "The First Chapter"}, {title: "Part Two"}, {title: "Chapter 3"}, {title: "Epilogue"},
	}, tocs[1])
}

func TestMOBIGenerator_LeavesTOCWhenChaptersDoNotMatch(t *testing.T) {
	t.Parallel()

	for name, edit := range map[string]func([]tocChapter) []tocChapter{
		"unedited": func(tree []tocChapter) []tocChapter { return tree },
		"added": func(tree []tocChapter) []tocChapter {
			return append(tree, tocChapter{title: "New", href: "filepos:0000000001"})
		},
		"reordered": func(tree []tocChapter) []tocChapter {
			tree[0], tree[1] = tree[1], tree[0]
			return tree
		},
		"none": func([]tocChapter) []tocChapter { return nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := testgen.MOBIOptions{Kind: testgen.MOBIKindCombo, Title: "Book", TOC: mobiTOC}
			src, out := generateMOBIChapters(t, opts, models.FileTypeMOBI, edit)
			assert.Equal(t, mobiTOCTitles(t, src), mobiTOCTitles(t, out))
		})
	}
}
