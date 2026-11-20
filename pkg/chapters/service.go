package chapters

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// ShouldUpdateChapters determines if chapters should be updated based on priority rules.
// Returns true if the new chapters should replace existing ones.
//
// When forceRefresh is true, sidecar-sourced chapters are skipped so that the
// embedded file metadata wins — matching the convention used by
// shouldApplySidecarScalar / shouldApplySidecarRelationship in pkg/worker.
// All non-sidecar sources still apply unconditionally under forceRefresh.
func ShouldUpdateChapters(newChapters []mediafile.ParsedChapter, newSource string, existingSource *string, forceRefresh bool) bool {
	// Never update with empty chapters
	if len(newChapters) == 0 {
		return false
	}

	if forceRefresh {
		// Skip sidecar so embedded file metadata wins; everything else applies.
		return newSource != models.DataSourceSidecar
	}

	// Treat nil/empty existing source as filepath priority (lowest priority)
	existingSourceValue := ""
	if existingSource != nil {
		existingSourceValue = *existingSource
	}
	if existingSourceValue == "" {
		existingSourceValue = models.DataSourceFilepath
	}

	newPriority := models.GetDataSourcePriority(newSource)
	existingPriority := models.GetDataSourcePriority(existingSourceValue)

	// Higher or equal priority wins (lower number = higher priority)
	return newPriority <= existingPriority
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// ListChapters retrieves all chapters for a file, building nested structure.
func (svc *Service) ListChapters(ctx context.Context, fileID int) ([]*models.Chapter, error) {
	var chapters []*models.Chapter
	err := svc.db.NewSelect().
		Model(&chapters).
		Where("file_id = ?", fileID).
		Order("sort_order ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return buildChapterTree(chapters), nil
}

// ReplaceChapters makes a file's chapters match chapters, and reports
// whether any row changed. Rows are updated in place, so a Resync that reads
// the same chapters writes nothing (ADR 0023).
func (svc *Service) ReplaceChapters(ctx context.Context, fileID int, chapters []mediafile.ParsedChapter) (bool, error) {
	var changed bool
	err := svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		existing, err := lockChapters(ctx, tx, fileID)
		if err != nil {
			return err
		}
		changed, err = replaceChapters(ctx, tx, fileID, existing, chapters)
		return err
	})
	return changed, err
}

