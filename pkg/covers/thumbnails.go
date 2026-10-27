package covers

import (
	"bufio"
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // Register GIF cover decoding.
	"image/jpeg"
	_ "image/png" // Register PNG cover decoding.
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // Register WebP cover decoding.
	"golang.org/x/sync/semaphore"
)

// Thumbnail is a resized cover and its representation-specific cache key.
type Thumbnail struct {
	Data []byte
	Key  string
}

// ErrInvalidThumbnailSize rejects sizes outside the fixed cache tiers.
var ErrInvalidThumbnailSize = errors.New("thumbnail size must be 128, 256, 512, 1024, or 2048")

// InvalidThumbnailSizeMessage is the 422 a cover or page route answers a
// size outside the tiers with.
const InvalidThumbnailSizeMessage = "Thumbnail size must be 128, 256, 512, 1024, or 2048."

// ErrUnsupportedThumbnail preserves originals the bounded raster decoder cannot resize.
var ErrUnsupportedThumbnail = errors.New("cover cannot be resized")

// DefaultThumbnailCacheMaxBytes bounds cached thumbnails to 1 GiB.
const DefaultThumbnailCacheMaxBytes = 1 << 30

const maxThumbnailSourcePixels = 32_000_000

// thumbnailDecodeBudget caps the estimated bytes of the resizes running at
// once, so a host with many CPUs and no container CPU limit cannot decode
// enough covers together to exceed a container's memory limit. With Go's
// default GC headroom the heap can reach about twice this.
const thumbnailDecodeBudget = 256 << 20

// thumbnailDecodeWeight is what one resize reserves from the budget: its
// estimated bytes, but at least an equal share per CPU so resizes never
// outnumber CPUs. A resize estimated above the whole budget runs alone.
func thumbnailDecodeWeight(bytes, procs int64) int64 {
	return min(thumbnailDecodeBudget, max(bytes, thumbnailDecodeBudget/max(1, procs)))
}

// thumbnailDecodeBytes estimates the memory resize allocates for a source
// whose header is cfg and whose decoding costs bpp bytes per pixel (see
// decodeBytesPerPixel): the decoded image, each halving, Catmull-Rom's
// scratch buffer (32 bytes per destination column per source row), the
// output and its flattened copy, and the decoder's and encoder's working
// buffers.
func thumbnailDecodeBytes(cfg image.Config, bpp int64, size int, aspect string) int64 {
	total := int64(cfg.Width) * int64(cfg.Height) * bpp
	crop, dw, dh := thumbnailGeometry(cfg.Width, cfg.Height, size, aspect)
	w, h := crop.Dx(), crop.Dy()
	for halveForThumbnail(w, h, dw, dh) {
		w, h = (w+1)/2, (h+1)/2
		total += int64(w) * int64(h) * 4
	}
	if w != dw || h != dh {
		total += int64(dw) * int64(h) * 32
	}
	return total + int64(dw)*int64(dh)*4*3 + 1<<20
}

// decodeBytesPerPixel estimates what decoding r costs per source pixel,
// from its format, color model, and header. Progressive JPEGs keep every DCT
// coefficient (4 bytes per sample per component) beside the image, CMYK and
// RGB-coded JPEGs convert through an intermediate image, and interlaced PNGs
// decode each pass before merging. A header that cannot be read counts as
// the costliest layout.
func decodeBytesPerPixel(r io.ReadSeeker, format string, model color.Model) int64 {
	switch format {
	case "jpeg":
		progressive := jpegIsProgressive(r)
		switch {
		case model == color.CMYKModel && progressive:
			return 26
		case model == color.CMYKModel:
			return 9
		case model == color.GrayModel && progressive:
			return 6
		case model == color.GrayModel:
			return 2
		case model == color.RGBAModel && progressive:
			return 20
		case model == color.RGBAModel:
			return 8
		case progressive:
			return 16
		}
	case "png":
		bpp := int64(4)
		if model == color.RGBA64Model || model == color.NRGBA64Model {
			bpp = 10
		}
		if pngIsInterlaced(r) {
			bpp *= 2
		}
		return bpp
	case "webp":
		if model == color.NYCbCrAModel {
			return 8
		}
	}
	return 4
}

