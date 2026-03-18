package worker

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
)

// ComputeBulkFingerprint computes a composite fingerprint hash from sorted file IDs and their individual hashes.
func ComputeBulkFingerprint(fileIDs []int, fileHashes []string) string {
	type entry struct {
		id   int
		hash string
	}
	entries := make([]entry, len(fileIDs))
	for i := range fileIDs {
		entries[i] = entry{id: fileIDs[i], hash: fileHashes[i]}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].id < entries[j].id
	})

	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%d:%s\n", e.id, e.hash)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// DeduplicateFilenames takes a map of fileID -> filename and appends (2), (3), etc. for duplicates.
func DeduplicateFilenames(names map[int]string) map[int]string {
	counts := make(map[string][]int)
	for id, name := range names {
		counts[name] = append(counts[name], id)
	}

	result := make(map[int]string, len(names))
	for name, ids := range counts {
		if len(ids) == 1 {
			result[ids[0]] = name
			continue
		}
		sort.Ints(ids)
		for i, id := range ids {
			if i == 0 {
				result[id] = name
			} else {
				ext := filepath.Ext(name)
				base := strings.TrimSuffix(name, ext)
				result[id] = fmt.Sprintf("%s (%d)%s", base, i+1, ext)
			}
		}
	}
	return result
}

