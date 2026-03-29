# Search Context Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Simplify the plugin search context to `{ query, author, identifiers }` and add author/identifier controls to the identify dialog.

**Architecture:** Replace the nested `{ query, book: {...}, file: {...} }` search context with a flat `{ query, author, identifiers }` shape, used identically by both the interactive identify flow and the automatic scan pipeline. The identify dialog gains an author text input and dismissable identifier chips.

**Tech Stack:** Go (Echo, goja), React (TypeScript), TailwindCSS

**Spec:** `docs/superpowers/specs/2026-03-28-search-context-simplification-design.md`

---

### Task 1: Simplify SearchContext in Go Backend

**Files:**
- Modify: `pkg/plugins/handler.go:135-138` (update searchPayload)
- Modify: `pkg/plugins/handler.go:1313-1327` (rewrite context building in searchMetadata)
- Modify: `pkg/plugins/handler.go:1723-1783` (delete buildSearchBookContext)
- Modify: `pkg/worker/scan_unified.go:2606-2638` (rewrite context building in runMetadataEnrichers)
- Modify: `pkg/worker/scan_unified.go:2944-3055` (delete buildFileContext and buildBookContext)
- Modify: `pkg/plugins/hooks_test.go:586-588` (update search context in test)
- Modify: `pkg/plugins/testdata/hooks-enricher/main.js` (update to use new context shape)

- [ ] **Step 1: Update searchPayload to accept author and identifiers**

In `pkg/plugins/handler.go`, replace the `searchPayload` struct (lines 135-138):

```go
type searchPayload struct {
	Query       string                       `json:"query" validate:"required"`
	BookID      int                          `json:"book_id" validate:"required"`
	Author      string                       `json:"author"`
	Identifiers []mediafile.ParsedIdentifier `json:"identifiers"`
}
```

- [ ] **Step 2: Rewrite searchMetadata context building**

In `pkg/plugins/handler.go`, replace the context building section (lines 1313-1327) with:

```go
	// Build flat search context from payload
	searchCtx := map[string]interface{}{
		"query": payload.Query,
	}
	if payload.Author != "" {
		searchCtx["author"] = payload.Author
	}
	if len(payload.Identifiers) > 0 {
		ids := make([]map[string]interface{}, len(payload.Identifiers))
		for i, id := range payload.Identifiers {
			ids[i] = map[string]interface{}{
				"type":  id.Type,
				"value": id.Value,
			}
		}
		searchCtx["identifiers"] = ids
	}

	var allResults []EnrichSearchResult
	for _, rt := range runtimes {
```

Remove the per-plugin `searchCtx` creation inside the loop — the context is now built once above the loop and reused. The `resp, sErr := h.manager.RunMetadataSearch(ctx, rt, searchCtx)` call stays the same.

- [ ] **Step 3: Delete buildSearchBookContext**

Delete the entire `buildSearchBookContext` function (lines 1723-1783 in `pkg/plugins/handler.go`). It's no longer called anywhere.

- [ ] **Step 4: Simplify book relation loading in searchMetadata**

The handler still needs to load the book for the library access check and disabled fields computation, but no longer needs Authors, BookSeries, or File Identifiers. Simplify the query (lines 1285-1293) to only load what's needed:

```go
		var b models.Book
		err = h.db.NewSelect().Model(&b).
			Where("b.id = ?", payload.BookID).
			Scan(ctx)
		if err == nil {
			book = &b
		}
```

The `h.enrich.bookStore.RetrieveBook` path (line 1282) can stay as-is since it's a convenience method that loads relations anyway — no harm in having extra data loaded.

- [ ] **Step 5: Rewrite scan pipeline context building**

In `pkg/worker/scan_unified.go`, replace the context building in `runMetadataEnrichers` (lines 2612-2638):

```go
	// Determine author: first author name if available
	var author string
	if len(book.Authors) > 0 {
		for _, a := range book.Authors {
			if a.Person != nil {
				author = a.Person.Name
				break
			}
		}
	}

	// Collect identifiers from the file being enriched
	var identifiers []map[string]interface{}
	if file != nil && len(file.Identifiers) > 0 {
		identifiers = make([]map[string]interface{}, len(file.Identifiers))
		for i, id := range file.Identifiers {
			identifiers[i] = map[string]interface{}{
				"type":  id.Type,
				"value": id.Value,
			}
		}
	}

	var enrichedMeta mediafile.ParsedMetadata
	modified := false

	for _, rt := range runtimes {
		// Check if enricher handles this file type
		enricherCap := rt.Manifest().Capabilities.MetadataEnricher
		if enricherCap == nil {
			continue
		}
		handles := false
		for _, ft := range enricherCap.FileTypes {
			if ft == file.FileType {
				handles = true
				break
			}
		}
		if !handles {
			continue
		}

		// Build flat search context (same shape as interactive identify)
		searchCtx := map[string]interface{}{
			"query": query,
		}
		if author != "" {
			searchCtx["author"] = author
		}
		if len(identifiers) > 0 {
			searchCtx["identifiers"] = identifiers
		}
```

