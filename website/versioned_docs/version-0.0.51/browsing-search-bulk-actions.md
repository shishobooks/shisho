# Browsing, Search, and Bulk Actions

Shisho provides library-scoped search, gallery controls, and selection tools for working with many books at once.

## Search

The global search field in the header is scoped to the current library. It searches **Books**, **Series**, and **People**, then groups matching results by resource type. Switch libraries before searching if the item belongs elsewhere.

The search field in the library gallery searches books only. Use it with **Filter** to narrow the gallery without searching series or people.

## Filters

Open **Filter** on the library gallery to combine any of these criteria:

- **File type**
- **Genres**
- **Tags**
- **Language**
- **Review state**, including **Needs review** and **Reviewed**

Active filters appear as removable chips. Filters apply together, so a book must satisfy the current combination.

## Gallery Sort

Open **Sort** to build a multi-level sort. Add levels, choose ascending or descending direction, remove levels, and drag them into priority order. Available fields include **Title**, **Author**, **Series**, **Date added**, **Date released**, **Page count**, and **Duration**.

A sort in the page URL is temporary and can be bookmarked or shared. **Save as my default for this library** stores the current sort for the current user and library. Each user can therefore choose a different default for every library.

The saved default also controls book ordering in that library's [OPDS](./opds.md) feed and [eReader Browser](./ereader-browser.md). A temporary browser URL sort does not change those views.

## Gallery Size

Open **Size** to choose **S**, **M**, **L**, or **XL** covers. The control is available on the library home, the series list, and list detail pages. It is not a per-library setting.

**Save as my default everywhere** stores one global gallery-size preference for the current user. A size in the page URL temporarily overrides that preference on the current page.

The inline **Size** control is hidden on small screens. On those devices, open the user menu, select **User Settings**, and change the gallery size under **Appearance**.

## Selecting Books

Click the explicit **Select** button on the library gallery to enter selection mode. Click books to add or remove them from the selection. Selections remain active as you move between gallery pages.

Shift-click selects a range only within the current page. It cannot create a range across pagination boundaries.

The selection toolbar provides these actions:

- **Add** opens **Add to List**.
- **More** includes **Mark reviewed** and **Mark needs review**.
- **Merge** combines two or more selected books in the same library.
- **Delete** deletes the selected books and their disk content.
- **Download** creates a bulk download.

Merge and delete can have destructive filesystem effects. Read [Managing Books and Files](./managing-books-and-files.md) first.

## Bulk Downloads

Select books, click **Download**, and choose from the available **EPUB**, **CBZ**, **M4B**, and **PDF** types. Shisho includes every matching main file, so a book with multiple editions of the same type contributes each edition. Supplements and sidecar files are not included.

Shisho generates download copies with format-specific metadata:

- EPUB metadata and cover data are written into the EPUB.
- CBZ metadata is written to `ComicInfo.xml`.
- M4B metadata and edited chapters are written into MP4 metadata and chapter structures.
- PDF metadata and edited chapters are written into the PDF information and outline.

The source files in the library are not replaced by these generated copies. Shisho prepares the ZIP in a background job, and you can navigate elsewhere while it runs.

### Bulk Download Permissions

Current backend routing requires all three permissions: `jobs:read`, `jobs:write`, and `books:read`. With the default roles, only **Admin** has the required jobs permissions. Bulk download is therefore Admin-only unless you define a custom role with this combination.

### Bulk Download Cache

Generated files and ZIP archives use the download cache. Cached output has no time-to-live expiration. Shisho evicts older cached output based on usage when the configured cache size limit is exceeded. See [Configuration](./configuration.md#cache) for `download_cache_max_size_gb`.
