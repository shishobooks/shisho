package covers

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/gif"  // Register GIF cover decoding.
	_ "image/jpeg" // Register JPEG cover decoding.
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
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
)

// Thumbnail is a resized cover and its representation-specific cache key.
type Thumbnail struct {
	Data []byte
	Key  string
}

// ErrInvalidThumbnailSize rejects sizes outside the fixed cache tiers.
var ErrInvalidThumbnailSize = errors.New("thumbnail size must be 128, 256, 512, 1024, or 2048")

// ErrUnsupportedThumbnail preserves originals the bounded raster decoder cannot resize.
var ErrUnsupportedThumbnail = errors.New("cover cannot be resized")

// DefaultThumbnailCacheMaxBytes bounds cached thumbnails to 256 MiB.
const DefaultThumbnailCacheMaxBytes = 256 << 20

const maxThumbnailSourcePixels = 32_000_000

// ErrInvalidThumbnailAspect rejects unbounded crop variants.
var ErrInvalidThumbnailAspect = errors.New("thumbnail aspect must be book or square")

func validThumbnailSize(size int) bool {
	return size == ThumbnailSize128 || size == ThumbnailSize256 || size == ThumbnailSize512 || size == ThumbnailSize1024 || size == ThumbnailSize2048
}

type thumbnailEntry struct {
	key     string
	bytes   int64
	touched time.Time
	element *list.Element
}

// ThumbnailCache persists only requested cover sizes. One instance is shared
// by all API cover routes and cache management. Its mutex covers disk reads,
// publication, and eviction, never decoding or resizing. Two independent
// generations can run at once; duplicate cold requests wait for the same file.
type ThumbnailCache struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex
	ready    bool
	entries  map[string]*thumbnailEntry
	lru      list.List
	total    int64
	flights  map[string]chan struct{}
	slots    chan struct{}
	epoch    uint64
}

// NewThumbnailCache creates a lazy cache beneath the server's cache directory.
func NewThumbnailCache(dir string, maxBytes int64) *ThumbnailCache {
	return &ThumbnailCache{
		dir:      filepath.Join(dir, "cover-thumbnails"),
		maxBytes: maxBytes,
		flights:  make(map[string]chan struct{}),
		slots:    make(chan struct{}, 2),
	}
}

// Get returns a PNG fitting within size pixels on its longest side, without
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
	if !validThumbnailSize(size) {
		return Thumbnail{}, ErrInvalidThumbnailSize
	}
	if err := ctx.Err(); err != nil {
		return Thumbnail{}, errors.WithStack(err)
	}
	source := FileCoverPath(file)
	info, err := os.Stat(source)
	if err != nil {
		return Thumbnail{}, errors.WithStack(err)
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%d", ThumbnailRenderKey, source, info.ModTime().UnixNano(), info.Size())))
	version := fmt.Sprintf("%d-%x-", file.ID, hash[:16])
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
		data, err := c.readLocked(key)
		if err != nil || data != nil {
			c.mu.Unlock()
			return Thumbnail{Data: data, Key: key}, err
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
		select {
		case <-ctx.Done():
			return Thumbnail{}, errors.WithStack(ctx.Err())
		case c.slots <- struct{}{}:
		}
		data, err = resizeThumbnail(ctx, source, size, aspect)
		<-c.slots
		if err != nil {
			return Thumbnail{}, err
		}
		current, err := os.Stat(source)
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
			err = c.publishLocked(key, version, file.ID, data)
		}
		c.mu.Unlock()
		return Thumbnail{Data: data, Key: key}, err
	}
}

func resizeThumbnail(ctx context.Context, path string, size int, aspect string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, errors.Wrap(ErrUnsupportedThumbnail, err.Error())
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxThumbnailSourcePixels/cfg.Height {
		return nil, errors.Wrap(ErrUnsupportedThumbnail, "cover exceeds thumbnail decode limit")
	}
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
	bounds := img.Bounds()
	if aspect != "" {
		ratio := 1.0
		if aspect == "book" {
			ratio = 2.0 / 3.0
		}
		width, height := bounds.Dx(), bounds.Dy()
		if float64(width)/float64(height) > ratio {
			crop := max(1, int(math.Round(float64(height)*ratio)))
			bounds.Min.X += (width - crop) / 2
			bounds.Max.X = bounds.Min.X + crop
		} else {
			crop := max(1, int(math.Round(float64(width)/ratio)))
			bounds.Min.Y += (height - crop) / 2
			bounds.Max.Y = bounds.Min.Y + crop
		}
	}
	scale := math.Min(1, float64(size)/float64(max(bounds.Dx(), bounds.Dy())))
	width := max(1, int(math.Round(float64(bounds.Dx())*scale)))
	height := max(1, int(math.Round(float64(bounds.Dy())*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
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
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".png") {
			continue
		}
		key := strings.TrimSuffix(file.Name(), ".png")
		parts := strings.Split(key, "-")
		if len(parts) != 4 {
			continue
		}
		if parts[3] != "original" && parts[3] != "book" && parts[3] != "square" {
			continue
		}
		if _, err := strconv.Atoi(parts[0]); err != nil {
			continue
		}
		size, err := strconv.Atoi(parts[2])
		if err != nil || !validThumbnailSize(size) {
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

func (c *ThumbnailCache) path(key string) string { return filepath.Join(c.dir, key+".png") }

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

func (c *ThumbnailCache) readLocked(key string) ([]byte, error) {
	entry, ok := c.entries[key]
	if !ok {
		return nil, nil
	}
	data, err := os.ReadFile(c.path(key))
	if os.IsNotExist(err) {
		c.forgetLocked(entry)
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	c.lru.MoveToBack(entry.element)
	// Persist approximate recency for restarts without a metadata write on
	// every gallery hit. In-process LRU order is updated on every access.
	now := time.Now()
	if now.Sub(entry.touched) >= time.Minute {
		if err := os.Chtimes(c.path(key), now, now); err == nil {
			entry.touched = now
		}
	}
	return data, nil
}

func (c *ThumbnailCache) publishLocked(key, version string, fileID int, data []byte) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return errors.WithStack(err)
	}
	if err := fileutils.WriteFileAtomic(c.path(key), data, 0644); err != nil {
		return err
	}
	c.addLocked(&thumbnailEntry{key: key, bytes: int64(len(data)), touched: time.Now()})
	prefix := strconv.Itoa(fileID) + "-"
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