- [ ] **Step 6: Delete buildFileContext and buildBookContext**

Delete both functions from `pkg/worker/scan_unified.go`:
- `buildFileContext` (lines 2944-2963)
- `buildBookContext` (lines 2965-3055+)

These are only called from `runMetadataEnrichers` which no longer uses them.

- [ ] **Step 7: Update test search context**

In `pkg/plugins/hooks_test.go`, update the search context in `TestSearchMetadataCarriesAllFields` (lines 586-588):

```go
	searchCtx := map[string]interface{}{
		"query":  "Test",
		"author": "Test Author",
	}
```

- [ ] **Step 8: Update test plugin to use new context**

In `pkg/plugins/testdata/hooks-enricher/main.js`, the plugin already uses `context.query` (line 5). No change needed — it doesn't access `context.book` or `context.file`.

- [ ] **Step 9: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -v -count=1 2>&1 | tail -20`
Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/worker/ -v -count=1 2>&1 | tail -20`

Expected: All tests pass.

- [ ] **Step 10: Commit**

```bash
git add pkg/plugins/handler.go pkg/worker/scan_unified.go pkg/plugins/hooks_test.go
git commit -m "[Backend] Simplify search context to { query, author, identifiers }

Replace nested { query, book, file } search context with flat
{ query, author, identifiers }. Same shape for both interactive
identify and automatic scan enrichment. Delete buildSearchBookContext,
buildBookContext, and buildFileContext."
```

---

### Task 2: Update TypeScript SDK SearchContext

**Files:**
- Modify: `packages/plugin-sdk/hooks.d.ts:21-34` (rewrite SearchContext)

- [ ] **Step 1: Rewrite SearchContext interface**

In `packages/plugin-sdk/hooks.d.ts`, replace the `SearchContext` interface (lines 21-34):

```typescript
/** Context passed to metadataEnricher.search(). */
export interface SearchContext {
  /** Search query — title or free-text. Always present. */
  query: string;
  /** Author name to narrow results. Optional. */
  author?: string;
  /** Structured identifiers for direct lookup (ISBN, ASIN, etc.). Optional. */
  identifiers?: Array<{ type: string; value: string }>;
}
```

- [ ] **Step 2: Commit**

```bash
git add packages/plugin-sdk/hooks.d.ts
git commit -m "[Feature] Simplify SearchContext to { query, author, identifiers }"
```

---

### Task 3: Add Author and Identifier Controls to Identify Dialog

**Files:**
- Modify: `app/components/library/IdentifyBookDialog.tsx` (add state, inputs, update mutation call)
- Modify: `app/hooks/queries/plugins.ts:523-532` (update usePluginSearch payload type)

- [ ] **Step 1: Update usePluginSearch mutation payload type**

In `app/hooks/queries/plugins.ts`, update the mutation type (lines 523-532):

```typescript
export const usePluginSearch = () => {
  return useMutation<
    PluginSearchResponse,
    ShishoAPIError,
    {
      query: string;
      bookId: number;
      author?: string;
      identifiers?: Array<{ type: string; value: string }>;
    }
  >({
    mutationFn: ({ query, bookId, author, identifiers }) => {
      return API.request<PluginSearchResponse>("POST", "/plugins/search", {
        query,
        book_id: bookId,
        author: author || undefined,
        identifiers: identifiers?.length ? identifiers : undefined,
      });
    },
  });
};
```

- [ ] **Step 2: Add author and identifiers state to IdentifyBookDialog**

In `app/components/library/IdentifyBookDialog.tsx`, add state variables after the existing `query` state (after line 45):

```typescript
  const [author, setAuthor] = useState("");
  const [identifiers, setIdentifiers] = useState<
    Array<{ type: string; value: string }>
  >([]);
```

Update the dialog open effect (lines 62-69) to pre-fill these:

```typescript
  useEffect(() => {
    if (open) {
      setStep("search");
      setQuery(book.title);
      // Pre-fill author from first author
      const firstAuthor = book.authors?.[0]?.person?.name ?? "";
      setAuthor(firstAuthor);
      // Pre-fill identifiers from all files
      const allIds: Array<{ type: string; value: string }> = [];
      const seen = new Set<string>();
      for (const file of book.files ?? []) {
        for (const id of file.identifiers ?? []) {
          const key = `${id.type}:${id.value}`;
          if (!seen.has(key)) {
            seen.add(key);
            allIds.push({ type: id.type, value: id.value });
          }
        }
      }
      setIdentifiers(allIds);
      setSelectedResult(null);
      setSelectedFileId(mainFiles.length > 1 ? mainFiles[0].id : undefined);
      hasSearchedRef.current = false;
    }
  }, [open, book.title, book.authors, book.files, mainFiles]);
```

- [ ] **Step 3: Update search mutation calls to pass author and identifiers**

Update the auto-search effect (lines 72-76):

```typescript
  useEffect(() => {
    if (open && query && !hasSearchedRef.current) {
      hasSearchedRef.current = true;
      searchMutation.mutate({
        query,
        bookId: book.id,
        author: author || undefined,
        identifiers: identifiers.length > 0 ? identifiers : undefined,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, query]);
```

