---
sidebar_position: 40
---

# Supported Formats

Shisho supports a variety of book formats across three media types.

All supported formats are fingerprinted with a content sha256 hash for move
and rename detection. Future versions will add format-specific fuzzy
fingerprints (cover pHash, text SimHash, etc.) — see
[File Fingerprints](./file-fingerprints.md).

## Ebooks

- **EPUB** — Full [metadata extraction](./metadata#epub) including title, authors, series, description, cover art, language, and more. Includes an in-app reader with font size, theme, flow (paginated or scrolled), and auto-hide controls
- **PDF** — Full [metadata extraction](./metadata#pdf) including title, authors, description, cover art, page count, language, and chapter extraction from PDF bookmarks. Includes an in-app viewer with fit-width/fit-height modes and auto-hide controls

## Audiobooks

- **M4B** — Full [metadata extraction](./metadata#m4b) including title, authors, narrators, series, chapters, cover art, language, and abridged status. Includes an in-app player with play/pause, a draggable seek bar, and elapsed/total time, showing the cover, title, author, and narrator. When the file has chapters, the player adds chapter navigation: a dropdown that jumps to a chapter's start, chapter markers along the seek bar, the current chapter shown and updated live as playback crosses a boundary, and previous/next chapter buttons (previous restarts the current chapter when more than about 5 seconds in, otherwise jumps to the prior chapter). It also has skip back and forward buttons (30 seconds), mapped to the left and right arrow keys. A file with no chapters plays normally with chapter navigation absent

## Comics

- **CBZ** — Full [metadata extraction](./metadata#cbz) from ComicInfo.xml including title, authors, series, cover art, and language. Includes an in-app viewer with fit-width/fit-height modes and auto-hide controls

## Downloads

Shisho can generate download files in additional formats:

- **KePub** — Kobo-optimized EPUB format for [Kobo e-readers](./kobo-sync)
