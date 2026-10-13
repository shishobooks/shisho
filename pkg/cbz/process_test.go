package cbz

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"math"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// namedPages returns entries named by their index. They have no data, so
// tests run them through processInOrder with a flat cost instead of
// ProcessInOrder, which reads each page's header.
func namedPages(n int) []*zip.File {
	pages := make([]*zip.File, n)
	for i := range pages {
		pages[i] = &zip.File{FileHeader: zip.FileHeader{Name: strconv.Itoa(i)}}
	}
	return pages
}

// processFlat runs processInOrder as ProcessInOrder does, with every page
// costing the same small amount.
func processFlat[T any](ctx context.Context, pages []*zip.File, process func(*zip.File) (T, error), write func(int, T) error) error {
	return processInOrder(ctx, pages, runtime.GOMAXPROCS(0), pageMemoryBudget, func(*zip.File) int64 { return 1 }, process, write)
}

// Results reach write in page order even when later pages finish first.
func TestProcessInOrder_WritesInOrder(t *testing.T) {
	t.Parallel()
	pages := namedPages(50)
	var got []string
	err := processFlat(context.Background(), pages, func(f *zip.File) (string, error) {
		i, _ := strconv.Atoi(f.Name)
		time.Sleep(time.Duration(len(pages)-i) * 100 * time.Microsecond)
		return f.Name, nil
	}, func(i int, name string) error {
		assert.Equal(t, strconv.Itoa(i), name)
		got = append(got, name)
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, got, len(pages))
}

func TestProcessInOrder_NoPages(t *testing.T) {
	t.Parallel()
	err := processFlat(context.Background(), nil, func(*zip.File) (int, error) {
		t.Fatal("processed a page that does not exist")
		return 0, nil
	}, func(int, int) error {
		t.Fatal("wrote a page that does not exist")
		return nil
	})
	require.NoError(t, err)
}

// A panic in process fails that page instead of the process, and nothing
// from that page on is written.
func TestProcessInOrder_PanicFailsPage(t *testing.T) {
	t.Parallel()
	written := 0
	err := processFlat(context.Background(), namedPages(20), func(f *zip.File) (int, error) {
		if f.Name == "7" {
			panic("bad image")
		}
		return 0, nil
	}, func(int, int) error {
		written++
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "page 8")
	assert.Contains(t, err.Error(), "bad image")
	assert.Equal(t, 7, written)
}

// An error from write is returned as is, and no page is processed once
// ProcessInOrder has returned.
func TestProcessInOrder_WriteErrorStops(t *testing.T) {
	t.Parallel()
	writeErr := errors.New("disk full")
	var returned, late atomic.Bool
	err := processFlat(context.Background(), namedPages(200), func(*zip.File) (int, error) {
		if returned.Load() {
			late.Store(true)
		}
		return 0, nil
	}, func(i int, _ int) error {
		if i == 3 {
			return writeErr
		}
		return nil
	})
	returned.Store(true)
	require.ErrorIs(t, err, writeErr)
	assert.Equal(t, writeErr.Error(), err.Error())
	assert.False(t, late.Load(), "a page was processed after ProcessInOrder returned")
}

func TestProcessInOrder_Canceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := processFlat(ctx, namedPages(20), func(*zip.File) (int, error) {
		return 0, nil
	}, func(int, int) error {
		return errors.New("wrote a page after cancellation")
	})
	require.ErrorIs(t, err, context.Canceled)
}

// trackActive wraps process to record the most calls running at once.
func trackActive(process func(*zip.File) (int, error)) (func(*zip.File) (int, error), *atomic.Int32) {
	var active, peak atomic.Int32
	return func(f *zip.File) (int, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		return process(f)
	}, &peak
}

// Pages run at once only while their estimated memory fits the budget,
// however many workers there are.
func TestProcessInOrder_StaysWithinMemoryBudget(t *testing.T) {
	t.Parallel()
	process, peak := trackActive(func(*zip.File) (int, error) { return 0, nil })
	written := 0
	err := processInOrder(context.Background(), namedPages(40), 8, 100, func(*zip.File) int64 { return 30 }, process, func(int, int) error {
		written++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 40, written)
	assert.LessOrEqual(t, peak.Load(), int32(3), "pages of 30 against a budget of 100")
	assert.Greater(t, peak.Load(), int32(1), "pages within the budget should still run in parallel")
}

// A page estimated above the whole budget still runs, by itself.
func TestProcessInOrder_PageOverBudgetRunsAlone(t *testing.T) {
	t.Parallel()
	var sawOthers atomic.Bool
	var bigRunning, othersRunning atomic.Int32
	process := func(f *zip.File) (int, error) {
		if f.Name == "5" {
			bigRunning.Add(1)
			defer bigRunning.Add(-1)
			time.Sleep(5 * time.Millisecond)
			if othersRunning.Load() > 0 {
				sawOthers.Store(true)
			}
			return 0, nil
		}
		othersRunning.Add(1)
		defer othersRunning.Add(-1)
		if bigRunning.Load() > 0 {
			sawOthers.Store(true)
		}
		time.Sleep(time.Millisecond)
		return 0, nil
	}
	cost := func(f *zip.File) int64 {
		if f.Name == "5" {
			return 1000
		}
		return 10
	}
	written := 0
	err := processInOrder(context.Background(), namedPages(20), 8, 100, cost, process, func(int, int) error {
		written++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 20, written)
	assert.False(t, sawOthers.Load(), "another page ran beside the page over budget")
}

// zipEntries returns the entries of a zip holding pages, keyed by name.
func zipEntries(t *testing.T, pages map[string][]byte) map[string]*zip.File {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range pages {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		entries[f.Name] = f
	}
	return entries
}

// The estimate counts a page's data twice and its decoded pixels, from the
// header alone. A progressive JPEG also counts the coefficients the decoder
// keeps, 4 bytes per sample; an unreadable page counts its data only; and
// an estimate too large for int64 saturates.
func TestEstimatePageMemory(t *testing.T) {
	t.Parallel()
	var gray, color bytes.Buffer
	require.NoError(t, jpeg.Encode(&gray, image.NewGray(image.Rect(0, 0, 100, 200)), nil))
	require.NoError(t, jpeg.Encode(&color, image.NewRGBA(image.Rect(0, 0, 100, 200)), nil))
	// image/jpeg writes only baseline JPEGs; a progressive one differs in
	// its frame marker, which is all the estimate reads.
	progressive := bytes.Replace(color.Bytes(), []byte{0xff, 0xc0}, []byte{0xff, 0xc2}, 1)

	entries := zipEntries(t, map[string][]byte{
		"gray.jpg":        gray.Bytes(),
		"progressive.jpg": progressive,
		"broken.jpg":      []byte("not an image"),
	})
	data := func(name string) int64 {
		return 2 * int64(entries[name].UncompressedSize64) //nolint:gosec // small test entries
	}
	assert.Equal(t, data("gray.jpg")+100*200, estimatePageMemory(entries["gray.jpg"]))
	// image/jpeg subsamples color 4:2:0, 1.5 samples per pixel.
	assert.Equal(t, data("progressive.jpg")+100*200*3/2*5, estimatePageMemory(entries["progressive.jpg"]))
	assert.Equal(t, data("broken.jpg"), estimatePageMemory(entries["broken.jpg"]))
	// A zip entry can declare any size, and opening this one panics.
	bogus := &zip.File{FileHeader: zip.FileHeader{Name: "bogus.jpg", UncompressedSize64: math.MaxUint64}}
	assert.Equal(t, int64(math.MaxInt64), estimatePageMemory(bogus))
}
