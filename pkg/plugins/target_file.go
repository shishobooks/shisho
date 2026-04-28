package plugins

import "github.com/shishobooks/shisho/pkg/models"

// resolveTargetFile picks the file a per-book plugin operation should target.
// When fileID is non-nil, the matching file is returned (or nil if absent).
// When fileID is nil, the first file with FileRoleMain is returned —
// supplements never represent the book and must not be the implicit target
// for book-level operations like enrichment search or apply-metadata.
// Returns nil when nothing suitable is available.
func resolveTargetFile(files []*models.File, fileID *int) *models.File {
	if fileID != nil {
		for _, f := range files {
			if f.ID == *fileID {
				return f
			}
		}
		return nil
	}
	for _, f := range files {
		if f.FileRole == models.FileRoleMain {
			return f
		}
	}
	return nil
}
