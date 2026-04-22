# foliate-js (vendored)

Vendored copy of [foliate-js](https://github.com/johnfactotum/foliate-js) used by the in-app EPUB reader.

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

Some of these (e.g. `fb2.js`, `mobi.js`, `pdf.js`, `progress.js`, `search.js`, `tts.js`) are not imported directly by our reader, but are transitive imports pulled in by `view.js` / `epub.js` / the paginator. They must be present for the vendored set to load without module-resolution errors. Do not prune files without first verifying nothing in the closure imports them.

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
4. Re-copy each file in the **Vendored file list** above from `/tmp/foliate-js-src/` to the matching path under `app/libraries/foliate/`. Preserve the `ui/` and `vendor/` subdirectories.
5. Check for new transitive imports that weren't in the previous snapshot:
   ```bash
   grep -rEn "from ['\"]\.|import\(['\"]\." app/libraries/foliate/
   ```
   For every relative import (`./foo.js`, `./ui/bar.js`, etc.), confirm the target exists in the directory. If any are missing, copy them from upstream and add them to the **Vendored file list** in this README.
6. Update the **Vendored file list** section above to match the new set:
   ```bash
   find app/libraries/foliate -type f -not -name README.md -not -name LICENSE | sort
   ```
7. Run `mise check:quiet` and test with several real EPUBs in the reader — foliate-js's API occasionally shifts.
