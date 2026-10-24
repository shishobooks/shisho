package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReachableFileTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		from string
		want []string
	}{
		{FileTypeEPUB, []string{FileTypeEPUB, FileTypeKePub}},
		{FileTypeCBZ, []string{FileTypeCBZ, FileTypeKePub, FileTypeEPUB}},
		{FileTypeKePub, []string{FileTypeKePub, FileTypeEPUB}},
		{FileTypeAZW3, []string{FileTypeAZW3, FileTypeEPUB, FileTypeKePub}},
		{FileTypeMOBI, []string{FileTypeMOBI, FileTypeEPUB, FileTypeKePub}},
		{FileTypeM4B, []string{FileTypeM4B}},
		{FileTypePDF, []string{FileTypePDF}},
	}
	for _, tt := range tests {
		t.Run(tt.from, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ReachableFileTypes(tt.from))
		})
	}
}

func TestConversionChain(t *testing.T) {
	t.Parallel()

	t.Run("a File Type reaches itself with no steps", func(t *testing.T) {
		t.Parallel()
		chain, ok := ConversionChain(FileTypeEPUB, FileTypeEPUB)
		assert.True(t, ok)
		assert.Empty(t, chain)
	})

	t.Run("a direct step is one step", func(t *testing.T) {
		t.Parallel()
		chain, ok := ConversionChain(FileTypeCBZ, FileTypeEPUB)
		assert.True(t, ok)
		assert.Equal(t, []ConversionStep{{From: FileTypeCBZ, To: FileTypeEPUB}}, chain)
	})

	t.Run("no path", func(t *testing.T) {
		t.Parallel()
		_, ok := ConversionChain(FileTypeM4B, FileTypeEPUB)
		assert.False(t, ok)
	})
}

func TestConversionGraph_ChainsStepsAutomatically(t *testing.T) {
	t.Parallel()

	g := conversionGraph{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "d"},
		{From: "a", To: "c"},
	}

	chain, ok := g.chain("a", "d")
	assert.True(t, ok)
	assert.Equal(t, []ConversionStep{{From: "a", To: "c"}, {From: "c", To: "d"}}, chain, "takes the shortest chain")

	assert.Equal(t, []string{"a", "b", "c", "d"}, g.reachable("a"))
	assert.Equal(t, []string{"b", "c", "d"}, g.reachable("b"))
	assert.Equal(t, []string{"d"}, g.reachable("d"))
}

func TestFile_ReachableFileTypesResolved(t *testing.T) {
	t.Parallel()

	main := &File{FileType: FileTypeCBZ, FileRole: FileRoleMain}
	supplement := &File{FileType: FileTypeEPUB, FileRole: FileRoleSupplement}
	ResolveFileDisplayNames([]*File{main, supplement})

	assert.Equal(t, []string{FileTypeCBZ, FileTypeKePub, FileTypeEPUB}, main.ReachableFileTypes)
	assert.Equal(t, []string{FileTypeEPUB}, supplement.ReachableFileTypes, "a supplement downloads only as itself")
}