// ReplaceChaptersWithSource replaces a file's chapters and records their
// source in one transaction, so the chapters are never stored under a source
// that would let a scan replace them.
func (svc *Service) ReplaceChaptersWithSource(ctx context.Context, fileID int, chapters []mediafile.ParsedChapter, source string) error {
	return svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		existing, err := lockChapters(ctx, tx, fileID)
		if err != nil {
			return err
		}
		if _, err := replaceChapters(ctx, tx, fileID, existing, chapters); err != nil {
			return err
		}
		_, err = tx.NewUpdate().
			Model((*models.File)(nil)).
			Set("chapter_source = ?", source).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", fileID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// lockChapters reads a file's chapters inside tx after taking the write
// lock, so a concurrent replace can't work from the same stale rows, and the
// transaction never has to upgrade from reading to writing, which SQLite
// refuses once another connection has written. The write rewrites a column
// with its own value, which changes nothing and records nothing in the
// change log.
func lockChapters(ctx context.Context, tx bun.Tx, fileID int) ([]*models.Chapter, error) {
	_, err := tx.NewUpdate().
		Model((*models.File)(nil)).
		Set("chapter_source = chapter_source").
		Where("id = ?", fileID).
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var existing []*models.Chapter
	if err := tx.NewSelect().Model(&existing).Where("file_id = ?", fileID).Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return existing, nil
}

// chapterPosition is where a chapter sits: under its parent, at its index.
type chapterPosition struct {
	parentID  int
	sortOrder int
}

// replaceChapters updates the file's chapter at each position to the new
// one there, inserts chapters at new positions, and deletes the rest of
// existing, and reports whether it wrote anything.
func replaceChapters(ctx context.Context, tx bun.Tx, fileID int, existing []*models.Chapter, chapters []mediafile.ParsedChapter) (bool, error) {
	byPosition := make(map[chapterPosition]*models.Chapter, len(existing))
	for _, ch := range existing {
		parent := 0
		if ch.ParentID != nil {
			parent = *ch.ParentID
		}
		byPosition[chapterPosition{parentID: parent, sortOrder: ch.SortOrder}] = ch
	}

	sync := &chapterSync{tx: tx, fileID: fileID, byPosition: byPosition, kept: map[int]bool{}, now: time.Now()}
	if err := sync.level(ctx, nil, chapters); err != nil {
		return false, err
	}

	var stale []int
	for _, ch := range existing {
		if !sync.kept[ch.ID] {
			stale = append(stale, ch.ID)
		}
	}
	if len(stale) > 0 {
		if _, err := tx.NewDelete().Model((*models.Chapter)(nil)).Where("id IN (?)", bun.List(stale)).Exec(ctx); err != nil {
			return false, errors.WithStack(err)
		}
		sync.changed = true
	}
	return sync.changed, nil
}

type chapterSync struct {
	tx         bun.Tx
	fileID     int
	byPosition map[chapterPosition]*models.Chapter
	kept       map[int]bool
	now        time.Time
	changed    bool
}

// level syncs the chapters under parentID, then each one's children.
func (s *chapterSync) level(ctx context.Context, parentID *int, chapters []mediafile.ParsedChapter) error {
	parent := 0
	if parentID != nil {
		parent = *parentID
	}
	for i, ch := range chapters {
		row, ok := s.byPosition[chapterPosition{parentID: parent, sortOrder: i}]
		switch {
		case !ok:
			row = &models.Chapter{
				CreatedAt:        s.now,
				UpdatedAt:        s.now,
				FileID:           s.fileID,
				ParentID:         parentID,
				SortOrder:        i,
				Title:            ch.Title,
				StartPage:        ch.StartPage,
				StartTimestampMs: ch.StartTimestampMs,
				Href:             ch.Href,
			}
			if _, err := s.tx.NewInsert().Model(row).Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
			s.changed = true
		case row.Title != ch.Title || !equalPtr(row.StartPage, ch.StartPage) ||
			!equalPtr(row.StartTimestampMs, ch.StartTimestampMs) || !equalPtr(row.Href, ch.Href):
			row.Title = ch.Title
			row.StartPage = ch.StartPage
			row.StartTimestampMs = ch.StartTimestampMs
			row.Href = ch.Href
			row.UpdatedAt = s.now
			_, err := s.tx.NewUpdate().Model(row).
				Column("title", "start_page", "start_timestamp_ms", "href", "updated_at").
				WherePK().
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
			s.changed = true
		}
		s.kept[row.ID] = true
		if err := s.level(ctx, &row.ID, ch.Children); err != nil {
			return err
		}
	}
	return nil
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// DeleteChaptersForFile deletes all chapters for a file.
func (svc *Service) DeleteChaptersForFile(ctx context.Context, fileID int) error {
	_, err := svc.db.NewDelete().
		Model((*models.Chapter)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	return errors.WithStack(err)
}

// buildChapterTree converts a flat list of chapters into a nested tree.
func buildChapterTree(chapters []*models.Chapter) []*models.Chapter {
	// Build lookup map
	byID := make(map[int]*models.Chapter)
	for _, ch := range chapters {
		ch.Children = []*models.Chapter{} // Initialize empty slice
		byID[ch.ID] = ch
	}

	// Build tree
	roots := make([]*models.Chapter, 0)
	for _, ch := range chapters {
		if ch.ParentID == nil {
			roots = append(roots, ch)
		} else if parent, ok := byID[*ch.ParentID]; ok {
			parent.Children = append(parent.Children, ch)
		}
	}

	return roots
}
