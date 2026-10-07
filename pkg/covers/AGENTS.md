# Cover Serving and Thumbnails

- **Serve covers through `ServeBookCover` or `ServeFileCover`, after authorizing** the book, file, library, or Share Link. Neither helper checks access, and no thumbnail parameter bypasses it.
- **API callers pass the one `ThumbnailCache` built in `server.New`.** Never build another instance for the same cache directory: request deduplication, resize concurrency, the LRU index, and cache clearing belong to the instance, so a second one would track, evict, and clear only its own work.
- **Bump `ThumbnailRenderKey` when changing resize or encoding settings**, or browsers keep showing thumbnails from the old renderer.