// jpegIsProgressive walks r's marker segments to its start-of-frame marker.
func jpegIsProgressive(r io.ReadSeeker) bool {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return true
	}
	br := bufio.NewReader(r)
	var soi [2]byte
	if _, err := io.ReadFull(br, soi[:]); err != nil || soi != [2]byte{0xFF, 0xD8} {
		return true
	}
	for {
		b, err := br.ReadByte()
		if err != nil || b != 0xFF {
			return true
		}
		marker := byte(0xFF)
		for marker == 0xFF {
			if marker, err = br.ReadByte(); err != nil {
				return true
			}
		}
		switch {
		case marker == 0xC2 || marker == 0xC6 || marker == 0xCA || marker == 0xCE:
			return true
		case marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC:
			return false
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD8):
			continue
		}
		var length [2]byte
		if _, err := io.ReadFull(br, length[:]); err != nil {
			return true
		}
		n := int(length[0])<<8 | int(length[1])
		if n < 2 {
			return true
		}
		if _, err := br.Discard(n - 2); err != nil {
			return true
		}
	}
}

// pngIsInterlaced reads the interlace method from r's IHDR chunk, which the
// PNG format requires to come first.
func pngIsInterlaced(r io.ReadSeeker) bool {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return true
	}
	var header [29]byte
	if _, err := io.ReadFull(r, header[:]); err != nil || string(header[12:16]) != "IHDR" {
		return true
	}
	return header[28] != 0
}

// thumbnailGeometry returns the centered 2:3 (book) or square crop of a
// width x height source, relative to its origin, and the size that crop
// scales to so its longest side fits size without upscaling. The crop matches
// the web UI's object-cover, so a wide source does not become an undersized
// thumbnail in a tall frame.
func thumbnailGeometry(width, height, size int, aspect string) (image.Rectangle, int, int) {
	crop := image.Rect(0, 0, width, height)
	if aspect != "" {
		ratio := 1.0
		if aspect == "book" {
			ratio = 2.0 / 3.0
		}
		if float64(width)/float64(height) > ratio {
			w := max(1, int(math.Round(float64(height)*ratio)))
			crop.Min.X = (width - w) / 2
			crop.Max.X = crop.Min.X + w
		} else {
			h := max(1, int(math.Round(float64(width)/ratio)))
			crop.Min.Y = (height - h) / 2
			crop.Max.Y = crop.Min.Y + h
		}
	}
	scale := math.Min(1, float64(size)/float64(max(crop.Dx(), crop.Dy())))
	return crop, max(1, int(math.Round(float64(crop.Dx())*scale))), max(1, int(math.Round(float64(crop.Dy())*scale)))
}

// halveForThumbnail reports whether a w x h image is still at least four
// times the dw x dh target, so another exact halving comes before
// Catmull-Rom. Leaving Catmull-Rom up to 4x keeps text as sharp as running
// it over the full source, while its scratch buffer stays under 128 bytes
// per destination pixel.
func halveForThumbnail(w, h, dw, dh int) bool {
	return max(w, h) >= 4*max(dw, dh)
}

// thumbnailJPEGQuality keeps cover lettering crisp at small sizes while
// staying several times smaller than PNG.
const thumbnailJPEGQuality = 85

// thumbnailExt is the current file format. legacyThumbnailExt files come from
// an earlier renderer; the index skips them, so loadLocked deletes them rather
// than leave them outside the size limit.
const (
	thumbnailExt       = ".jpg"
	legacyThumbnailExt = ".png"
)

// ErrInvalidThumbnailAspect rejects unbounded crop variants.
var ErrInvalidThumbnailAspect = errors.New("thumbnail aspect must be book or square")

// PageThumbnailKey is the render key a page thumbnail's URL carries as r:
// the page image's own key (the PDF render settings or the CBZ page key)
// and this renderer's, so a change to either fetches the thumbnail anew.
func PageThumbnailKey(pageKey string) string {
	return pageKey + "." + ThumbnailRenderKey
}

// ValidThumbnailSize reports whether size is one of the cache's tiers.
func ValidThumbnailSize(size int) bool {
	return size == ThumbnailSize128 || size == ThumbnailSize256 || size == ThumbnailSize512 || size == ThumbnailSize1024 || size == ThumbnailSize2048
}

type thumbnailEntry struct {
	key     string
	bytes   int64
	touched time.Time
	element *list.Element
}

// ThumbnailCache persists only requested cover sizes. One instance is shared
// by all API cover routes and cache management. Its mutex covers the index,
// publication, and eviction, never cache hit reads, decoding, or resizing.
// Independent generations run at once within the decode budget, at most one
// per GOMAXPROCS (which follows a container's CPU limit, or the host's cores
// without one); duplicate cold requests wait for the same file. The budget
// admits waiters in arrival order, so a large cover waiting for room holds
// back smaller ones queued behind it rather than starving.
type ThumbnailCache struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex
	ready    bool
	entries  map[string]*thumbnailEntry
	lru      list.List
	total    int64
	flights  map[string]chan struct{}
	decodes  *semaphore.Weighted
	epoch    uint64
}

