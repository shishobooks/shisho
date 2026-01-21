# Use Saved Chapters in File Generation Implementation Plan

**Goal:** Integrate saved chapter information from the database into file generation so that user-edited chapters appear in downloaded files.

**Design Spec:** Investigation results from Explore agent + user request

**Architecture:** Add Chapters relation to File model, include chapters in fingerprint for cache invalidation, load chapters during download, and use DB chapters in M4B generator. CBZ/EPUB support deferred to future work.

**Tech Stack:** Go (Bun ORM, Echo), existing fingerprint/generator infrastructure

**Design Decision - Nested Chapters:** M4B format does not support nested/hierarchical chapters. When DB chapters have Children, only top-level chapters will be written to M4B. The Children are preserved in fingerprint for cache invalidation but ignored in M4B generation.

---

## Feature 1: Add Chapters Relation to File Model

- [x] Task 1.1 Add Chapters relation field to File model
  - File: `pkg/models/file.go`
  - Add after line 46 (after Identifiers field):
    ```go
    Chapters []*Chapter `bun:"rel:has-many,join:id=file_id" json:"chapters,omitempty" tstype:"Chapter[]"`
    ```
  - The Chapter model already has the inverse relation (`File *File bun:"rel:belongs-to,join:file_id=id"`)

- [x] Task 1.2 Run `make tygo` to regenerate TypeScript types
  - Verify generated types include the new Chapters field

## Feature 2: Add Chapters to Fingerprint

**Dependencies:** Feature 1

- [x] Task 2.1a Write failing test: "chapters are included in fingerprint"
  - File: `pkg/downloadcache/fingerprint_test.go`
  - Create file with Chapters containing model chapters
  - Verify FingerprintChapter fields populated (Title, SortOrder, StartTimestampMs)

- [x] Task 2.1b Write failing test: "different chapters produce different hash"
  - Same file, create two files with different chapter titles
  - Verify hash differs

- [x] Task 2.1c Write failing test: "chapter order affects hash"
  - Create two files with same chapters in different order
  - Verify hash changes when chapter order differs

- [x] Task 2.1d Write failing test: "chapters sorted by sort_order for consistent fingerprinting"
  - Create file with chapters SortOrder 2, 0, 1
  - Verify fingerprint has them in 0, 1, 2 order

- [x] Task 2.1e Write failing test: "nested chapters are included"
  - Create chapters with Children populated (parent has child chapters)
  - Verify FingerprintChapter.Children is populated recursively with correct Title/SortOrder

- [x] Task 2.1f Write failing test: "file with no chapters has empty chapters slice"
  - Create file with nil/empty Chapters
  - Verify fp.Chapters is empty slice, not nil

- [x] Task 2.1g Write failing test: "chapters with nil optional fields fingerprint correctly"
  - Create chapter with nil StartPage, nil Href, nil StartTimestampMs
  - Verify fingerprint computes without error

