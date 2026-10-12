# Stream Generated Files while they are built

A Generated File used to be built in full before the download sent its first byte. For a large file that silence outlasted device clients: KOReader's OPDS download gives up after 15 seconds without data, and its disconnect canceled the build, so every retry started over and the book never arrived. We now stream every Generated File as it is built. Generation runs in the background, detached from any request, and writes into the download cache; each download, the first included, reads that file as it grows, so bytes reach the client within moments and a disconnect never wastes the work. Every format's generator writes front to back so output can leave as soon as it exists: zip formats emit entries in order and put anything that depends on later entries (the KePub OPF and nav) at the end, and PDFs are updated incrementally, the original bytes followed by an appended section, instead of being rewritten through pdfcpu.

## Considered options

- Serving the Original File when generation runs past a deadline, finishing the Generated File in the background for next time. It works for every format with little code, but the same book would sometimes arrive with Shisho's metadata and sometimes without, depending on how busy the server was, and users cannot tell why. Rejected for that inconsistency.
- Keeping the connection open with repeated `100 Continue` responses before the real one. KOReader's HTTP library skips them, but reverse proxies may not pass them on, and many operators run Shisho behind one.

## Consequences

- The first download of a format whose size depends on its content (CBZ, KePub) has no `Content-Length` and no Range support; downloads served from the cache have both. Formats that can compute their size before writing (EPUB, M4B, MOBI, AZW3, PDF) send it.
- HEAD runs only the cheap preparation step and does not generate, so the web UI's pre-download check returns at once.
- A failure after bytes have been sent cuts the connection. The cache remembers it for that file's current metadata: device routes then serve the Original File, and the web UI shows its error with Download Original.
- An incrementally updated PDF keeps its original bytes, so it is not re-optimized and keeps any digital signature valid. Permission-restricted PDFs get an encrypted update; PDFs that need a password to open get the Original File.
- Plugin output generators write a whole file to a path, so they cannot stream without a change to the plugin contract.
