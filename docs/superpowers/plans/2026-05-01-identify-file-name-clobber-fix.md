# Identify Phase 1 — Backend `file.Name` Clobber Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop `persistMetadata` from silently overwriting `file.Name` with `book.Title`, and accept an explicit `file.Name` (with `file.NameSource`) in the apply payload so the upcoming Phase 2 frontend can opt-in per request.

**Architecture:** Two narrowly-scoped backend changes in `pkg/plugins/`. (1) Remove the unconditional title→file.Name mirror in `persistMetadata`. (2) Plumb a new `applyOverrides` struct (`FileName`, `FileNameSource`) from the apply payload through `persistMetadata` so the apply path can explicitly set `file.Name` when, and only when, the payload says so. `mediafile.ParsedMetadata` is **not** modified — `file.Name` is an apply-path-only concern (plugins don't model it), so introducing a new internal struct keeps the SDK contract clean.

**Tech Stack:** Go (Echo, Bun), testify. Backend-only.

---

## Spec reference

This implements Phase 1 of `docs/superpowers/specs/2026-05-01-identify-flow-design.md`:

- `pkg/plugins/handler_persist_metadata.go`: remove the unconditional copy of `title → file.Name`. Only write `file.Name` when the apply payload explicitly includes it.
- `pkg/plugins/handler_apply_metadata.go`: accept an explicit `file.Name` (and `file.NameSource`) in the request payload.
- Backend tests: verify silent overwrites no longer happen on a non-primary file identify; verify explicit `file.Name` in the payload still applies correctly with the right `NameSource`.

## File structure

| File | Change |
|------|--------|
| `pkg/plugins/handler_apply_metadata.go` | Modify. Add `FileName *string` and `FileNameSource *string` to `applyPayload` (json: `file_name`, `file_name_source`). Plumb a new `*applyOverrides` value into `persistMetadata`. |
| `pkg/plugins/handler_persist_metadata.go` | Modify. New parameter `overrides *applyOverrides`. Delete the auto-mirror block ("Mirror the identified title onto the target main file's Name…"). Add a new explicit-write block guarded on `overrides != nil && overrides.FileName != nil`. |
| `pkg/plugins/handler_convert.go` | Modify. Add a new `convertFieldsToOverrides(fields)` returning `*applyOverrides`. (Keeps `convertFieldsToMetadata` unchanged so existing tests are unaffected.) |
| `pkg/plugins/handler_apply_metadata_test.go` | Modify existing tests + add new ones (see Task 6). |
| `pkg/plugins/handler_persist_metadata_test.go` | Modify call sites for the new signature; existing assertions stay correct because no overrides means no `file.Name` write. |

## Behavior summary

**Before this change** — `persistMetadata`, given any non-empty `md.Title`, copies the title onto `targetFile.Name` whenever `targetFile.FileRole == FileRoleMain`. This silently clobbers user-set names like `"Harry Potter (Full-Cast Edition)"` whenever the user identifies any file (primary or not) against a generic plugin result.

**After this change** —
- Old frontend (no `file_name` in payload) → `file.Name` is **never** written by identify. Title clobbering stops.
- New frontend (Phase 2 — `file_name` in payload) → `file.Name` is written to whatever the payload says, with `NameSource` from the payload (or, if the payload omits source, defaulting to the plugin source `plugin:scope/id`).

The `FileRoleMain` restriction goes away on the explicit path: if Phase 2 ever surfaces the Name field for a supplement, the frontend's explicit opt-in is the gate. Phase 1's behavior shift on its own does not enable supplements to be touched (old payloads never set `file_name`), so this is purely future-proofing.

---

### Task 1: Add `applyOverrides` struct and converter

**Files:**
- Modify: `pkg/plugins/handler_convert.go`

- [ ] **Step 1: Add `applyOverrides` struct and `convertFieldsToOverrides` helper to `handler_convert.go`**

Append below `convertFieldsToMetadata` (after the closing `}` on the existing function):

```go
// applyOverrides carries apply-path-only signals that don't belong on
// mediafile.ParsedMetadata (which is part of the public plugin SDK
// contract). These come exclusively from the identify apply payload —
// plugins do not model them.
type applyOverrides struct {
	// FileName is the value to write to file.Name. Nil = no change.
	// Empty string is treated as nil (treat absent or "" as no-op so
	// callers don't need to special-case empty inputs).
	FileName *string
	// FileNameSource is the value to write to file.NameSource. Nil
	// means "default to the plugin source for this apply call".
	FileNameSource *string
}

// convertFieldsToOverrides extracts apply-path-only signals from the
// untyped fields map. Returns nil when no overrides are present, so
// callers can cheaply skip the explicit-write code path.
func convertFieldsToOverrides(fields map[string]any) *applyOverrides {
	var out *applyOverrides
	if v, ok := fields["file_name"].(string); ok && v != "" {
		if out == nil {
			out = &applyOverrides{}
		}
		out.FileName = &v
	}
	if v, ok := fields["file_name_source"].(string); ok && v != "" {
		if out == nil {
			out = &applyOverrides{}
		}
		out.FileNameSource = &v
	}
	return out
}
```

- [ ] **Step 2: Run package tests to confirm nothing else broke**

Run: `go test ./pkg/plugins/... -run TestConvert -count=1`
Expected: PASS (existing convert tests continue to pass; we only added new code).

- [ ] **Step 3: Commit**

```bash
git add pkg/plugins/handler_convert.go
git commit -m "[Backend] Add applyOverrides struct and convertFieldsToOverrides helper"
```

---

### Task 2: Unit-test the new converter helper

**Files:**
- Modify: `pkg/plugins/handler_convert_test.go`

- [ ] **Step 1: Read the existing test file to find a sensible insertion point**

Run: `wc -l pkg/plugins/handler_convert_test.go`
(No assertions; this just confirms the file exists and orients the next step.)

- [ ] **Step 2: Append failing tests**

Append to `pkg/plugins/handler_convert_test.go`:

```go
func TestConvertFieldsToOverrides_NilWhenAbsent(t *testing.T) {
	t.Parallel()

	got := convertFieldsToOverrides(map[string]any{
		"title": "Some Title",
	})
	require.Nil(t, got, "absence of file_name must yield nil overrides so callers can skip the explicit-write path")
}

func TestConvertFieldsToOverrides_NilWhenEmptyString(t *testing.T) {
	t.Parallel()

	got := convertFieldsToOverrides(map[string]any{
		"file_name":        "",
		"file_name_source": "",
	})
	require.Nil(t, got, "empty strings must be treated as absent — no overrides")
}

func TestConvertFieldsToOverrides_FileNameOnly(t *testing.T) {
	t.Parallel()

	got := convertFieldsToOverrides(map[string]any{
		"file_name": "Harry Potter (Full-Cast Edition)",
	})
	require.NotNil(t, got)
	require.NotNil(t, got.FileName)
	assert.Equal(t, "Harry Potter (Full-Cast Edition)", *got.FileName)
	assert.Nil(t, got.FileNameSource, "no source given — caller defaults to plugin source")
}

func TestConvertFieldsToOverrides_BothFields(t *testing.T) {
	t.Parallel()

	got := convertFieldsToOverrides(map[string]any{
		"file_name":        "Custom Name",
		"file_name_source": "manual",
	})
	require.NotNil(t, got)
	require.NotNil(t, got.FileName)
	assert.Equal(t, "Custom Name", *got.FileName)
	require.NotNil(t, got.FileNameSource)
	assert.Equal(t, "manual", *got.FileNameSource)
}
```

If `assert`/`require` are not yet imported in this test file, ensure the imports include:

```go
import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 3: Run the new tests to confirm they pass**

Run: `go test ./pkg/plugins/... -run TestConvertFieldsToOverrides -count=1 -v`
Expected: 4 tests, all PASS. (They pass immediately because Task 1 already implemented the helper — this is a backfill of unit coverage rather than strict TDD.)

- [ ] **Step 4: Commit**

```bash
git add pkg/plugins/handler_convert_test.go
git commit -m "[Test] Cover convertFieldsToOverrides for empty/full payloads"
```

---

### Task 3: Update `persistMetadata` signature (red — failing tests first)

**Files:**
- Modify: `pkg/plugins/handler_persist_metadata_test.go`
- Modify: `pkg/plugins/handler_apply_metadata_test.go`

The plan adds a parameter to `persistMetadata`. Existing tests must compile against the new signature. We do this **first**, before changing the function body, so the next task is a true red→green TDD step.

- [ ] **Step 1: Mechanically update every `persistMetadata` call in both test files to pass `nil` as the new `overrides` argument**

In `pkg/plugins/handler_persist_metadata_test.go` and `pkg/plugins/handler_apply_metadata_test.go` (if any direct `persistMetadata` calls exist there), replace every call of the form

```go
h.persistMetadata(ctx, book, file, md, "test", "plugin-id", testLogger())
```

with

```go
h.persistMetadata(ctx, book, file, md, "test", "plugin-id", nil, testLogger())
```

The new parameter sits between `pluginID` and `log` to match the function signature change in Task 4. There are roughly 14 call sites in `handler_persist_metadata_test.go`. Use a single sed pass or bulk Edit to apply the change uniformly:

Run: `grep -n "h.persistMetadata(" pkg/plugins/handler_persist_metadata_test.go pkg/plugins/handler_apply_metadata_test.go`

Then update each call site so the third-from-last argument list ends with `…, "plugin-id", nil, testLogger())` (or whatever logger expression the call already uses).

- [ ] **Step 2: Confirm the test file does not yet compile (failing build is the red state)**

Run: `go build ./pkg/plugins/...`
Expected: FAIL — the production `persistMetadata` still has the old signature, so call sites in tests now have an extra argument and the package doesn't build. This is the intended red state for Task 4.

- [ ] **Step 3: Commit (compiling tests come in Task 4 alongside the function change)**

Don't commit yet — wait until Task 4's signature change makes the package compile and all tests pass. We'll commit Tasks 3 and 4 together.

---

### Task 4: Implement the new `persistMetadata` signature (green)

**Files:**
- Modify: `pkg/plugins/handler_persist_metadata.go`

- [ ] **Step 1: Update the function signature and remove the auto-mirror block**

Replace the current function header

```go
func (h *handler) persistMetadata(ctx context.Context, book *models.Book, targetFile *models.File, md *mediafile.ParsedMetadata, pluginScope, pluginID string, log logger.Logger) error {
```

with

```go
func (h *handler) persistMetadata(ctx context.Context, book *models.Book, targetFile *models.File, md *mediafile.ParsedMetadata, pluginScope, pluginID string, overrides *applyOverrides, log logger.Logger) error {
```

In the same file, **delete** the inner block that auto-copies title onto file.Name. The block looks exactly like:

```go
		// Mirror the identified title onto the target main file's Name so
		// file organization and downloads reflect it. Supplements keep their
		// own filename-based label.
		if targetFile != nil && targetFile.FileRole == models.FileRoleMain {
			titleCopy := title
			targetFile.Name = &titleCopy
			targetFile.NameSource = &pluginSource
			fileColumns = append(fileColumns, "name", "name_source")
		}
```

Remove those nine lines (including the leading comment) entirely. Leave the `if title != ""` block that updates `book.Title` and friends untouched.

- [ ] **Step 2: Add the explicit-write block for `file.Name`**

Insert the new block immediately after the file-level URL block (after the `// URL (file-level, applied to target file)` block ends with `fileColumns = append(fileColumns, "url", "url_source")`) and before the `// Release date (file-level, applied to target file)` block:

```go
	// Name (file-level, applied to target file). Only written when the
	// caller explicitly opted in via overrides.FileName — replaces the
	// pre-Phase-1 behavior that silently mirrored book.Title onto
	// file.Name on every identify and clobbered user-set edition names.
	if overrides != nil && overrides.FileName != nil && targetFile != nil {
		nameCopy := *overrides.FileName
		targetFile.Name = &nameCopy

		nameSource := pluginSource
		if overrides.FileNameSource != nil {
			nameSource = *overrides.FileNameSource
		}
		nameSourceCopy := nameSource
		targetFile.NameSource = &nameSourceCopy

		fileColumns = append(fileColumns, "name", "name_source")
	}
```

- [ ] **Step 3: Update the lone production caller to pass `nil`**

In `pkg/plugins/handler_apply_metadata.go`, the current call is:

```go
if err := h.persistMetadata(ctx, book, targetFile, md, payload.PluginScope, payload.PluginID, log); err != nil {
```

Change it to (Task 5 will replace `nil` with the real overrides — for now, a `nil` placeholder keeps the build green):

```go
if err := h.persistMetadata(ctx, book, targetFile, md, payload.PluginScope, payload.PluginID, nil, log); err != nil {
```

- [ ] **Step 4: Build the package**

Run: `go build ./pkg/plugins/...`
Expected: PASS — signature changes are now consistent across production and test callers.

- [ ] **Step 5: Run the full plugins test suite**

Run: `go test ./pkg/plugins/... -count=1`

Expected: most tests PASS. **A handful will FAIL** because they assert old auto-mirror behavior:

- `TestApplyMetadata_UpdatesMainFileName_WhenTitleChanges` — expects `*file.Name == "New Title"` after a title-only payload. With Phase 1, the apply path no longer auto-syncs file.Name, so `file.Name` stays `nil`.
- `TestApplyMetadata_FallbackTargetsMainFile_NotSupplement` — expects `main.Name` to be set to "New Title".
- `TestApplyMetadata_PreservesVolumeNotation_CBZ` — expects `*file.Name == "Naruto v1"`.

These failures are **expected red** for the behavior change. Task 6 rewrites them to reflect the new behavior.

For now, leave them failing and continue.

- [ ] **Step 6: Commit Tasks 3 + 4 together**

```bash
git add pkg/plugins/handler_persist_metadata.go \
        pkg/plugins/handler_apply_metadata.go \
        pkg/plugins/handler_persist_metadata_test.go \
        pkg/plugins/handler_apply_metadata_test.go
git commit -m "[Backend] Replace auto-mirror of book.Title onto file.Name with explicit override"
```

(Behavior tests for the new explicit path follow in Task 6. The existing apply-path tests that asserted the old auto-mirror are intentionally left failing in this commit so the next commit's diff cleanly shows their rewrite.)

---

### Task 5: Plumb `file_name` / `file_name_source` through `applyMetadata`

**Files:**
- Modify: `pkg/plugins/handler_apply_metadata.go`

- [ ] **Step 1: Add the two new fields to `applyPayload`**

Replace the `applyPayload` struct with:

```go
type applyPayload struct {
	BookID         int            `json:"book_id" validate:"required"`
	FileID         *int           `json:"file_id"`
	Fields         map[string]any `json:"fields" validate:"required"`
	FileName       *string        `json:"file_name"`
	FileNameSource *string        `json:"file_name_source"`
	PluginScope    string         `json:"plugin_scope" validate:"required"`
	PluginID       string         `json:"plugin_id" validate:"required"`
}
```

`file_name` and `file_name_source` sit at the **top level of the payload**, not inside the `fields` map. They are explicit apply-path signals; `fields` is metadata-shaped (matches `mediafile.ParsedMetadata`). Keeping them separated makes the wire contract crisp: `fields` continues to map 1:1 to ParsedMetadata, while `file_name`/`file_name_source` are recognized only by the apply handler.

- [ ] **Step 2: Build the overrides value from the typed payload fields**

In `applyMetadata`, immediately after `md := convertFieldsToMetadata(payload.Fields)` (currently around line 66), add:

```go
	var overrides *applyOverrides
	if (payload.FileName != nil && *payload.FileName != "") || (payload.FileNameSource != nil && *payload.FileNameSource != "") {
		overrides = &applyOverrides{
			FileName:       payload.FileName,
			FileNameSource: payload.FileNameSource,
		}
	}
```

The empty-string guard keeps callers that send `"file_name": ""` from accidentally writing an empty `file.Name`. This mirrors `convertFieldsToOverrides`'s treatment of empty strings as absent.

(Note: `convertFieldsToOverrides` from Task 1 stays in the codebase as the public extraction helper; we don't use it here because the new fields are top-level, not nested in `Fields`. The helper is still useful for tests and stays a stable shape for any future caller that processes a flattened map.)

- [ ] **Step 3: Pass `overrides` to `persistMetadata`**

Update the call site you previously edited in Task 4 step 3:

```go
if err := h.persistMetadata(ctx, book, targetFile, md, payload.PluginScope, payload.PluginID, overrides, log); err != nil {
```

(replacing the placeholder `nil` with `overrides`).

- [ ] **Step 4: Build the package**

Run: `go build ./pkg/plugins/...`
Expected: PASS.

- [ ] **Step 5: Run plugin tests (still expecting the three failures from Task 4)**

Run: `go test ./pkg/plugins/... -count=1`
Expected: same three pre-existing failures from Task 4 (unchanged) plus all other tests PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/plugins/handler_apply_metadata.go
git commit -m "[Backend] Accept explicit file_name/file_name_source in identify apply payload"
```

---

### Task 6: Rewrite stale tests and add new behavior tests

**Files:**
- Modify: `pkg/plugins/handler_apply_metadata_test.go`

- [ ] **Step 1: Replace the three stale tests with new ones reflecting the post-fix behavior**

The three failing tests from Task 4 step 5 assert the old auto-mirror. Rewrite them as follows.

#### 6a. `TestApplyMetadata_UpdatesMainFileName_WhenTitleChanges` → `TestApplyMetadata_DoesNotAutoSyncMainFileName_WhenOnlyTitleSent`

Replace the entire test body with:

```go
// TestApplyMetadata_DoesNotAutoSyncMainFileName_WhenOnlyTitleSent verifies
// the Phase 1 fix: a title-only payload no longer silently mirrors book.Title
// onto the main file's Name. Old frontends that don't ship file_name continue
// to function (no errors), but file.Name is left untouched. Edition-specific
// names like "Harry Potter (Full-Cast Edition)" are no longer clobbered on
// re-identify. To opt in, callers must send file_name explicitly.
func TestApplyMetadata_DoesNotAutoSyncMainFileName_WhenOnlyTitleSent(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeEPUB)
	originalName := "Custom Edition Name"
	file.Name = &originalName

	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContext(t, map[string]any{"title": "New Title"})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	assert.Equal(t, "New Title", book.Title, "book.Title should still update")
	require.NotNil(t, file.Name)
	assert.Equal(t, "Custom Edition Name", *file.Name, "file.Name must NOT be silently overwritten by book.Title")
	assert.Nil(t, file.NameSource, "file.NameSource must NOT be set when no explicit file_name was sent")
}
```

#### 6b. `TestApplyMetadata_FallbackTargetsMainFile_NotSupplement`

Tighten the assertions: the targeting still picks the main file (still important — supplements should never be the apply target unless explicitly chosen), but `main.Name` is no longer expected to be auto-set. Replace the trailing assertions block (the four `require`/`assert` lines after `require.NoError(t, err)`) with:

```go
	// The main file is still the targeted file (supplement is skipped),
	// but with Phase 1 file.Name is no longer auto-synced from book.Title.
	// Both files' Name/NameSource stay nil on a title-only payload.
	assert.Nil(t, main.Name, "main file Name must NOT be auto-set by a title-only payload")
	assert.Nil(t, main.NameSource, "main file NameSource must NOT be auto-set by a title-only payload")
	assert.Nil(t, supplement.Name, "supplement Name must remain untouched")

	// Verify the main file *was* targeted (not the supplement) by
	// checking that book.Title was written through to the right scope.
	assert.Equal(t, "New Title", book.Title, "book.Title should be set, confirming the apply ran")
```

#### 6c. `TestApplyMetadata_PreservesVolumeNotation_CBZ`

The original assertion was that book.Title preserves the verbatim "Naruto v1" string and that `file.Name` mirrors it. The mirror is gone; the title-preservation assertion stays. Replace the trailing assertions block (the three lines after `require.NoError(t, err)`) with:

```go
	assert.Equal(t, "Naruto v1", book.Title, "book.Title must not be volume-normalized on identify")
	assert.Nil(t, file.Name, "file.Name must NOT be auto-set by a title-only payload (Phase 1 fix)")
```

- [ ] **Step 2: Add new tests covering the explicit-write path**

Append to `pkg/plugins/handler_apply_metadata_test.go`:

```go
// newApplyEchoContextWithFileName builds an Echo context where the apply
// payload carries an explicit top-level file_name and (optionally) a
// file_name_source — the new Phase 1 wire signal.
func newApplyEchoContextWithFileName(t *testing.T, fields map[string]any, fileName string, fileNameSource string) echo.Context {
	t.Helper()
	payload := applyPayload{
		BookID:      1,
		Fields:      fields,
		PluginScope: "test",
		PluginID:    "enricher",
	}
	if fileName != "" {
		fn := fileName
		payload.FileName = &fn
	}
	if fileNameSource != "" {
		fns := fileNameSource
		payload.FileNameSource = &fns
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &models.User{
		ID:            1,
		LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}},
	})
	return c
}

// TestApplyMetadata_ExplicitFileName_AppliedWithPluginSourceByDefault verifies
// that a payload carrying file_name (without file_name_source) writes
// file.Name and stamps NameSource with the plugin source — the default for
// a value the user accepted as-is from the plugin's proposal.
func TestApplyMetadata_ExplicitFileName_AppliedWithPluginSourceByDefault(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeEPUB)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContextWithFileName(t,
		map[string]any{"title": "New Title"},
		"New Title",
		"")

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name, "file.Name must be set when file_name is explicit")
	assert.Equal(t, "New Title", *file.Name)
	require.NotNil(t, file.NameSource, "file.NameSource must be set when file_name is explicit")
	assert.Equal(t, "plugin:test/enricher", *file.NameSource,
		"absent file_name_source defaults to the plugin source for this apply call")
}

