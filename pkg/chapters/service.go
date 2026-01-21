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
func ShouldUpdateChapters(newChapters []mediafile.ParsedChapter, newSource string, existingSource *string, forceRefresh bool) bool {
	// Never update with empty chapters
	if len(newChapters) == 0 {
		return false
	}

	// Force refresh bypasses priority checks
	if forceRefresh {
		return true
	}

	// Treat nil/empty existing source as filepath priority (lowest priority)
	existingSourceValue := ""
	if existingSource != nil {
		existingSourceValue = *existingSource
	}
	if existingSourceValue == "" {
		existingSourceValue = models.DataSourceFilepath
	}

	newPriority := models.DataSourcePriority[newSource]
	existingPriority := models.DataSourcePriority[existingSourceValue]

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

// ReplaceChapters deletes all existing chapters for a file and inserts new ones.
func (svc *Service) ReplaceChapters(ctx context.Context, fileID int, chapters []mediafile.ParsedChapter) error {
	return svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Delete existing chapters
		_, err := tx.NewDelete().
			Model((*models.Chapter)(nil)).
			Where("file_id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Insert new chapters
		return insertChapters(ctx, tx, fileID, nil, chapters)
	})
}

// DeleteChaptersForFile deletes all chapters for a file.
func (svc *Service) DeleteChaptersForFile(ctx context.Context, fileID int) error {
	_, err := svc.db.NewDelete().
		Model((*models.Chapter)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	return errors.WithStack(err)
}

// insertChapters recursively inserts chapters with their children.
func insertChapters(ctx context.Context, tx bun.Tx, fileID int, parentID *int, chapters []mediafile.ParsedChapter) error {
	now := time.Now()
	for i, ch := range chapters {
		model := &models.Chapter{
			CreatedAt:        now,
			UpdatedAt:        now,
			FileID:           fileID,
			ParentID:         parentID,
			SortOrder:        i,
			Title:            ch.Title,
			StartPage:        ch.StartPage,
			StartTimestampMs: ch.StartTimestampMs,
			Href:             ch.Href,
		}

		_, err := tx.NewInsert().Model(model).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Recursively insert children
		if len(ch.Children) > 0 {
			if err := insertChapters(ctx, tx, fileID, &model.ID, ch.Children); err != nil {
				return err
			}
		}
	}
	return nil
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