// NewThumbnailCache creates a lazy cache beneath the server's cache directory.
func NewThumbnailCache(dir string, maxBytes int64) *ThumbnailCache {
	return &ThumbnailCache{
		dir:      filepath.Join(dir, "cover-thumbnails"),
		maxBytes: maxBytes,
		flights:  make(map[string]chan struct{}),
		decodes:  semaphore.NewWeighted(thumbnailDecodeBudget),
	}
}

// Get returns a JPEG fitting within size pixels on its longest side, without
// upscaling the original. Returned bytes stay usable even if another request
// evicts the file or an administrator clears the cache before it is served.
func (c *ThumbnailCache) Get(ctx context.Context, file *models.File, size int) (Thumbnail, error) {
	return c.GetForAspect(ctx, file, size, "")
}

// GetForAspect applies the same centered crop as a book or square cover frame.
func (c *ThumbnailCache) GetForAspect(ctx context.Context, file *models.File, size int, aspect string) (Thumbnail, error) {
	if aspect != "" && aspect != "book" && aspect != "square" {
		return Thumbnail{}, ErrInvalidThumbnailAspect
	}
	cover := FileCoverPath(file)
	return c.get(ctx, strconv.Itoa(file.ID), cover, "", cover, size, aspect)
}

// PageSource is the book file a page comes from, which versions the page's
// thumbnails: its path, and the key of what decides which image a page
// number names (cbzpages.CBZPageKey).
type PageSource struct {
	Path string
	Key  string
}

// GetForPage returns a JPEG of a CBZ page, decoded from the copy the page
// cache extracted at pagePath, fitting within size pixels on its longest
// side and keeping the page's shape. Each page is cached on its own beside
// the file's cover thumbnails, under the same budget, and is cleared with
// them. Its version follows the book file, not pagePath, whose mtime the page
// cache bumps as the copy is used.
func (c *ThumbnailCache) GetForPage(ctx context.Context, fileID int, page int, source PageSource, pagePath string, size int) (Thumbnail, error) {
	return c.get(ctx, pageOwner(fileID, page), source.Path, "#"+source.Key, pagePath, size, "")
}

// pageOwner names a page's thumbnails in their keys, as "{fileID}p{page}",
// apart from the file's cover thumbnails, which are keyed by the file ID.
func pageOwner(fileID int, page int) string {
	return strconv.Itoa(fileID) + "p" + strconv.Itoa(page)
}

// validOwner reports whether a key's first part names a cover ("12") or a
// page ("12p3").
func validOwner(owner string) bool {
	fileID, page, isPage := strings.Cut(owner, "p")
	if _, err := strconv.Atoi(fileID); err != nil {
		return false
	}
	if !isPage {
		return true
	}
	_, err := strconv.Atoi(page)
	return err == nil
}

