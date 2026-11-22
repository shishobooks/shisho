# foliate-js (vendored)

Vendored copy of [foliate-js](https://github.com/johnfactotum/foliate-js) used by the in-app reader for EPUB, MOBI, and AZW3 files.

- **Source:** https://github.com/johnfactotum/foliate-js
- **License:** MIT (see `LICENSE` in this directory)
- **Commit:** 76dcd8f0f7ccabd59199fc5eddbe012d8d463b18

foliate-js is distributed as plain ES modules with no build step and no npm release, so we vendor a pinned snapshot.

## Vendored file list

The following files are currently vendored in this directory (excluding `README.md` and `LICENSE`):

```
comic-book.js
epub.js
epubcfi.js
fb2.js
fixed-layout.js
mobi.js
overlayer.js
paginator.js
pdf.js
progress.js
search.js
text-walker.js
tts.js
ui/menu.js
ui/tree.js
vendor/fflate.js
vendor/zip.js
view.js
```

**`pdf.js` is intentionally stubbed — do not re-copy it from upstream.** Upstream foliate's `pdf.js` depends on a vendored pdfjs distribution under `vendor/pdfjs/` that we don't ship. PDFs are handled by the separate `PDFReader` component, so foliate's PDF path is never exercised at runtime — but Vite/Rolldown resolves dynamic imports at build time, so `pdf.js` must still exist *and* all of its own imports must resolve. That's why we keep a stub rather than deleting the file, and why replacing it with upstream's version without also vendoring `vendor/pdfjs/` breaks `pnpm build`.

Some of the other files (e.g. `fb2.js`, `progress.js`, `tts.js`) are not imported directly by our reader either, but are transitive imports pulled in by `view.js` / `epub.js` / the paginator. They must be present for the vendored set to load without module-resolution errors. Do not prune files without first verifying nothing in the closure imports them.

## Local patches

`fixed-layout.js`, `mobi.js`, `paginator.js`, `search.js`, and `view.js` carry changes marked `Shisho patch` in comments. Reapply them after copying a new upstream snapshot unless upstream has fixed the same bug. `app/libraries/foliate-mobi.test.ts` tests the `mobi.js` patches and fails when one or its call site is missing, `app/libraries/foliate-search.test.ts` fails when the `search.js` patch is missing, and `app/libraries/foliate-paginator.test.ts` fails when the paginator's `settled()` is.

## To update

1. Clone or update the upstream repo:
   ```bash
   git clone https://github.com/johnfactotum/foliate-js /tmp/foliate-js-src
   # or, if already cloned:
   cd /tmp/foliate-js-src && git fetch && git checkout <new-sha>
   ```
2. Record the new commit SHA and update the **Commit** field above.
3. Re-copy the LICENSE file:
   ```bash
   cp /tmp/foliate-js-src/LICENSE app/libraries/foliate/LICENSE
   ```
4. Before copying, find the local patches with `grep -n "Shisho patch" app/libraries/foliate/*.js`; see **Local patches** above.
5. Re-copy each file in the **Vendored file list** above from `/tmp/foliate-js-src/` to the matching path under `app/libraries/foliate/`. Preserve the `ui/` and `vendor/` subdirectories. **Exception:** do not re-copy `pdf.js`; see the stub note above. If you need to refresh it, also vendor `vendor/pdfjs/` from upstream.
6. Check for new transitive imports that weren't in the previous snapshot:
   ```bash
   grep -rEn "from ['\"]\.|import\(['\"]\." app/libraries/foliate/
   ```
   For every relative import (`./foo.js`, `./ui/bar.js`, etc.), confirm the target exists in the directory. If any are missing, copy them from upstream and add them to the **Vendored file list** in this README.
7. Update the **Vendored file list** section above to match the new set:
   ```bash
   find app/libraries/foliate -type f -not -name README.md -not -name LICENSE | sort
   ```
8. Reapply the local patches, then run `mise check:quiet` and test with several real EPUB, MOBI, and AZW3 files in the reader, since foliate-js's API occasionally shifts.
9. Recheck the code that opens a book read by part, which uses foliate's internals rather than `view.js`'s API, against the new snapshot. `app/libraries/rangeEpub.ts` builds the loader `makeZipLoader` builds, subclasses zip.js's `BlobReader`, reads zip entries' `offset`, `rawFilename`, and `rawExtraField`, and copies `epub.js`'s `resolveURL`, the `shouldReplace` types, and the selectors and `url()` pattern `Loader.loadReplaced` and `replaceCSS` use. `app/libraries/rangeMobi.ts` opens a book as `makeBook` does, with `MOBI` and `unzlibSync`. `app/types/foliate-epub.d.ts` and `app/types/foliate-mobi.d.ts` declare what both use. A change there can break opening outright or quietly stop a chapter's photos loading at once; `app/libraries/rangeEpub.test.ts` and `app/libraries/rangeMobi.test.ts` catch most of it.
