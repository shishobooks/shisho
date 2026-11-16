# The Shisho app reads CBZ and PDF Files through the same native image pager

The **Shisho app** shows every **Page-based** File in one native pager. Each page is decoded at screen size off the main thread, and only the page being zoomed is drawn at full resolution. A CBZ supplies its images directly. A PDF has each page rendered to an image on the device, from the downloaded file. Page turns, zoom, Page flow, Reading Direction, spreads, and memory limits then behave the same for both, and a fix to one reaches the other.

This image pipeline is one of two page sources for the app's single native pager. The other is foliate for Reflowable Files (ADR 0026), so page turns and tap zones behave the same for every File.

## Considered options

- **Apple's PDFKit viewer for PDFs.** It handled text PDFs well in the prototype. On a large scanned PDF, though, it dropped hundreds of frames while paging and grew to almost 4 GB of memory that a memory warning didn't release, which on a phone means the app gets killed. Scanned PDFs are a large share of the PDFs a library holds.
- **Server-rendered page images, as the web reader uses.** Rejected because they aren't available offline unless every page is rendered and downloaded ahead of time.

## Consequences

- **PDF text selection and search aren't part of the pager.** If they are wanted later, they come from the PDF's own text layer drawn over the page image, not from switching viewers.
- **A PDF's outline is the table of contents**, through Shisho's chapters for the File (ADR 0022).
- **Page order comes from the server.** A CBZ's page numbers are positions in the order Shisho chooses: which entries count as pages, sorted naturally. The server sends that list with the File, so the apps never sort pages themselves, and a page number means the same page to the web reader, both apps, chapters, and covers.
