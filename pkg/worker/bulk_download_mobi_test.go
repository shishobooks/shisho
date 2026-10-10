package worker

import (
	"archive/zip"
	"io"
	"path/filepath"
	"sort"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A bulk download includes MOBI and AZW3 files, each generated with the
// book's current metadata.
func TestProcessBulkDownloadJob_IncludesMOBIAndAZW3(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	tc.worker.downloadCache = downloadcache.NewCache(t.TempDir(), 1<<30)
	t.Cleanup(tc.worker.downloadCache.Wait)

	libDir := testgen.TempLibraryDir(t)
	testgen.GenerateMOBI(t, testgen.CreateSubDir(t, libDir, "Mobi Book"), "book.mobi",
		testgen.MOBIOptions{Kind: testgen.MOBIKindCombo, Title: "Mobi In File", HasCover: true})
	testgen.GenerateMOBI(t, testgen.CreateSubDir(t, libDir, "AZW3 Book"), "book.azw3",
		testgen.MOBIOptions{Kind: testgen.MOBIKindKF8, Title: "AZW3 In File", HasCover: true})
	tc.createLibrary([]string{libDir})
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 2)
	fileIDs := make([]int, 0, len(files))
	for _, f := range files {
		fileIDs = append(fileIDs, f.ID)
		_, err := tc.db.NewUpdate().Model((*models.File)(nil)).
			Set("name = ?", "Edited "+f.FileType).Where("id = ?", f.ID).Exec(tc.ctx)
		require.NoError(t, err)
	}

	job := &models.Job{
		Type:       models.JobTypeBulkDownload,
		Status:     models.JobStatusInProgress,
		DataParsed: &models.JobBulkDownloadData{FileIDs: fileIDs},
	}
	require.NoError(t, tc.worker.jobService.CreateJob(tc.ctx, job))
	jobLog := tc.worker.jobLogService.NewJobLogger(tc.ctx, job.ID, tc.worker.log)
	require.NoError(t, tc.worker.ProcessBulkDownloadJob(tc.ctx, job, jobLog))

	data := job.DataParsed.(*models.JobBulkDownloadData)
	assert.Equal(t, 2, data.FileCount)
	archive, err := zip.OpenReader(filepath.Join(tc.worker.downloadCache.BulkZipDir(), data.ZipFilename))
	require.NoError(t, err)
	defer archive.Close()

	var titles []string
	for _, entry := range archive.File {
		r, err := entry.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(r)
		require.NoError(t, err)
		r.Close()
		meta, err := mobi.Parse(testgen.WriteFile(t, t.TempDir(), entry.Name, content))
		require.NoError(t, err)
		titles = append(titles, filepath.Ext(entry.Name)+" "+meta.Title)
	}
	sort.Strings(titles)
	assert.Equal(t, []string{".azw3 Edited azw3", ".mobi Edited mobi"}, titles)
}
