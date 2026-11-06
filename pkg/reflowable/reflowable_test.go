package reflowable

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// harnessWord is a word start crengine reported, and the text from it to the
// next word start.
type harnessWord struct {
	XPointer string `json:"xpointer"`
	Text     string `json:"text"`
}

// foliatePlace is a place foliate made a CFI for.
type foliatePlace struct {
	Spine  int    `json:"spine"`
	Offset int    `json:"offset"`
	CFI    string `json:"cfi"`
}

func loadJSON[T any](t *testing.T, name string) map[string][]T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	out := map[string][]T{}
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

// variants opens the fixture every way a File can be delivered: an EPUB as
// itself and as a KePub, and a KePub as itself and stripped to EPUB. The key
// says which reader data applies: "epub" or "kepub".
func variants(t *testing.T) map[string]map[string]*Book {
	t.Helper()
	epubPath, kepubPath := buildFixture(t)
	open := func(path string, kobo bool) *Book {
		book, err := Open(context.Background(), path, kobo)
		require.NoError(t, err)
		return book
	}
	return map[string]map[string]*Book{
		"epub": {
			"EPUB as itself":   open(epubPath, false),
			"KePub as an EPUB": open(kepubPath, false),
		},
		"kepub": {
			"EPUB as a KePub": open(epubPath, true),
			"KePub as itself": open(kepubPath, true),
		},
	}
}

func firstWord(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// XPointers read and write exactly as crengine does, in both forms of the
// book, including its whitespace collapsing, the whitespace-only text it
// drops, and comments and CDATA starting new text nodes.
func TestXPointersMatchKOReader(t *testing.T) {
	t.Parallel()
	words := loadJSON[harnessWord](t, "handoff-xpointers.json")
	for form, books := range variants(t) {
		for name, book := range books {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				for _, word := range words[form] {
					place, err := book.FromXPointer(word.XPointer)
					require.NoError(t, err, word.XPointer)
					require.True(t, place.Exact, word.XPointer)
					assert.Equal(t, word.XPointer, book.XPointer(place))
					if want := firstWord(word.Text); want != "" {
						assert.True(t, strings.HasPrefix(book.textAt(place, 40), want),
							"%s: %q doesn't start with %q", word.XPointer, book.textAt(place, 40), want)
					}
				}
			})
		}
	}
}

// CFIs read and write exactly as the web reader's foliate does.
func TestCFIsMatchFoliate(t *testing.T) {
	t.Parallel()
	places := loadJSON[foliatePlace](t, "handoff-cfis.json")
	for form, books := range variants(t) {
		for name, book := range books {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				for _, want := range places[form] {
					place, err := book.FromCFI(want.CFI)
					require.NoError(t, err, want.CFI)
					require.True(t, place.Exact, want.CFI)
					assert.Equal(t, want.Spine, place.Spine, want.CFI)
					assert.Equal(t, want.Offset, place.Offset, want.CFI)
					assert.Equal(t, want.CFI, book.CFI(Place{Spine: want.Spine, Offset: want.Offset, Exact: true}))
				}
			})
		}
	}
}

// A place goes from a CFI to an XPointer and back to the same CFI, across
// the span markup a KePub adds, landing on the same text both ways.
func TestCFIToXPointerToCFI(t *testing.T) {
	t.Parallel()
	words := loadJSON[harnessWord](t, "handoff-xpointers.json")
	books := variants(t)
	for _, webName := range []string{"EPUB as itself", "KePub as itself"} {
		for _, koreaderForm := range []string{"epub", "kepub"} {
			web := books["epub"][webName]
			if webName == "KePub as itself" {
				web = books["kepub"][webName]
			}
			koreader := books[koreaderForm]["EPUB as itself"]
			if koreaderForm == "kepub" {
				koreader = books[koreaderForm]["EPUB as a KePub"]
			}
			t.Run(webName+" to "+koreaderForm, func(t *testing.T) {
				t.Parallel()
				for _, word := range words[koreaderForm] {
					fromKOReader, err := koreader.FromXPointer(word.XPointer)
					require.NoError(t, err)
					cfi := web.CFI(fromKOReader)

					fromWeb, err := web.FromCFI(cfi)
					require.NoError(t, err, cfi)
					require.True(t, fromWeb.Exact, cfi)
					assert.Equal(t, fromKOReader.Offset, fromWeb.Offset, cfi)
					assert.Equal(t, word.XPointer, koreader.XPointer(fromWeb), cfi)
					assert.Equal(t, cfi, web.CFI(fromWeb))
				}
			})
		}
	}
}