- [x] Task 2.2 Run tests to verify they fail
  - Expected: compilation error (FingerprintChapter doesn't exist) or assertion failures

- [x] Task 2.3 Add FingerprintChapter struct to fingerprint.go
  - File: `pkg/downloadcache/fingerprint.go`
  - Add struct after FingerprintIdentifier (around line 67):
    ```go
    // FingerprintChapter represents chapter information for fingerprinting.
    type FingerprintChapter struct {
        Title            string               `json:"title"`
        SortOrder        int                  `json:"sort_order"`
        StartPage        *int                 `json:"start_page,omitempty"`
        StartTimestampMs *int64               `json:"start_timestamp_ms,omitempty"`
        Href             *string              `json:"href,omitempty"`
        Children         []FingerprintChapter `json:"children,omitempty"`
    }
    ```
  - Add Chapters field to Fingerprint struct (after CoverPage): `Chapters []FingerprintChapter json:"chapters,omitempty"`

- [x] Task 2.4 Add convertChaptersToFingerprint helper function
  - File: `pkg/downloadcache/fingerprint.go`
  - Add helper function:
    ```go
    // convertChaptersToFingerprint recursively converts model chapters to fingerprint format.
    func convertChaptersToFingerprint(chapters []*models.Chapter) []FingerprintChapter {
        if len(chapters) == 0 {
            return []FingerprintChapter{}
        }
        result := make([]FingerprintChapter, len(chapters))
        for i, ch := range chapters {
            result[i] = FingerprintChapter{
                Title:            ch.Title,
                SortOrder:        ch.SortOrder,
                StartPage:        ch.StartPage,
                StartTimestampMs: ch.StartTimestampMs,
                Href:             ch.Href,
                Children:         convertChaptersToFingerprint(ch.Children),
            }
        }
        return result
    }
    ```

- [x] Task 2.5 Update ComputeFingerprint to call helper
  - File: `pkg/downloadcache/fingerprint.go`
  - Note: `sort` package is already imported in this file
  - In `ComputeFingerprint()`, after cover handling (around line 218), add:
    ```go
    // Add chapters (sorted by SortOrder for consistent fingerprinting)
    if file != nil && len(file.Chapters) > 0 {
        chapters := make([]*models.Chapter, len(file.Chapters))
        copy(chapters, file.Chapters)
        sort.Slice(chapters, func(i, j int) bool {
            return chapters[i].SortOrder < chapters[j].SortOrder
        })
        fp.Chapters = convertChaptersToFingerprint(chapters)
    } else {
        fp.Chapters = []FingerprintChapter{}
    }
    ```

- [x] Task 2.6 Run tests to verify they pass

- [x] Task 2.7 Run `make check` to verify no regressions

## Feature 3: Load Chapters in Book Retrieval

**Dependencies:** Feature 1

- [x] Task 3.1a Write failing test: "RetrieveBook loads chapters for each file"
  - File: `pkg/books/service_test.go` (create if doesn't exist, or add to existing)
  - Use test setup pattern from `internal/testgen` package and test DB setup from existing tests
  - Create a book with a file using existing test helpers
  - Create chapters for that file via chapter service (no parent)
  - Call RetrieveBook
  - Assert file.Chapters is populated with correct data
  - Assert chapters are ordered by sort_order

- [x] Task 3.1b Write failing test: "RetrieveBook loads nested chapters via Children"
  - Create chapters with parent_id set (parent -> child relationship)
  - Call RetrieveBook
  - Assert parent chapter's Children field is populated
  - Note: Bun auto-loads Children via `rel:has-many,join:id=parent_id` on Chapter model

- [x] Task 3.2 Run tests to verify they fail
  - Expected: file.Chapters is nil/empty

- [x] Task 3.3 Add Chapters relation loading to RetrieveBook
  - File: `pkg/books/service.go`
  - In `RetrieveBook()`, add after `.Relation("Files.Identifiers")` (around line 161):
    ```go
    Relation("Files.Chapters", func(sq *bun.SelectQuery) *bun.SelectQuery {
        return sq.Order("ch.sort_order ASC")
    }).
    ```
  - Note: Bun supports nested relation callbacks - same pattern as `Files.Narrators`
  - Children are auto-loaded via Chapter model's `rel:has-many,join:id=parent_id` on Children field

- [x] Task 3.4a Write failing test: "RetrieveBookByFilePath loads chapters for each file"
  - File: `pkg/books/service_test.go`
  - Same setup as Task 3.1a but use RetrieveBookByFilePath

- [x] Task 3.4b Write failing test: "RetrieveBookByFilePath loads nested chapters"
  - Same setup as Task 3.1b but use RetrieveBookByFilePath

- [x] Task 3.5 Add Chapters relation loading to RetrieveBookByFilePath
  - File: `pkg/books/service.go`
  - Same change as RetrieveBook (add after `.Relation("Files.Identifiers")`, around line 213)

- [x] Task 3.6 Run tests to verify they pass

- [x] Task 3.7 Run `make check` to verify no regressions

## Feature 4: Update M4B Generator to Use Database Chapters

**Dependencies:** Feature 1, Feature 3

- [x] Task 4.1a Write failing test: "uses chapters from file model instead of source"
  - File: `pkg/filegen/m4b_test.go`
  - Create source M4B with chapters via testgen (e.g., chapters "Source Ch 1", "Source Ch 2")
  - Create book and file with different chapters in file.Chapters model (e.g., "DB Chapter 1", "DB Chapter 2", "DB Chapter 3")
  - Generate M4B
  - Parse result and verify chapters match file.Chapters titles, not source chapters

- [x] Task 4.1b Write failing test: "preserves source chapters when file has no chapters"
  - Create source M4B with chapters
  - Create book and file with no chapters (file.Chapters = nil)
  - Generate M4B
  - Parse result and verify source chapters are preserved

- [x] Task 4.1c Write failing test: "chapter order in generated M4B matches SortOrder"
  - Create file.Chapters with SortOrder 2, 0, 1
  - Generate M4B
  - Parse and verify chapters appear in 0, 1, 2 order

- [x] Task 4.1d Write failing test: "chapters with missing StartTimestampMs use zero"
  - Create chapter with nil StartTimestampMs
  - Generate M4B
  - Verify chapter has Start: 0

- [x] Task 4.1e Write failing test: "nested chapters use only top-level for M4B"
  - Create chapters with Children (M4B doesn't support nesting)
  - Generate M4B
  - Verify only top-level chapters appear (Children ignored per design decision)

- [x] Task 4.1f Write failing test: "chapters with empty title are included"
  - Create chapter with Title = ""
  - Generate M4B
  - Verify chapter appears with empty title

- [x] Task 4.2 Run tests to verify they fail
  - Expected: generated file has source chapters, not model chapters

- [x] Task 4.3 Add helper function to convert models.Chapter to mp4.Chapter
  - File: `pkg/filegen/m4b.go`
  - Note: `sort` package is already imported in this file
  - Add function (uses only top-level chapters per design decision):
    ```go
    // convertModelChaptersToMP4 converts database chapters to MP4 format.
    // M4B doesn't support nested chapters, so only top-level chapters are used.
    // duration is the total file duration, used to calculate the End time of the last chapter.
    func convertModelChaptersToMP4(chapters []*models.Chapter, duration time.Duration) []mp4.Chapter {
        if len(chapters) == 0 {
            return nil
        }

        // Sort by SortOrder
        sorted := make([]*models.Chapter, len(chapters))
        copy(sorted, chapters)
        sort.Slice(sorted, func(i, j int) bool {
            return sorted[i].SortOrder < sorted[j].SortOrder
        })

        result := make([]mp4.Chapter, len(sorted))
        for i, ch := range sorted {
            // Handle nil StartTimestampMs - default to 0
            start := time.Duration(0)
            if ch.StartTimestampMs != nil {
                start = time.Duration(*ch.StartTimestampMs) * time.Millisecond
            }

            // Calculate End: next chapter start or total duration
            var end time.Duration
            if i+1 < len(sorted) {
                if sorted[i+1].StartTimestampMs != nil {
                    end = time.Duration(*sorted[i+1].StartTimestampMs) * time.Millisecond
                } else {
                    end = duration
                }
            } else {
                end = duration
            }

            result[i] = mp4.Chapter{
                Title: ch.Title,
                Start: start,
                End:   end,
            }
        }
        return result
    }
    ```

- [x] Task 4.4 Update buildMetadata to use file chapters when available
  - File: `pkg/filegen/m4b.go`
  - In `buildMetadata()`, after line 78 (`Chapters: src.Chapters`), add logic to override:
    ```go
    // Use database chapters if available, otherwise preserve source chapters
    if file != nil && len(file.Chapters) > 0 {
        meta.Chapters = convertModelChaptersToMP4(file.Chapters, src.Duration)
    }
    ```
  - This maintains backward compatibility - only override when DB chapters exist

- [x] Task 4.5 Run tests to verify they pass

- [x] Task 4.6 Run `make check` to verify no regressions

## Feature 5: Integration Test for End-to-End Chapter Flow

**Dependencies:** Features 1-4

- [x] Task 5.1a Write integration test: "chapter changes invalidate cache"
  - File: `pkg/downloadcache/cache_test.go` (add to existing)
  - Create source M4B with testgen
  - Create book/file with chapters via test setup (e.g., chapter "Original Title")
  - Generate via cache (first generation)
  - Update chapter title directly in file.Chapters model (change to "Updated Title")
  - Compute new fingerprint hash
  - Verify hash differs from original (cache invalidated)

- [x] Task 5.1b Write integration test: "same chapters hit cache"
  - Create source M4B
  - Create book/file with chapters
  - Generate via cache (first generation), note the path
  - Generate again without changing chapters
  - Verify same cached path returned

- [x] Task 5.2 Run integration tests to verify they pass

## Feature 6: Final Verification

- [x] Task 6.1 Run `make check` to verify all tests pass
  - This runs tests, Go lint, and JS lint

- [x] Task 6.2 Run `make lint` to verify code style
  - Fix any linting issues

- [x] Task 6.3 Manual verification with real M4B file (optional)
  - Start server with `make start`
  - Edit chapters for an M4B file in UI
  - Download the file
  - Verify chapters in downloaded file match edited chapters
