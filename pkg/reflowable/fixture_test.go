package reflowable

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/stretchr/testify/require"
)

// buildFixture writes the handoff EPUB and converts it to a KePub as Shisho
// delivers one, returning both paths.
func buildFixture(t *testing.T) (epubPath, kepubPath string) {
	t.Helper()
	dir := t.TempDir()
	epubPath = testgen.GenerateHandoffEPUB(t, dir, "handoff.epub")
	kepubPath = filepath.Join(dir, "handoff.kepub.epub")
	require.NoError(t, kepub.NewConverter().ConvertEPUB(context.Background(), epubPath, kepubPath))
	return epubPath, kepubPath
}

// TestWriteFixture writes the fixture EPUB and KePub to $REFLOWABLE_FIXTURE_DIR,
// for taking the positions the tests expect from the readers themselves:
//
//	REFLOWABLE_FIXTURE_DIR=$PWD/tmp/fixture go test ./pkg/reflowable -run TestWriteFixture
//	mise koreader xpointers --file tmp/fixture/handoff.epub
//	mise koreader xpointers --file tmp/fixture/handoff.kepub.epub
//	node pkg/reflowable/testdata/foliate-cfis.mjs tmp/fixture > pkg/reflowable/testdata/handoff-cfis.json
//
// testdata/handoff-xpointers.json holds the two xpointers outputs side by
// side, word by word (both books have the same words), with every word from
// the first three spine items and every seventh from the long last one.
func TestWriteFixture(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("REFLOWABLE_FIXTURE_DIR")
	if dir == "" {
		t.Skip("REFLOWABLE_FIXTURE_DIR is not set")
	}
	epubPath, kepubPath := buildFixture(t)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, path := range []string{epubPath, kepubPath} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.Base(path)), data, 0o644))
	}
}