// ProcessBulkDownloadJob generates metadata-injected files and creates a zip archive.
func (w *Worker) ProcessBulkDownloadJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
	log := logger.FromContext(ctx)

	var data models.JobBulkDownloadData
	if err := json.Unmarshal([]byte(job.Data), &data); err != nil {
		return errors.Wrap(err, "failed to parse bulk download job data")
	}

	if len(data.FileIDs) == 0 {
		return errors.New("no file IDs provided")
	}

	jobLog.Info(fmt.Sprintf("starting bulk download for %d files", len(data.FileIDs)), nil)

	type fileWithBook struct {
		file *models.File
		book *models.Book
	}
	filesWithBooks := make([]fileWithBook, 0, len(data.FileIDs))

	for _, fileID := range data.FileIDs {
		file, err := w.bookService.RetrieveFileWithRelations(ctx, fileID)
		if err != nil {
			jobLog.Warn(fmt.Sprintf("skipping file %d: %v", fileID, err), nil)
			continue
		}
		book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
		if err != nil {
			jobLog.Warn(fmt.Sprintf("skipping file %d: failed to load book: %v", fileID, err), nil)
			continue
		}
		filesWithBooks = append(filesWithBooks, fileWithBook{file: file, book: book})
	}

	if len(filesWithBooks) == 0 {
		return errors.New("no valid files found for bulk download")
	}

	// Compute composite fingerprint
	fileIDs := make([]int, len(filesWithBooks))
	fileHashes := make([]string, len(filesWithBooks))
	for i, fw := range filesWithBooks {
		fileIDs[i] = fw.file.ID
		fp, err := downloadcache.ComputeFingerprint(fw.book, fw.file)
		if err != nil {
			return errors.Wrapf(err, "failed to compute fingerprint for file %d", fw.file.ID)
		}
		hash, err := fp.Hash()
		if err != nil {
			return errors.Wrapf(err, "failed to hash fingerprint for file %d", fw.file.ID)
		}
		fileHashes[i] = hash
	}
	compositeHash := ComputeBulkFingerprint(fileIDs, fileHashes)

	// Check if zip already exists in cache
	if w.downloadCache.BulkZipExists(compositeHash) {
		zipPath := w.downloadCache.BulkZipPath(compositeHash)
		info, err := os.Stat(zipPath)
		if err == nil {
			jobLog.Info("bulk zip already cached, skipping generation", nil)
			data.ZipFilename = filepath.Base(zipPath)
			data.SizeBytes = info.Size()
			data.FileCount = len(filesWithBooks)
			data.FingerprintHash = compositeHash
			return w.completeBulkDownloadJob(ctx, job, &data)
		}
	}

	// Generate each file via download cache
	total := len(filesWithBooks)
	cachedPaths := make(map[int]string, total)
	downloadNames := make(map[int]string, total)

	for i, fw := range filesWithBooks {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "job cancelled")
		}

		cachedPath, downloadFilename, err := w.downloadCache.GetOrGenerate(ctx, fw.book, fw.file)
		if err != nil {
			jobLog.Warn(fmt.Sprintf("failed to generate file %d (%s): %v", fw.file.ID, fw.file.Filepath, err), nil)
			continue
		}
		cachedPaths[fw.file.ID] = cachedPath
		downloadNames[fw.file.ID] = downloadFilename

		if w.broker != nil {
			w.broker.Publish(events.NewBulkDownloadProgressEvent(
				job.ID, "generating", i+1, total, data.EstimatedSizeBytes,
			))
		}

		log.Debug("generated file for bulk download", logger.Data{
			"file_id": fw.file.ID, "progress": fmt.Sprintf("%d/%d", i+1, total),
		})
	}

	if len(cachedPaths) == 0 {
		return errors.New("no files were successfully generated")
	}

	// Publish zipping status
	if w.broker != nil {
		w.broker.Publish(events.NewBulkDownloadProgressEvent(
			job.ID, "zipping", total, total, data.EstimatedSizeBytes,
		))
	}

	// Deduplicate filenames
	dedupedNames := DeduplicateFilenames(downloadNames)

	// Create the zip file
	bulkDir := w.downloadCache.BulkZipDir()
	if err := os.MkdirAll(bulkDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create bulk zip directory")
	}

	zipPath := w.downloadCache.BulkZipPath(compositeHash)
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return errors.Wrap(err, "failed to create zip file")
	}

	zipWriter := zip.NewWriter(zipFile)

	// Sort file IDs for deterministic zip ordering
	sortedFileIDs := make([]int, 0, len(cachedPaths))
	for fileID := range cachedPaths {
		sortedFileIDs = append(sortedFileIDs, fileID)
	}
	sort.Ints(sortedFileIDs)

	for _, fileID := range sortedFileIDs {
		cachedPath := cachedPaths[fileID]
		filename := dedupedNames[fileID]

		header := &zip.FileHeader{
			Name:   filename,
			Method: zip.Store,
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			zipWriter.Close()
			zipFile.Close()
			os.Remove(zipPath)
			return errors.Wrapf(err, "failed to create zip entry for %s", filename)
		}

		f, err := os.Open(cachedPath)
		if err != nil {
			zipWriter.Close()
			zipFile.Close()
			os.Remove(zipPath)
			return errors.Wrapf(err, "failed to open cached file %s", cachedPath)
		}

		_, err = io.Copy(writer, f)
		f.Close()
		if err != nil {
			zipWriter.Close()
			zipFile.Close()
			os.Remove(zipPath)
			return errors.Wrapf(err, "failed to write %s to zip", filename)
		}
	}

	if err := zipWriter.Close(); err != nil {
		zipFile.Close()
		os.Remove(zipPath)
		return errors.Wrap(err, "failed to finalize zip")
	}
	zipFile.Close()

	// Get zip file size
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return errors.Wrap(err, "failed to stat zip file")
	}

	jobLog.Info(fmt.Sprintf("bulk download zip created: %d files, %d bytes", len(cachedPaths), zipInfo.Size()), nil)

	data.ZipFilename = filepath.Base(zipPath)
	data.SizeBytes = zipInfo.Size()
	data.FileCount = len(cachedPaths)
	data.FingerprintHash = compositeHash

	return w.completeBulkDownloadJob(ctx, job, &data)
}

func (w *Worker) completeBulkDownloadJob(ctx context.Context, job *models.Job, data *models.JobBulkDownloadData) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "failed to marshal bulk download result")
	}
	job.Data = string(dataBytes)
	job.DataParsed = data

	return w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
		Columns: []string{"data"},
	})
}