// The estimate, a spine item and how far through its text, carries a place
// between every pair of forms and lands within a sentence of it: the fixture's longest is under 100 characters.
func TestEstimateBetweenEveryForm(t *testing.T) {
	t.Parallel()
	words := loadJSON[harnessWord](t, "handoff-xpointers.json")
	books := variants(t)
	plain, kobo := books["epub"]["EPUB as itself"], books["kepub"]["EPUB as a KePub"]

	type form struct {
		name string
		to   func(Place) (models.ReadingLocator, error)
		from func(models.ReadingLocator) (Place, error)
	}
	cfiIn := func(book *Book) form {
		return form{"CFI",
			func(p Place) (models.ReadingLocator, error) {
				cfi := book.CFI(p)
				return models.ReadingLocator{CFI: &cfi}, nil
			},
			func(l models.ReadingLocator) (Place, error) { return book.FromCFI(*l.CFI) },
		}
	}
	xpointerIn := func(book *Book) form {
		return form{"XPointer",
			func(p Place) (models.ReadingLocator, error) {
				xp := book.XPointer(p)
				return models.ReadingLocator{XPointer: &xp}, nil
			},
			func(l models.ReadingLocator) (Place, error) { return book.FromXPointer(*l.XPointer) },
		}
	}
	koboSpan := form{"Kobo span",
		func(p Place) (models.ReadingLocator, error) {
			span, err := kobo.KoboSpan(p)
			if err != nil {
				return models.ReadingLocator{}, err
			}
			encoded := span.Source + "#" + span.ID
			return models.ReadingLocator{KoboSpan: &encoded}, nil
		},
		func(l models.ReadingLocator) (Place, error) {
			source, id, _ := strings.Cut(*l.KoboSpan, "#")
			return kobo.FromKoboSpan(KoboSpan{Source: source, ID: id})
		},
	}
	forms := []form{cfiIn(plain), xpointerIn(plain), xpointerIn(kobo), koboSpan}
	forms[0].name, forms[1].name, forms[2].name = "web reader CFI", "KOReader XPointer in the EPUB", "KOReader XPointer in the KePub"

	for _, a := range forms {
		for _, b := range forms {
			t.Run(a.name+" to "+b.name, func(t *testing.T) {
				t.Parallel()
				for _, word := range words["epub"] {
					start, err := plain.FromXPointer(word.XPointer)
					require.NoError(t, err)
					locator, err := a.to(start)
					require.NoError(t, err)
					inA, err := a.from(locator)
					require.NoError(t, err)

					estimate := inA.Estimate()
					assert.False(t, estimate.Exact)
					locator, err = b.to(estimate)
					require.NoError(t, err)
					landed, err := b.from(locator)
					require.NoError(t, err)

					assert.Equal(t, start.Spine, landed.Spine, word.XPointer)
					assert.InDelta(t, start.Offset, landed.Offset, 100, "%s landed at %q", word.XPointer, plain.textAt(landed, 30))
				}
			})
		}
	}
}

