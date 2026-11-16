# The Shisho app downloads Original Files and shows metadata only from its own copy of the catalog

Devices normally receive a Generated File, so the copy carries current metadata. The **Shisho app** downloads and streams the **Original File** instead, in its own File Type, and never reads titles, authors, covers, or chapters from inside the file. It shows them from its local copy of the catalog, which follows the server's changes. A metadata edit on the server shows up in the app at its next sync, even for a Download made long before, and nothing is downloaded again.

## Considered options

- **Download the Generated File, as other Devices do.** Rejected because a downloaded copy goes stale the moment its metadata is edited. Keeping it current would mean downloading it again, for example a whole 1 GB audiobook because one chapter was renamed.

## Consequences

- **Delivery Rules and Device scope don't apply to the Shisho app.** This narrows ADR 0011, where every Device type supplies preset Delivery Rules. The app's Device type sets no limit on File Types, a Book's ebook and audiobook Files are downloaded independently, and an app Device always covers every Library its user can access.
- **The app reads a Generated File in only two cases:**
  - a File Type the app's reader can't open, which it reads through a **Conversion**;
  - sharing or exporting a file out of the app, which fetches the current Generated File so the copy carries Shisho's metadata.

  In both cases the server builds the file into its download cache before the download starts, so even a first build downloads with Range support and a known size. This narrows ADR 0010, which streams a first build while it's made.
- **Positions line up with the web reader.** The EPUB generator rewrites only the package document, the navigation documents, and the cover, so a location in an Original File points to the same text in its Generated File.
- **Chapters come from Shisho, not from the file, wherever they can be navigated to.** The player and the reader's table of contents use Shisho's chapter list for EPUB, CBZ, PDF, and M4B Files, so chapter edits apply to existing Downloads. For MOBI and AZW3 Files, the app uses the table of contents inside the file until Shisho's stored chapter locations are mapped to places the app's reader can go to.