Update the `handleSearch` function (lines 78-81):

```typescript
  const handleSearch = () => {
    if (!query.trim()) return;
    setSelectedResult(null);
    searchMutation.mutate({
      query: query.trim(),
      bookId: book.id,
      author: author.trim() || undefined,
      identifiers: identifiers.length > 0 ? identifiers : undefined,
    });
  };
```

- [ ] **Step 4: Add author input and identifier chips to the search UI**

In the search step UI, after the search bar `<div className="flex gap-2">...</div>` block (after line 213), add:

```tsx
            {/* Author and identifier filters */}
            <div className="space-y-3">
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">Author</Label>
                <Input
                  className="h-8 text-sm"
                  onChange={(e) => setAuthor(e.target.value)}
                  onKeyDown={handleKeyDown}
                  placeholder="Author name (optional)"
                  value={author}
                />
              </div>
              {identifiers.length > 0 && (
                <div className="space-y-1">
                  <Label className="text-xs text-muted-foreground">
                    Identifiers
                  </Label>
                  <div className="flex flex-wrap gap-1.5">
                    {identifiers.map((id, i) => (
                      <Badge
                        className="max-w-full gap-1 pr-1"
                        key={`${id.type}-${id.value}-${i}`}
                        variant="secondary"
                      >
                        <span
                          className="truncate"
                          title={`${id.type}:${id.value}`}
                        >
                          {formatIdentifierType(id.type, pluginIdentifierTypes)}:{" "}
                          {id.value}
                        </span>
                        <button
                          className="shrink-0 rounded-sm hover:bg-muted-foreground/20 p-0.5 cursor-pointer"
                          onClick={() =>
                            setIdentifiers(identifiers.filter((_, j) => j !== i))
                          }
                          type="button"
                        >
                          <X className="h-3 w-3" />
                        </button>
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
            </div>
```

Add the `X` import at the top of the file (line 2). Update the lucide-react import to include `X`:

```typescript
import { ExternalLink, Loader2, Search, X } from "lucide-react";
```

- [ ] **Step 5: Run linting and verify**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise lint:js 2>&1 | tail -10`

Expected: No errors.

- [ ] **Step 6: Commit**

```bash
git add app/components/library/IdentifyBookDialog.tsx app/hooks/queries/plugins.ts
git commit -m "[Frontend] Add author input and identifier chips to identify dialog

Pre-fills author from first book author and identifiers from all files.
Author is editable, identifiers are individually dismissable chips.
Both are sent as part of the search payload."
```

---

### Task 4: Update Documentation

**Files:**
- Modify: `website/docs/plugins/development.md` (update SearchContext docs)
- Modify: `pkg/plugins/CLAUDE.md` (update hook docs and scan pipeline section)

- [ ] **Step 1: Update plugin development docs**

In `website/docs/plugins/development.md`, find the metadata enricher search context documentation and update it to show the new flat shape:

```javascript
metadataEnricher: {
  search: function(context) {
    // context.query       — search query (title or free text)
    // context.author      — author name (optional)
    // context.identifiers — [{ type, value }] (optional)

    // Check for ISBN in query (users may paste ISBNs into the search box)
    var isbnMatch = context.query.match(/^97[89]\d{10}$/);
    if (isbnMatch) {
      // Direct ISBN lookup
    }

    // Use author to narrow results
    var searchUrl = "https://api.example.com/search?q=" + shisho.url.encodeURIComponent(context.query);
    if (context.author) {
      searchUrl += "&author=" + shisho.url.encodeURIComponent(context.author);
    }

    // Check for known identifiers
    if (context.identifiers) {
      for (var i = 0; i < context.identifiers.length; i++) {
        var id = context.identifiers[i];
        if (id.type === "isbn_13") {
          // Direct lookup by ISBN
        }
      }
    }

    return { results: [...] };
  }
}
```

Remove any references to `context.book` or `context.file` from the enricher documentation.

- [ ] **Step 2: Update pkg/plugins/CLAUDE.md**

Update the metadataEnricher hook documentation to show the new context shape. Remove `context.book` and `context.file` references. Update the scan pipeline integration section to reflect the flat context.

- [ ] **Step 3: Commit**

```bash
git add website/docs/plugins/development.md pkg/plugins/CLAUDE.md
git commit -m "[Docs] Update plugin docs for simplified SearchContext

Document the new flat { query, author, identifiers } search context.
Remove context.book and context.file references."
```

---

### Task 5: Run Full Validation

**Files:** None (validation only)

- [ ] **Step 1: Run mise check:quiet**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise check:quiet`

Expected: All checks pass.

- [ ] **Step 2: Verify no stale context.book references**

Run: `grep -r "context\.book\|context\.file\|bookCtx\|fileCtx" pkg/plugins/ pkg/worker/ --include="*.go" | grep -v "_test.go" | grep -v CLAUDE`

Expected: No matches in production code.

Run: `grep -r "context\.book\|context\.file" packages/plugin-sdk/ website/docs/plugins/`

Expected: No matches.