// TestApplyMetadata_ExplicitFileName_HonorsExplicitSource verifies that
// when the payload carries file_name_source, that exact value is written
// to file.NameSource. This lets the Phase 2 frontend distinguish "user
// accepted the plugin's proposed Name" from "user edited Name manually".
func TestApplyMetadata_ExplicitFileName_HonorsExplicitSource(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Old Title", models.FileTypeEPUB)
	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	c := newApplyEchoContextWithFileName(t,
		map[string]any{"title": "New Title"},
		"My Custom Edition Name",
		models.DataSourceManual)

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name)
	assert.Equal(t, "My Custom Edition Name", *file.Name)
	require.NotNil(t, file.NameSource)
	assert.Equal(t, models.DataSourceManual, *file.NameSource,
		"explicit file_name_source must be written verbatim")
}

// TestApplyMetadata_ExplicitFileName_PreservesEditionName_NonPrimaryFile is
// the regression test for the original spec bug: identifying a non-primary
// file against a generic plugin result no longer corrupts an edition-specific
// file.Name. Before Phase 1, this scenario silently mirrored book.Title onto
// file.Name. After Phase 1, file.Name only updates when the payload says so.
// Here, the payload omits file_name entirely (modeling an old frontend OR a
// new frontend where the user unchecked the Name row).
func TestApplyMetadata_ExplicitFileName_PreservesEditionName_NonPrimaryFile(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Harry Potter and the Sorcerer's Stone", models.FileTypeEPUB)
	originalName := "Harry Potter and the Sorcerer's Stone (Full-Cast Edition)"
	file.Name = &originalName
	originalSource := models.DataSourceManual
	file.NameSource = &originalSource

	store := &stubBookStoreForApply{
		stubBookStoreForPersist: stubBookStoreForPersist{book: book},
	}
	h := newApplyTestHandler(store)
	// Title-only payload — no file_name — same as a non-primary identify
	// against a generic plugin result. The bug being fixed: previously this
	// would clobber the edition name with the bare book title.
	c := newApplyEchoContext(t, map[string]any{
		"title": "Harry Potter and the Sorcerer's Stone",
	})

	err := h.applyMetadata(c)
	require.NoError(t, err)

	require.NotNil(t, file.Name)
	assert.Equal(t, "Harry Potter and the Sorcerer's Stone (Full-Cast Edition)", *file.Name,
		"edition-specific file.Name must NOT be clobbered by a title-only identify (Phase 1 spec bug fix)")
	require.NotNil(t, file.NameSource)
	assert.Equal(t, models.DataSourceManual, *file.NameSource,
		"file.NameSource must NOT be replaced by the plugin source")
}
```

- [ ] **Step 3: Run the apply tests**

Run: `go test ./pkg/plugins/... -run TestApplyMetadata -count=1 -v`
Expected: all PASS (the three rewritten tests now match the new behavior; the three new tests cover the explicit path and the regression).

- [ ] **Step 4: Run the full plugins test suite**

Run: `go test ./pkg/plugins/... -count=1`
Expected: PASS, no failures, no skips beyond pre-existing ones.

- [ ] **Step 5: Commit**

```bash
git add pkg/plugins/handler_apply_metadata_test.go
git commit -m "[Test] Rewrite identify-apply tests for explicit file.Name path"
```

---

### Task 7: Final verification — full Go test suite + lint

The previous tasks ran the targeted test slice. Before declaring Phase 1 done, run the broader checks called out in the project CLAUDE.md ("Go-only edits → `mise lint test`").

- [ ] **Step 1: Run full Go tests**

Run: `mise test`
Expected: PASS. Watch specifically for `pkg/plugins/...` and `pkg/worker/...` — the worker uses persistMetadata indirectly via the scan path through the manager (it shouldn't, since worker calls plugins via `RunMetadataSearch`, not `persistMetadata`, but verify).

If any failure surfaces in another package, investigate before patching. The most likely failure surface is a hand-rolled fake or compile-time call that I missed during the signature change. Grep is the answer:

```bash
grep -rn "persistMetadata(" --include="*.go"
```

If it shows hits outside `pkg/plugins/`, update those call sites (this should only matter if `persistMetadata` was leaked outside the package, which currently it is not — it's an unexported method).

- [ ] **Step 2: Run Go lint**

Run: `mise lint`
Expected: clean.

- [ ] **Step 3: Verify spec phase tracking**

Open `docs/superpowers/specs/2026-05-01-identify-flow-design.md` and find the Phase 1 section under `## Phases`. Update its heading from

