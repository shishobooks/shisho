package cbzpages

// CBZPageKey changes when the image a page number names can change for every
// archive at once (the order of cbz.PageImages, or how pages are extracted);
// bump it with any such change. It names cached pages, so pages cached under a
// previous rule are never served, and reaches the browser as the r query
// parameter on CBZ page URLs. A request whose r is absent or stale is served
// private, no-store, so a tab loaded before an upgrade cannot cache a page
// under its old URL.
const CBZPageKey = "2"
