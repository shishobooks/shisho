// Package fingerprints provides CRUD operations on the file_fingerprints table.
//
// A file_fingerprint row stores one algorithm's fingerprint for one file.
// The MVP only writes sha256 rows for exact-content move/rename detection,
// but the schema is already shape-compatible with future fuzzy algorithms
// (cover pHash, text SimHash, Chromaprint, etc.) so they can reuse the same
// table, service, and generation job.
package fingerprints

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// Insert upserts a fingerprint for (file_id, algorithm). If a row already
// exists for this pair the call is a no-op (ON CONFLICT DO NOTHING) — callers
// that need to replace a stale value should DeleteForFile first.
func (svc *Service) Insert(ctx context.Context, fileID int, algorithm, value string) error {
	fp := &models.FileFingerprint{
		FileID:    fileID,
		Algorithm: algorithm,
		Value:     value,
		CreatedAt: time.Now(),
	}
	_, err := svc.db.
		NewInsert().
		Model(fp).
		On("CONFLICT (file_id, algorithm) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// FindFilesByHash returns all files in the library whose fingerprint for the
// given algorithm matches value. Used for move detection.
func (svc *Service) FindFilesByHash(ctx context.Context, libraryID int, algorithm, value string) ([]*models.File, error) {
	var files []*models.File
	err := svc.db.
		NewSelect().
		Model(&files).
		Join("JOIN file_fingerprints AS ffp ON ffp.file_id = f.id").
		Where("ffp.algorithm = ?", algorithm).
		Where("ffp.value = ?", value).
		Where("f.library_id = ?", libraryID).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	return files, nil
}

// DeleteForFile removes all fingerprints for a file. Called when a file's
// content changes (size/mtime mismatch during rescan) so the next hash
// generation job recomputes a fresh fingerprint.
func (svc *Service) DeleteForFile(ctx context.Context, fileID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.FileFingerprint)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// CountForFile returns how many fingerprints exist for a file (across all algorithms).
func (svc *Service) CountForFile(ctx context.Context, fileID int) (int, error) {
	count, err := svc.db.
		NewSelect().
		Model((*models.FileFingerprint)(nil)).
		Where("file_id = ?", fileID).
		Count(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return count, nil
}

// ListFilesMissingAlgorithm returns IDs of files in the library that do not
// yet have a fingerprint for the given algorithm. Used by the hash generation
// job to determine what work needs doing.
//
// Includes supplement files by design: supplements are real files on disk and
// benefit from content-hash move/rename detection just like main files.
// Callers that only want main files should filter the result in Go.
func (svc *Service) ListFilesMissingAlgorithm(ctx context.Context, libraryID int, algorithm string) ([]int, error) {
	var ids []int
	err := svc.db.
		NewSelect().
		Model((*models.File)(nil)).
		Column("f.id").
		Join("LEFT JOIN file_fingerprints AS ffp ON ffp.file_id = f.id AND ffp.algorithm = ?", algorithm).
		Where("f.library_id = ?", libraryID).
		Where("ffp.id IS NULL").
		Scan(ctx, &ids)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	return ids, nil
}

// ListForFile returns all fingerprints for a file matching the given algorithm.
// Used by the scan move reconciliation phase to look up each orphan's stored sha256.
func (svc *Service) ListForFile(ctx context.Context, fileID int, algorithm string) ([]*models.FileFingerprint, error) {
	var fps []*models.FileFingerprint
	err := svc.db.
		NewSelect().
		Model(&fps).
		Where("file_id = ?", fileID).
		Where("algorithm = ?", algorithm).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	return fps, nil
}
