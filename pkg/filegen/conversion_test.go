package filegen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conversionFor returns the generator that delivers a File of type from as
// type to through Conversion.
func conversionFor(t *testing.T, from, to string) *conversionGenerator {
	t.Helper()
	gen, err := GeneratorFor(from, to)
	require.NoError(t, err)
	cg, ok := gen.(*conversionGenerator)
	require.True(t, ok, "%s to %s is not a Conversion", from, to)
	return cg
}

func TestConversionSteps_EachHasCode(t *testing.T) {
	t.Parallel()
	for _, step := range models.ConversionSteps {
		assert.Contains(t, conversionSteps, step, "%s to %s has no code that runs it", step.From, step.To)
	}
	assert.Len(t, conversionSteps, len(models.ConversionSteps), "a step with code is missing from models.ConversionSteps")
}

func TestGeneratorFor(t *testing.T) {
	t.Parallel()

	t.Run("its own File Type is its generator", func(t *testing.T) {
		t.Parallel()
		gen, err := GeneratorFor(models.FileTypeCBZ, models.FileTypeCBZ)
		require.NoError(t, err)
		assert.IsType(t, &CBZGenerator{}, gen)
	})

	t.Run("a reachable File Type is a Conversion", func(t *testing.T) {
		t.Parallel()
		for _, tc := range [][2]string{
			{models.FileTypeEPUB, models.FileTypeKePub},
			{models.FileTypeCBZ, models.FileTypeKePub},
			{models.FileTypeCBZ, models.FileTypeEPUB},
		} {
			gen := conversionFor(t, tc[0], tc[1])
			assert.Equal(t, tc[1], gen.SupportedType())
		}
	})

	t.Run("an unreachable File Type has no path", func(t *testing.T) {
		t.Parallel()
		for _, tc := range [][2]string{
			{models.FileTypeM4B, models.FileTypeEPUB},
			{models.FileTypePDF, models.FileTypeKePub},
			{models.FileTypeEPUB, models.FileTypeCBZ},
			{models.FileTypeEPUB, "nonsense"},
		} {
			_, err := GeneratorFor(tc[0], tc[1])
			require.ErrorIs(t, err, ErrNoConversion, "%s to %s", tc[0], tc[1])
		}
	})
}

// fakeStep is a Conversion step that lists the names of its input entries,
// or of the source when it is the first step, with its own name appended.
type fakeStep struct {
	name string
}

func (s fakeStep) prepare(_ context.Context, in stepInput) (Prepared, error) {
	var names []string
	if in.entries == nil {
		data, err := io.ReadAll(io.NewSectionReader(in.src, 0, in.src.Size()))
		if err != nil {
			return nil, err
		}
		names = strings.Fields(string(data))
	}
	for _, e := range in.entries {
		names = append(names, e.Name)
	}
	return &fakePrepared{names: append(names, s.name)}, nil
}

type fakePrepared struct {
	names   []string
	written bool
}

func (p *fakePrepared) Size() (int64, bool) { return 0, false }

func (p *fakePrepared) Write(_ context.Context, w io.Writer) error {
	p.written = true
	_, err := fmt.Fprint(w, strings.Join(p.names, " "))
	return err
}

func (p *fakePrepared) entries() []kepub.Entry {
	out := make([]kepub.Entry, 0, len(p.names))
	for _, n := range p.names {
		out = append(out, kepub.Entry{Name: n})
	}
	return out
}

type bytesSource struct{ *bytes.Reader }

func (s bytesSource) Size() int64 { return s.Reader.Size() }

// A chain hands each step's entries to the next without writing them, and
// only the last step writes.
func TestConversionGenerator_ChainPassesEntries(t *testing.T) {
	t.Parallel()
	first := fakeStep{name: "b"}
	gen := &conversionGenerator{to: "c", steps: []conversionStep{first, fakeStep{name: "c"}}}

	src := bytesSource{bytes.NewReader([]byte("x y"))}
	prepared, err := gen.Prepare(context.Background(), src, &models.Book{}, &models.File{})
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, prepared.Write(context.Background(), &out))
	assert.Equal(t, "x y b c", out.String())
}

type bytesOnlyStep struct{}

func (bytesOnlyStep) prepare(context.Context, stepInput) (Prepared, error) {
	return bytesOnly{}, nil
}

type bytesOnly struct{}

func (bytesOnly) Size() (int64, bool)                    { return 0, false }
func (bytesOnly) Write(context.Context, io.Writer) error { return nil }

// A step whose output cannot be read as entries cannot feed another step,
// and the chain fails in preparation rather than writing the wrong thing.
func TestConversionGenerator_StepWithoutEntriesCannotFeedAnother(t *testing.T) {
	t.Parallel()
	gen := &conversionGenerator{to: "c", steps: []conversionStep{bytesOnlyStep{}, fakeStep{name: "c"}}}
	_, err := gen.Prepare(context.Background(), bytesSource{bytes.NewReader(nil)}, &models.Book{}, &models.File{})
	var genErr *GenerationError
	require.ErrorAs(t, err, &genErr)
	assert.Equal(t, "c", genErr.FileType)
}
