package cbz

import (
	"archive/zip"
	"context"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/pkg/errors"
)

// CheckReadable returns an error for an entry archive/zip cannot open: one
// with an unsupported compression method, or an encrypted one. Generators
// check entries up front with it, so such a CBZ is rejected before a
// download starts instead of failing partway.
func CheckReadable(f *zip.File) error {
	if f.Method != zip.Store && f.Method != zip.Deflate || f.Flags&0x1 != 0 {
		return errors.Wrapf(zip.ErrAlgorithm, "cannot read %s", f.Name)
	}
	return nil
}

// ProcessInOrder runs process on the pages in parallel and passes each
// result to write in page order, on the calling goroutine. Each page gets a
// slot before a worker starts on it, and there are as many slots as
// workers, so workers never run more than that many pages ahead of write
// and memory stays bounded however many pages the book has.
//
// It stops at the first error. An error from process names the page; one
// from write is returned as is; when ctx ends it returns ctx's error. A
// panic in process is returned as that page's error, since workers run
// outside any recovery on the caller's goroutine. It returns only once no
// worker is reading a page.
func ProcessInOrder[T any](ctx context.Context, pages []*zip.File, process func(*zip.File) (T, error), write func(i int, result T) error) error {
	numWorkers := min(runtime.NumCPU(), len(pages))
	if numWorkers == 0 {
		return nil
	}

	type result struct {
		value T
		err   error
	}
	type job struct {
		page *zip.File
		out  chan<- result
	}

	ctx, cancel := context.WithCancel(ctx)
	jobs := make(chan job)
	pending := make(chan chan result, numWorkers)
	var wg sync.WaitGroup
	// Stop the dispatcher and workers, and wait for them, before returning,
	// so nothing reads the source once the caller is done with it.
	defer func() {
		cancel()
		wg.Wait()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(jobs)
		defer close(pending)
		for _, page := range pages {
			out := make(chan result, 1)
			select {
			case pending <- out:
			case <-ctx.Done():
				return
			}
			select {
			case jobs <- job{page: page, out: out}:
			case <-ctx.Done():
				return
			}
		}
	}()

	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if err := ctx.Err(); err != nil {
					j.out <- result{err: err}
					continue
				}
				value, err := processRecovering(j.page, process)
				j.out <- result{value: value, err: err}
			}
		}()
	}

	i := 0
	for out := range pending {
		var r result
		select {
		case r = <-out:
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		}
		if r.err != nil {
			if err := ctx.Err(); err != nil {
				return errors.WithStack(err)
			}
			return errors.Wrapf(r.err, "failed to process page %d", i+1)
		}
		if err := write(i, r.value); err != nil {
			return err
		}
		i++
	}
	// The dispatcher stops early only when ctx is done.
	return errors.WithStack(ctx.Err())
}

func processRecovering[T any](page *zip.File, process func(*zip.File) (T, error)) (value T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic processing %s: %v\n%s", page.Name, r, debug.Stack())
		}
	}()
	return process(page)
}