// get returns the thumbnail for owner of the image at decodePath, generating
// and publishing it when no current one is cached. The version comes from
// versionPath (its path, mtime, and size) and versionSuffix; publishing
// removes owner's thumbnails of earlier versions.
func (c *ThumbnailCache) get(ctx context.Context, owner, versionPath, versionSuffix, decodePath string, size int, aspect string) (Thumbnail, error) {
	if !ValidThumbnailSize(size) {
		return Thumbnail{}, ErrInvalidThumbnailSize
	}
	if err := ctx.Err(); err != nil {
		return Thumbnail{}, errors.WithStack(err)
	}
	// The source version is the path, nanosecond mtime, byte size, and render
	// key, plus the owner (the file ID, or file and page) in the key prefix,
	// so a metadata-only updated_at change does not regenerate thumbnails.
	info, err := os.Stat(versionPath)
	if err != nil {
		return Thumbnail{}, errors.WithStack(err)
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%d", ThumbnailRenderKey, versionPath+versionSuffix, info.ModTime().UnixNano(), info.Size())))
	version := fmt.Sprintf("%s-%x-", owner, hash[:16])
	kind := aspect
	if kind == "" {
		kind = "original"
	}
	key := version + strconv.Itoa(size) + "-" + kind
	c.mu.Lock()
	epoch := c.epoch
	c.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return Thumbnail{}, errors.WithStack(err)
		}
		c.mu.Lock()
		if err := c.loadLocked(); err != nil {
			c.mu.Unlock()
			return Thumbnail{}, err
		}
		if entry, touch := c.hitLocked(key); entry != nil {
			c.mu.Unlock()
			data, err := c.read(entry, touch)
			if err != nil || data != nil {
				return Thumbnail{Data: data, Key: key}, err
			}
			continue
		}
		if done, ok := c.flights[key]; ok {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return Thumbnail{}, errors.WithStack(ctx.Err())
			case <-done:
				continue
			}
		}
		done := make(chan struct{})
		c.flights[key] = done
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			delete(c.flights, key)
			close(done)
			c.mu.Unlock()
		}()
		data, err := c.resize(ctx, decodePath, size, aspect)
		if err != nil {
			return Thumbnail{}, err
		}
		current, err := os.Stat(versionPath)
		if err != nil {
			return Thumbnail{}, errors.WithStack(err)
		}
		if current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
			return Thumbnail{}, errors.Wrap(ErrUnsupportedThumbnail, "cover changed during generation")
		}
		c.mu.Lock()
		// Clear changes the epoch. Work started before it must not repopulate
		// the cache after the administrator's clear has completed.
		if err = ctx.Err(); err != nil {
			err = errors.WithStack(err)
		} else if c.epoch == epoch {
			err = c.publishLocked(key, version, owner, data)
		}
		c.mu.Unlock()
		return Thumbnail{Data: data, Key: key}, err
	}
}

// resize decodes the cover (bounded to maxThumbnailSourcePixels) once its
// estimated memory fits in the decode budget, then crops and scales it as
// thumbnailGeometry describes. Exact halvings with bilinear sampling average
// each 2x2 block until the image is within 4x of the target, and Catmull-Rom
// does the rest: fine detail is averaged rather than dropped, at a fraction
// of the cost of running Catmull-Rom over the full source. Covers are
// expected to be opaque; any transparency is flattened onto white because
// JPEG has no alpha.
func (c *ThumbnailCache) resize(ctx context.Context, path string, size int, aspect string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		return nil, errors.Wrap(ErrUnsupportedThumbnail, err.Error())
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxThumbnailSourcePixels/cfg.Height {
		return nil, errors.Wrap(ErrUnsupportedThumbnail, "cover exceeds thumbnail decode limit")
	}
	bpp := decodeBytesPerPixel(f, format, cfg.ColorModel)
	weight := thumbnailDecodeWeight(thumbnailDecodeBytes(cfg, bpp, size, aspect), int64(runtime.GOMAXPROCS(0)))
	if err := c.decodes.Acquire(ctx, weight); err != nil {
		return nil, errors.WithStack(err)
	}
	defer c.decodes.Release(weight)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, errors.WithStack(err)
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, errors.Wrap(ErrUnsupportedThumbnail, err.Error())
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	crop, width, height := thumbnailGeometry(img.Bounds().Dx(), img.Bounds().Dy(), size, aspect)
	src, sr := img, crop.Add(img.Bounds().Min)
	for halveForThumbnail(sr.Dx(), sr.Dy(), width, height) {
		half := image.NewRGBA(image.Rect(0, 0, (sr.Dx()+1)/2, (sr.Dy()+1)/2))
		draw.ApproxBiLinear.Scale(half, half.Bounds(), src, sr, draw.Src, nil)
		src, sr = half, half.Bounds()
		if err := ctx.Err(); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	if sr.Size() == dst.Rect.Size() {
		draw.Draw(dst, dst.Bounds(), src, sr.Min, draw.Src)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, sr, draw.Src, nil)
	}
	if !dst.Opaque() {
		flat := image.NewRGBA(dst.Bounds())
		draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), dst, image.Point{}, draw.Over)
		dst = flat
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: thumbnailJPEGQuality}); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	return buf.Bytes(), nil
}