```markdown
### Phase 1 · Backend `file.Name` clobber fix
```

to

```markdown
### Phase 1 · Backend `file.Name` clobber fix ✅ shipped
```

This signals to the next agent reading the spec that Phase 2 can now begin.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-05-01-identify-flow-design.md
git commit -m "[Docs] Mark identify Phase 1 as shipped"
```

- [ ] **Step 5: Push and open PR**

Phase 1 ships independently — don't wait on Phase 2. Hand off to the `ship-it` skill.

---

## Out of scope for this plan

Per the spec, these explicitly belong to later phases. Don't touch them in Phase 1 even if temptation strikes:

- Frontend `IdentifyReviewForm.tsx` — Phase 2.
- New shared frontend primitives (MixedCheckbox, ComboboxTypeahead, CompositeRow, StickySection, DialogFrame) — Phase 2.
- Per-field decision flags / per-field opt-in semantics on the apply payload — Phase 2 (Phase 1 only adds `file_name`/`file_name_source`).
- Edit-dialog ports (BookEditDialog, FileEditDialog) — Phase 3.
- Plain-text date input migration — Phase 4.
- Sidecar/scanner integration changes — none required for this fix; sidecar writes already pick up `file.Name` and `file.NameSource` whenever they're set.
- Plugin SDK changes — none required; ParsedMetadata is unchanged.

## Risk assessment

**Low risk.** This change subtracts behavior on the implicit path and adds a new explicit path that defaults to no-op. Old frontends become more correct (no more silent clobbering); the new frontend gets a clean, predictable knob. The signature change to `persistMetadata` is a private package method with one production caller — no API or SDK shape changes.

Watch for: any hidden caller in the worker pipeline that I missed. The Step 1 grep in Task 7 covers this.
