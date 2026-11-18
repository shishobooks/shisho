package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCatalogPDF writes a one-page PDF whose catalog has catalogExtra added,
// with extra objects numbered from 4.
func writeCatalogPDF(t *testing.T, catalogExtra string, extra ...string) string {
	t.Helper()
	objects := append([]string{
		"<< /Type /Catalog /Pages 2 0 R " + catalogExtra + " >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 612 792] >>",
		"<< /Type /Page /Parent 2 0 R >>",
	}, extra...)
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	path := filepath.Join(t.TempDir(), "book.pdf")
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o644))
	return path
}

func TestParse_ReadingDirection(t *testing.T) {
	t.Parallel()

	rtl := models.ReadingDirectionRTL
	ltr := models.ReadingDirectionLTR
	tests := []struct {
		name    string
		catalog string
		extra   []string
		want    *string
	}{
		{name: "R2L runs right to left", catalog: "/ViewerPreferences << /Direction /R2L >>", want: &rtl},
		{name: "L2R runs left to right", catalog: "/ViewerPreferences << /Direction /L2R >>", want: &ltr},
		{name: "preferences held in their own object", catalog: "/ViewerPreferences 4 0 R", extra: []string{"<< /Direction /R2L >>"}, want: &rtl},
		{name: "preferences with no Direction say nothing", catalog: "/ViewerPreferences << /HideToolbar true >>", want: nil},
		{name: "no preferences say nothing", catalog: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			meta, err := Parse(writeCatalogPDF(t, tt.catalog, tt.extra...))
			require.NoError(t, err)
			assert.Equal(t, tt.want, meta.ReadingDirection)
		})
	}
}