// loadLocked indexes the directory once, including after a server restart.
// Later hits and publications use the index instead of walking the disk.
func (c *ThumbnailCache) loadLocked() error {
	if c.ready {
		return nil
	}
	files, err := os.ReadDir(c.dir)
	if err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	c.entries = make(map[string]*thumbnailEntry)
	c.lru.Init()
	c.total = 0
	var entries []*thumbnailEntry
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), legacyThumbnailExt) {
			// Best effort: a file that cannot be removed is retried after the
			// next restart and must not take thumbnails offline meanwhile.
			_ = os.Remove(filepath.Join(c.dir, file.Name()))
			continue
		}
		if file.IsDir() || !strings.HasSuffix(file.Name(), thumbnailExt) {
			continue
		}
		key := strings.TrimSuffix(file.Name(), thumbnailExt)
		parts := strings.Split(key, "-")
		if len(parts) != 4 {
			continue
		}
		if parts[3] != "original" && parts[3] != "book" && parts[3] != "square" {
			continue
		}
		if !validOwner(parts[0]) {
			continue
		}
		size, err := strconv.Atoi(parts[2])
		if err != nil || !ValidThumbnailSize(size) {
			continue
		}
		info, err := file.Info()
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return errors.WithStack(err)
		}
		entries = append(entries, &thumbnailEntry{key: key, bytes: info.Size(), touched: info.ModTime()})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].touched.Before(entries[j].touched) })
	for _, entry := range entries {
		c.addLocked(entry)
	}
	c.ready = true
	return c.evictLocked()
}

func (c *ThumbnailCache) path(key string) string { return filepath.Join(c.dir, key+thumbnailExt) }

func (c *ThumbnailCache) addLocked(entry *thumbnailEntry) {
	entry.element = c.lru.PushBack(entry.key)
	c.entries[entry.key] = entry
	c.total += entry.bytes
}

func (c *ThumbnailCache) forgetLocked(entry *thumbnailEntry) {
	c.lru.Remove(entry.element)
	delete(c.entries, entry.key)
	c.total -= entry.bytes
}

// hitLocked marks an indexed thumbnail as recently used and reports whether
// its file's mtime should be refreshed. Persisting approximate recency for
// restarts avoids a metadata write on every gallery hit; in-process LRU order
// is updated on every access.
func (c *ThumbnailCache) hitLocked(key string) (*thumbnailEntry, bool) {
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToBack(entry.element)
	now := time.Now()
	if now.Sub(entry.touched) < time.Minute {
		return entry, false
	}
	entry.touched = now
	return entry, true
}

// read loads a hit without holding the mutex, so a page of cached covers is
// read in parallel. Files are published by atomic rename, so a read sees a
// whole thumbnail or none. Eviction, a newer cover version, or Clear can
// delete the file first; read then drops the entry if the index still holds
// it and returns nil data so the caller treats the request as a miss.
func (c *ThumbnailCache) read(entry *thumbnailEntry, touch bool) ([]byte, error) {
	path := c.path(entry.key)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c.mu.Lock()
		if c.entries[entry.key] == entry {
			c.forgetLocked(entry)
		}
		c.mu.Unlock()
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if touch {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
	return data, nil
}

func (c *ThumbnailCache) publishLocked(key, version, owner string, data []byte) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return errors.WithStack(err)
	}
	if err := fileutils.WriteFileAtomic(c.path(key), data, 0644); err != nil {
		return err
	}
	c.addLocked(&thumbnailEntry{key: key, bytes: int64(len(data)), touched: time.Now()})
	prefix := owner + "-"
	for other, entry := range c.entries {
		if strings.HasPrefix(other, prefix) && !strings.HasPrefix(other, version) {
			if err := c.removeLocked(entry); err != nil {
				return err
			}
		}
	}
	return c.evictLocked()
}

func (c *ThumbnailCache) removeLocked(entry *thumbnailEntry) error {
	if err := os.Remove(c.path(entry.key)); err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	c.forgetLocked(entry)
	return nil
}

func (c *ThumbnailCache) evictLocked() error {
	for c.total > c.maxBytes && c.lru.Len() > 0 {
		key := c.lru.Front().Value.(string)
		if err := c.removeLocked(c.entries[key]); err != nil {
			return err
		}
	}
	return nil
}

// SizeBytes reports cached files without decoding covers or generating sizes.
func (c *ThumbnailCache) SizeBytes() (int64, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLocked(); err != nil {
		return 0, 0, err
	}
	return c.total, len(c.entries), nil
}

// SetMaxBytes changes the live quota and evicts the oldest thumbnails now.
// In-flight generations use the new quota when they publish.
func (c *ThumbnailCache) SetMaxBytes(maxBytes int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxBytes = max(0, maxBytes)
	if err := c.loadLocked(); err != nil {
		return err
	}
	return c.evictLocked()
}

// Clear removes generated thumbnails; source covers are never modified.
func (c *ThumbnailCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.RemoveAll(c.dir); err != nil {
		return errors.WithStack(err)
	}
	c.epoch++
	c.ready = true
	c.entries = make(map[string]*thumbnailEntry)
	c.lru.Init()
	c.total = 0
	return nil
}
