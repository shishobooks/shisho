# Sidecar Metadata Files

Shisho stores portable metadata beside media files as JSON sidecars. Sidecars let scans recover supported book and file metadata without modifying the source media.

Sidecars are Shisho metadata portability artifacts, not complete database backups. They do not capture libraries, users, permissions, lists, review state, jobs, plugin configuration, source-priority history, or every database relationship. Intrinsic file properties such as file size, bitrate, duration, codec, and page count are intentionally excluded.

## Naming and Location

Shisho writes two sidecar levels:

- A **book sidecar** named `{book-name}.metadata.json`.
- A **file sidecar** named `{complete-media-filename}.metadata.json`.

Directory-based book:

```text
[Author] Book Title/
├── book.epub
├── book.epub.metadata.json
└── [Author] Book Title.metadata.json
```

Root-level book:

```text
library/
├── Book Title.m4b
├── Book Title.m4b.metadata.json
└── Book Title.metadata.json
```

Keep sidecars beside the media they describe. Shisho-generated cover files use a separate naming convention and are not embedded in the JSON.

## Book Sidecar Schema

The current schema version is `1`:

```json
{
  "version": 1,
  "title": "The Great Gatsby",
  "sort_title": "Great Gatsby, The",
  "subtitle": "A Novel",
  "description": "A story about the American Dream.",
  "authors": [
    {
      "name": "F. Scott Fitzgerald",
      "sort_name": "Fitzgerald, F. Scott",
      "sort_order": 0,
      "role": "writer"
    }
  ],
  "series": [
    {
      "name": "Classic American Literature",
      "sort_name": "Classic American Literature",
      "number": 1,
      "number_end": 3,
      "unit": "volume",
      "sort_order": 0
    }
  ],
  "genres": ["Classic", "Fiction"],
  "tags": ["1920s", "american-literature"]
}
```

All metadata fields are optional, but keep `"version": 1`. Constraints include:

- `number_end` requires `number` and must be greater than it. Shisho treats `number`, `number_end`, and `unit` as one group and ignores the whole group when it is malformed.
- Series `unit` may be `"volume"` or `"chapter"`. It affects CBZ series numbering; other formats ignore it.
- Author `role` applies to CBZ creator roles. Supported values are `writer`, `penciller`, `inker`, `colorist`, `letterer`, `cover_artist`, `editor`, and `translator`.

## File Sidecar Schema

```json
{
  "version": 1,
  "name": "Custom Display Name",
  "narrators": [
    {
      "name": "Stephen Fry",
      "sort_name": "Fry, Stephen",
      "sort_order": 0
    }
  ],
  "publisher": "Penguin Books",
  "release_date": "2004-09-30",
  "url": "https://example.com/book",
  "identifiers": [
    { "type": "isbn_13", "value": "9780743273565" },
    { "type": "asin", "value": "B000FC1GJC" }
  ],
  "chapters": [
    {
      "title": "Chapter 1",
      "start_timestamp_ms": 0,
      "children": [
        { "title": "Section 1.1", "start_timestamp_ms": 30000 }
      ]
    }
  ],
  "cover_page": 0,
  "language": "en-US",
  "abridged": true
}
```

File-sidecar constraints include:

- `release_date` uses `YYYY-MM-DD`.
- Built-in identifier types include `isbn_10`, `isbn_13`, `asin`, `uuid`, `goodreads`, `google`, and `other`. Plugins may define additional types.
- `cover_page` is a zero-indexed page number for CBZ and PDF.
- `language` is a BCP 47 tag such as `en`, `en-US`, or `zh-Hans`.
- `abridged` is `true`, `false`, or omitted when unknown.

Each chapter uses the position field for its media format:

| Format | Position Field | Meaning |
|--------|----------------|---------|
| EPUB | `href` | Content-document reference |
| CBZ | `start_page` | Zero-indexed page number |
| PDF | `start_page` | Zero-indexed page number |
| M4B | `start_timestamp_ms` | Milliseconds from the start |

Do not mix position fields within a chapter. Chapters may include nested `children` where the format supports a hierarchy.

## Priority and Aliases

During a scan, metadata precedence is:

| Priority | Source |
|----------|--------|
| Highest | Manual edits in Shisho |
| | Sidecar metadata |
| | Plugin metadata |
| | Embedded file metadata |
| Lowest | Filepath-derived metadata |

A sidecar can override plugin, embedded, and filepath values, but it does not override a field that still has manual priority in the database. See [Metadata](./metadata.md) for rescan and priority behavior.

Names for authors, narrators, series, genres, tags, and publishers use Shisho's normal alias resolution. A sidecar name that matches an alias resolves to the existing canonical resource instead of creating a duplicate.

## Reading and Writing

Library scans read sidecars and then write the resulting current metadata back to sidecars. Metadata edits in Shisho also write the affected sidecars. These writes are best-effort: a failed write does not roll back the scan or edit, and Shisho records the failure in its logs.

The operating-system account running Shisho needs write permission to the media directories. In a container deployment, verify both the mounted path's permissions and that the mount is not read-only. See [Deployment and Maintenance](./deployment-and-maintenance.md).

:::warning
Do not edit sidecars while Shisho is scanning or while someone is editing the same metadata in the web interface. A later Shisho write can overwrite concurrent external changes. Back up the sidecars first, or wait until Shisho is idle before editing.
:::

## Troubleshooting

### A Sidecar Is Ignored

**Symptom:** A scan does not apply values from a sidecar.

**Likely cause:** The JSON is malformed, a field has the wrong type, or a manual-priority database value takes precedence.

**Verify:** Validate the JSON against the schema above and check Shisho's logs for `failed to read book sidecar` or `failed to read file sidecar`. Compare the affected field's source in Shisho.

**Fix:** Correct the JSON and use the appropriate rescan mode described in [Metadata](./metadata.md) when you intend to replace current manual metadata.

### Sidecars Are Missing or Stale

**Symptom:** A scan or metadata edit succeeds, but the corresponding JSON file is absent or unchanged.

**Likely cause:** Shisho cannot write to the media directory or the mount is read-only.

**Verify:** Check directory ownership, mount options, and Shisho's logs for a sidecar write failure.

**Fix:** Grant the Shisho process write access to the media directory or make the mount writable, then repeat the scan or edit. See [Deployment and Maintenance](./deployment-and-maintenance.md).

### Names Resolve Unexpectedly

**Symptom:** A sidecar name links to an unexpected canonical resource or creates a new one.

**Likely cause:** The spelling matches an existing alias, or no intended alias exists.

**Verify:** Compare the sidecar spelling with the canonical name and aliases shown in Shisho.

**Fix:** Correct the sidecar name or configure the intended alias before rescanning. See [Metadata](./metadata.md).

### External Edits Disappear

**Symptom:** A hand-edited sidecar is replaced with different content.

**Likely cause:** A scan or web edit wrote the current database state while the external edit was in progress.

**Verify:** Check whether a scan or metadata edit ran at the same time and preserve the current sidecar before making another change.

**Fix:** Restore the external edit from backup when no scan or web edit is running, then rescan. See [Troubleshooting](./troubleshooting.md) for log guidance.
