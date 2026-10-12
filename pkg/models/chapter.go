package models

import (
	"slices"
	"sort"
	"time"

	"github.com/uptrace/bun"
)

type Chapter struct {
	bun.BaseModel `bun:"table:chapters,alias:ch" tstype:"-"`

	ID        int       `bun:",pk,autoincrement" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	FileID    int       `bun:",notnull" json:"file_id"`
	ParentID  *int      `json:"parent_id"`
	SortOrder int       `bun:",notnull" json:"sort_order"`
	Title     string    `bun:",notnull" json:"title"`

	// Position data (mutually exclusive based on file type)
	StartPage        *int    `json:"start_page"`         // CBZ/PDF: 0-indexed page number
	StartTimestampMs *int64  `json:"start_timestamp_ms"` // M4B: milliseconds from start
	Href             *string `json:"href"`               // EPUB: content document href; MOBI/AZW3: location, shown only

	// Relations
	File     *File      `bun:"rel:belongs-to,join:file_id=id" json:"-"`
	Parent   *Chapter   `bun:"rel:belongs-to,join:parent_id=id" json:"-"`
	Children []*Chapter `bun:"rel:has-many,join:id=parent_id" json:"children,omitempty"`
}

// ChapterTree returns the chapters as a tree of top-level chapters. A File
// loaded by RetrieveFileWithRelations lists every chapter in Chapters, nested
// ones included, with only copies of their direct children attached, so a
// list holding a chapter's parent is rebuilt from ParentID. A list of
// top-level chapters with their children attached is returned as it is.
func ChapterTree(chapters []*Chapter) []*Chapter {
	listed := make(map[int]bool, len(chapters))
	for _, ch := range chapters {
		if ch.ID != 0 {
			listed[ch.ID] = true
		}
	}
	hasParent := func(ch *Chapter) bool {
		return ch.ParentID != nil && listed[*ch.ParentID]
	}
	if !slices.ContainsFunc(chapters, hasParent) {
		return chapters
	}

	byParent := map[int][]*Chapter{}
	var roots []*Chapter
	for _, ch := range chapters {
		if hasParent(ch) {
			byParent[*ch.ParentID] = append(byParent[*ch.ParentID], ch)
		} else {
			roots = append(roots, ch)
		}
	}
	var build func(level []*Chapter) []*Chapter
	build = func(level []*Chapter) []*Chapter {
		level = slices.Clone(level)
		sort.SliceStable(level, func(i, j int) bool { return level[i].SortOrder < level[j].SortOrder })
		out := make([]*Chapter, 0, len(level))
		for _, ch := range level {
			node := *ch
			node.Children = build(byParent[ch.ID])
			out = append(out, &node)
		}
		return out
	}
	return build(roots)
}
