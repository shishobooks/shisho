package testgen

import (
	"testing"

	"github.com/shishobooks/shisho/internal/mobigen"
)

// GenerateMOBI writes a MOBI, AZW3, or combo file shaped like Calibre's
// output and returns its path.
func GenerateMOBI(t *testing.T, dir, filename string, opts MOBIOptions) string {
	t.Helper()
	return WriteFile(t, dir, filename, BuildMOBI(t, opts))
}

// BuildMOBI returns the bytes of a MOBI, AZW3, or combo file; see
// mobigen.Build.
func BuildMOBI(t *testing.T, opts MOBIOptions) []byte {
	t.Helper()
	b, err := mobigen.Build(opts)
	if err != nil {
		t.Fatalf("failed to build MOBI: %v", err)
	}
	return b
}
