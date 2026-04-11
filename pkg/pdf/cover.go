package pdf

import (
	"bytes"
	"image/jpeg"
	"io"
	"os"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pkg/errors"
)

// pdfiumPool is lazily initialized on first use via pdfiumOnce.
// go-pdfium's WASM backend embeds a ~15-25 MB PDFium binary, so we
// only pay that cost when cover extraction actually needs rendering.
var (
	pdfiumOnce sync.Once
	pdfiumPool pdfium.Pool
	pdfiumErr  error
)

// initPdfiumPool initializes the go-pdfium WASM pool. Called via sync.Once.
func initPdfiumPool() {
	pdfiumPool, pdfiumErr = webassembly.Init(webassembly.Config{
		MinIdle:  0,
		MaxIdle:  1,
		MaxTotal: 1,
	})
}

// extractCover attempts to extract a cover image from a PDF.
// Tier 1: Extract embedded images from page 1 via pdfcpu.
// Tier 2: Render page 1 to JPEG via go-pdfium WASM.
func extractCover(path string) ([]byte, string, error) {
	// Tier 1: try to extract an embedded image from page 1.
	data, mime, err := extractEmbeddedCover(path)
	if err == nil && len(data) > 0 {
		return data, mime, nil
	}

	// Tier 2: render page 1 to JPEG via go-pdfium WASM.
	return renderPageCover(path)
}

// extractEmbeddedCover uses pdfcpu to extract images from page 1 and
// returns the largest image by pixel area.
func extractEmbeddedCover(path string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	// Extract images from page 1 only.
	imageMaps, err := api.ExtractImagesRaw(f, []string{"1"}, conf)
	if err != nil {
		return nil, "", err
	}

	// Find the largest image by pixel area across all returned maps.
	var bestData []byte
	var bestMime string
	var bestArea int

	for _, imgMap := range imageMaps {
		for _, img := range imgMap {
			area := img.Width * img.Height
			if area <= bestArea {
				continue
			}
			// Read image data from the embedded io.Reader.
			data, err := io.ReadAll(img.Reader)
			if err != nil {
				continue
			}
			if len(data) == 0 {
				continue
			}
			bestData = data
			bestArea = area
			bestMime = mimeForFileType(img.FileType)
		}
	}

	if len(bestData) == 0 {
		return nil, "", nil
	}

	return bestData, bestMime, nil
}

// mimeForFileType converts pdfcpu's image FileType string to a MIME type.
func mimeForFileType(ft string) string {
	switch ft {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "tif", "tiff":
		// TIFF is not commonly used for covers; treat as PNG-ish.
		// In practice pdfcpu rarely returns TIFF for embedded images.
		return "image/tiff"
	default:
		return "image/png"
	}
}

// EnsurePdfiumPoolInit initializes the go-pdfium WASM pool if not already done.
// Safe to call multiple times; initialization happens at most once.
func EnsurePdfiumPoolInit() error {
	pdfiumOnce.Do(initPdfiumPool)
	return pdfiumErr
}

// PdfiumInstance returns a pdfium instance from the shared WASM pool.
// The caller must call instance.Close() when done to return it to the pool.
func PdfiumInstance(timeout time.Duration) (pdfium.Pdfium, error) {
	if err := EnsurePdfiumPoolInit(); err != nil {
		return nil, err
	}
	return pdfiumPool.GetInstance(timeout)
}

// renderPageCover renders page 1 of a PDF to JPEG using go-pdfium WASM.
func renderPageCover(path string) ([]byte, string, error) {
	return RenderPageJPEG(path, 0, 150, 85)
}

// RenderPageJPEG renders a single page of a PDF to JPEG using the shared
// pdfium WASM pool. pageIdx is 0-indexed.
func RenderPageJPEG(path string, pageIdx int, dpi int, quality int) ([]byte, string, error) {
	if pageIdx < 0 {
		return nil, "", errors.Errorf("page %d out of range", pageIdx)
	}

	instance, err := PdfiumInstance(30 * time.Second)
	if err != nil {
		return nil, "", err
	}
	defer instance.Close()

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		FilePath: &path,
	})
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
			Document: doc.Document,
		})
	}()

	pageCountResp, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to get page count")
	}
	if pageIdx >= pageCountResp.PageCount {
		return nil, "", errors.Errorf("page %d out of range (0-%d)", pageIdx, pageCountResp.PageCount-1)
	}

	render, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
		DPI: dpi,
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    pageIdx,
			},
		},
	})
	if err != nil {
		return nil, "", err
	}
	defer render.Cleanup()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, render.Result.Image, &jpeg.Options{Quality: quality}); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), "image/jpeg", nil
}
