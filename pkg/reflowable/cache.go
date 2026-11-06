package reflowable

import (
	"container/list"
	"sync"
	"time"
)

// cachedBooks is how many parsed Books Open keeps. Parsing a long KePub
// variant takes tens of milliseconds and tens of megabytes, and a pull or a
// reader opening reads the same few books again and again.
const cachedBooks = 8

type cacheKey struct {
	path     string
	size     int64
	modified time.Time
	kobo     bool
}

// bookCache keeps the Books read most recently.
type bookCache struct {
	mu      sync.Mutex
	order   *list.List
	entries map[cacheKey]*list.Element
}

type cacheEntry struct {
	key  cacheKey
	book *Book
}

var books = &bookCache{order: list.New(), entries: map[cacheKey]*list.Element{}}

func (c *bookCache) get(key cacheKey) *Book {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*cacheEntry).book
	}
	return nil
}

func (c *bookCache) put(key cacheKey, book *Book) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)
		return
	}
	c.entries[key] = c.order.PushFront(&cacheEntry{key: key, book: book})
	for c.order.Len() > cachedBooks {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*cacheEntry).key)
	}
}
