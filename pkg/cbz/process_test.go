package cbz

import (
	"archive/zip"
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// namedPages returns entries named by their index. ProcessInOrder never
// opens them itself, so they need no data.
func namedPages(n int) []*zip.File {
	pages := make([]*zip.File, n)
	for i := range pages {
		pages[i] = &zip.File{FileHeader: zip.FileHeader{Name: strconv.Itoa(i)}}
	}
	return pages
}

// Results reach write in page order even when later pages finish first.
func TestProcessInOrder_WritesInOrder(t *testing.T) {
	t.Parallel()
	pages := namedPages(50)
	var got []string
	err := ProcessInOrder(context.Background(), pages, func(f *zip.File) (string, error) {
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
	err := ProcessInOrder(context.Background(), nil, func(*zip.File) (int, error) {
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
	err := ProcessInOrder(context.Background(), namedPages(20), func(f *zip.File) (int, error) {
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
	err := ProcessInOrder(context.Background(), namedPages(200), func(*zip.File) (int, error) {
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
	err := ProcessInOrder(ctx, namedPages(20), func(*zip.File) (int, error) {
		return 0, nil
	}, func(int, int) error {
		return errors.New("wrote a page after cancellation")
	})
	require.ErrorIs(t, err, context.Canceled)
}
