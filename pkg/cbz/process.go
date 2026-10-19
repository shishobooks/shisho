package cbz

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"io"
	"math"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
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
// Pages also run at once only while their estimated memory fits
// pageMemoryBudget, since a worker decodes a whole page: a book of large
// pages runs fewer at a time instead of holding one decoded page per CPU.
//
// It stops at the first error. An error from process names the page; one
// from write is returned as is; when ctx ends it returns ctx's error. A
// panic in process is returned as that page's error, since workers run
// outside any recovery on the caller's goroutine. It returns only once no
// worker is reading a page.
func ProcessInOrder[T any](ctx context.Context, pages []*zip.File, process func(*zip.File) (T, error), write func(i int, result T) error) error {
	return processInOrder(ctx, pages, runtime.GOMAXPROCS(0), pageMemoryBudget, estimatePageMemory, process, write)
}

// pageMemoryBudget is the most estimated memory the pages being processed
// take at once. A page estimated above it runs alone. It counts what a
// page's processing allocates, not the heap: the garbage collector lets
// freed pages build up past it, and a page passed through unchanged holds
// its data until it is written.
const pageMemoryBudget = 256 << 20

// estimatePageMemory guesses the most memory processing f takes, for the
// callers in pkg/filegen and pkg/fixedlayout, which read the page and decode it
// with fixedlayout.ProcessImageForEreader: its data twice, since reading it grows
// a buffer, and its decoded pixels. A resized page is small next to its
// source. A page whose header cannot be read is passed through without
// decoding, so only its data counts. It runs outside any recovery, so a
// decoder panic counts as an unreadable header.
func estimatePageMemory(f *zip.File) int64 {
	// Summed as floats so a bogus size saturates instead of wrapping.
	// float64(math.MaxInt64) is 2^63, which does not convert to int64.
	n := 2*float64(f.UncompressedSize64) + decodeMemory(f)
	if n < math.MaxInt64 {
		return int64(n)
	}
	return math.MaxInt64
}

// decodeMemory is how many bytes decoding f takes, read from its header, or
// 0 if the header cannot be read.
func decodeMemory(f *zip.File) (n float64) {
	defer func() {
		if recover() != nil {
			n = 0
		}
	}()
	r, err := f.Open()
	if err != nil {
		return 0
	}
	defer r.Close()
	// DecodeConfig reads a JPEG up to its frame header, which says what the
	// decoder allocates.
	var header bytes.Buffer
	cfg, format, err := image.DecodeConfig(io.TeeReader(r, &header))
	if err != nil {
		return 0
	}
	pixels := float64(cfg.Width) * float64(cfg.Height)
	if format == "jpeg" {
		if samples, progressive, ok := jpegFrame(header.Bytes()); ok {
			// One byte per sample, and for a progressive JPEG the 32-bit
			// coefficients of every sample until the last scan.
			if progressive {
				return pixels * samples * 5
			}
			return pixels * samples
		}
	}
	return pixels * bytesPerPixel(cfg.ColorModel)
}

// jpegFrame reads the frame header of the JPEG starting data: how many
// samples it holds per pixel, counting each component at its subsampled
// size, and whether it is progressive.
func jpegFrame(data []byte) (samples float64, progressive, ok bool) {
	i := 2 // after the start-of-image marker
	for i+4 <= len(data) {
		if data[i] != 0xff {
			return 0, false, false
		}
		marker := data[i+1]
		if marker == 0xff { // fill byte
			i++
			continue
		}
		length := int(data[i+2])<<8 | int(data[i+3])
		// SOF0 to SOF15, other than DHT, JPG, and DAC.
		if marker >= 0xc0 && marker <= 0xcf && marker != 0xc4 && marker != 0xc8 && marker != 0xcc {
			frame := data[i+4 : min(i+2+length, len(data))]
			if len(frame) < 6 || len(frame) < 6+3*int(frame[5]) {
				return 0, false, false
			}
			n := int(frame[5])
			var hMax, vMax int
			for c := range n {
				hv := frame[6+3*c+1]
				hMax = max(hMax, int(hv>>4))
				vMax = max(vMax, int(hv&0x0f))
			}
			if hMax == 0 || vMax == 0 {
				return 0, false, false
			}
			for c := range n {
				hv := frame[6+3*c+1]
				samples += float64(int(hv>>4)*int(hv&0x0f)) / float64(hMax*vMax)
			}
			progressive = marker == 0xc2 || marker == 0xc6 || marker == 0xca || marker == 0xce
			return samples, progressive, true
		}
		i += 2 + length
	}
	return 0, false, false
}

// bytesPerPixel is how many bytes a decoded image in model takes per pixel.
func bytesPerPixel(model color.Model) float64 {
	switch model {
	case color.GrayModel:
		return 1
	case color.Gray16Model:
		return 2
	case color.YCbCrModel:
		return 3
	case color.RGBA64Model, color.NRGBA64Model:
		return 8
	}
	if _, ok := model.(color.Palette); ok {
		return 1
	}
	return 4
}

func processInOrder[T any](ctx context.Context, pages []*zip.File, workers int, budget int64, cost func(*zip.File) int64, process func(*zip.File) (T, error), write func(i int, result T) error) error {
	numWorkers := min(workers, len(pages))
	if numWorkers == 0 {
		return nil
	}

	type result struct {
		value T
		err   error
	}
	type job struct {
		page *zip.File
		cost int64
		out  chan<- result
	}

	ctx, cancel := context.WithCancel(ctx)
	jobs := make(chan job)
	pending := make(chan chan result, numWorkers)
	memory := semaphore.NewWeighted(budget)
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
			// Pages take budget in page order, so the next page write
			// needs is never waiting behind a later one.
			cost := min(max(cost(page), 1), budget)
			if err := memory.Acquire(ctx, cost); err != nil {
				return
			}
			select {
			case jobs <- job{page: page, cost: cost, out: out}:
			case <-ctx.Done():
				memory.Release(cost)
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
					memory.Release(j.cost)
					j.out <- result{err: err}
					continue
				}
				value, err := processRecovering(j.page, process)
				memory.Release(j.cost)
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