// A position that doesn't resolve in this copy of the book, such as one
// from another edition, falls back to the estimate.
func TestFallbackOnMismatchedDocument(t *testing.T) {
	t.Parallel()
	books := variants(t)
	plain, kobo := books["epub"]["EPUB as itself"], books["kepub"]["EPUB as a KePub"]

	section, err := plain.FromXPointer("/body/DocFragment[2]/body/section")
	require.NoError(t, err)

	t.Run("an element that isn't there lands at the deepest one that is", func(t *testing.T) {
		t.Parallel()
		place, err := plain.FromXPointer("/body/DocFragment[2]/body/section/p[99]/text().4")
		require.NoError(t, err)
		assert.False(t, place.Exact)
		assert.Equal(t, section.Spine, place.Spine)
		assert.Equal(t, section.Offset, place.Offset)

		place, err = plain.FromCFI("epubcfi(/6/4!/4/2/98/1:4)")
		require.NoError(t, err)
		assert.False(t, place.Exact)
		assert.Equal(t, section.Offset, place.Offset)
	})

	t.Run("an offset past the text stays in its node", func(t *testing.T) {
		t.Parallel()
		place, err := plain.FromXPointer("/body/DocFragment[2]/body/section/p[1]/text().5000")
		require.NoError(t, err)
		assert.False(t, place.Exact)
		end, err := plain.FromXPointer("/body/DocFragment[2]/body/section/p[1]/text().103")
		require.NoError(t, err)
		assert.Equal(t, end.Offset, place.Offset)
	})

	t.Run("a KePub's XPointer in the EPUB lands nearby", func(t *testing.T) {
		t.Parallel()
		inKobo := "/body/DocFragment[2]/body/div/div/section/p[2]/span[4]/text().8"
		exact, err := kobo.FromXPointer(inKobo)
		require.NoError(t, err)
		place := plain.Locate(models.ReadingLocator{XPointer: &inKobo}, 0.3)
		assert.False(t, place.Exact)
		assert.Equal(t, exact.Spine, place.Spine)
		assert.Less(t, place.Offset, exact.Offset)
	})

	t.Run("a spine item that isn't there falls back to the percentage", func(t *testing.T) {
		t.Parallel()
		missing := "/body/DocFragment[9]/body/p/text().3"
		place := plain.Locate(models.ReadingLocator{XPointer: &missing}, 0.9)
		assert.False(t, place.Exact)
		assert.Equal(t, plain.AtPercentage(0.9), place)
		assert.Equal(t, 3, place.Spine, "90% of the text is in the last chapter")

		cfi := "epubcfi(/6/40!/4/2/1:0)"
		assert.Equal(t, plain.AtPercentage(0.9), plain.Locate(models.ReadingLocator{CFI: &cfi}, 0.9))
		garbage := "not a position"
		assert.Equal(t, plain.AtPercentage(0.5), plain.Locate(models.ReadingLocator{XPointer: &garbage}, 0.5))
	})

	t.Run("a position that resolves is exact", func(t *testing.T) {
		t.Parallel()
		xp := "/body/DocFragment[2]/body/section/p[1]/text().58"
		place := plain.Locate(models.ReadingLocator{XPointer: &xp}, 0.1)
		assert.True(t, place.Exact)
		assert.True(t, strings.HasPrefix(plain.textAt(place, 10), "Beta"))
	})
}

// A place at the end of a paragraph's text, as the web reader saves when its
// screen starts right after it, stays in that paragraph rather than moving to
// the next, whose start is past the whitespace between them.
func TestXPointerAtTheEndOfAParagraph(t *testing.T) {
	t.Parallel()
	books := variants(t)
	plain, kobo := books["epub"]["EPUB as itself"], books["kepub"]["EPUB as a KePub"]
	end, err := plain.FromXPointer("/body/DocFragment[2]/body/section/p[1]/text().103")
	require.NoError(t, err)
	require.True(t, end.Exact)
	assert.Equal(t, "/body/DocFragment[2]/body/section/p[1]/text().103", plain.XPointer(end))
	assert.Equal(t, "/body/DocFragment[2]/body/div/div/section/p[1]/span[4]/text().14", kobo.XPointer(end))

	// Where the next text starts right there, as across inline markup, the
	// place is the start of that text.
	emphasis, err := plain.FromXPointer("/body/DocFragment[2]/body/section/p[2]/em/text().0")
	require.NoError(t, err)
	assert.Equal(t, "/body/DocFragment[2]/body/section/p[2]/em/text().0", plain.XPointer(emphasis))
}
