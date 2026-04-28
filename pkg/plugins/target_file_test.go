package plugins

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestResolveTargetFile(t *testing.T) {
	t.Parallel()

	intPtr := func(i int) *int { return &i }
	main := &models.File{ID: 1, FileRole: models.FileRoleMain}
	supp := &models.File{ID: 2, FileRole: models.FileRoleSupplement}
	main2 := &models.File{ID: 3, FileRole: models.FileRoleMain}

	tests := []struct {
		name    string
		files   []*models.File
		fileID  *int
		wantID  int // 0 means nil
		wantNil bool
	}{
		{
			name:   "fileID hit returns that file",
			files:  []*models.File{supp, main, main2},
			fileID: intPtr(2),
			wantID: 2, // supplement is returned when explicitly requested
		},
		{
			name:    "fileID miss returns nil",
			files:   []*models.File{main},
			fileID:  intPtr(99),
			wantNil: true,
		},
		{
			name:   "no fileID picks first main, skipping leading supplement",
			files:  []*models.File{supp, main, main2},
			fileID: nil,
			wantID: 1,
		},
		{
			name:   "no fileID with single main picks it",
			files:  []*models.File{main},
			fileID: nil,
			wantID: 1,
		},
		{
			name:    "no fileID with only supplements returns nil",
			files:   []*models.File{supp},
			fileID:  nil,
			wantNil: true,
		},
		{
			name:    "empty files returns nil",
			files:   nil,
			fileID:  nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveTargetFile(tt.files, tt.fileID)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			if assert.NotNil(t, got) {
				assert.Equal(t, tt.wantID, got.ID)
			}
		})
	}
}
