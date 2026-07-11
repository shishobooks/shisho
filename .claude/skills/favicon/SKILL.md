---
name: favicon
description: Use when creating or updating favicon files, app icons, or PWA icons for Shisho
---

# Favicon Generation

## Overview

Shisho uses a bookshelf icon on a dark rounded square for its favicon. Generate from SVG source by rendering each target size directly with Playwright (transparent background), then bundling the ICO with ImageMagick.

## Required Sizes

| File | Size | Purpose |
|------|------|---------|
| `favicon.svg` | 512x512 viewBox | Vector source, modern browsers |
| `favicon.ico` | 16x16 + 32x32 | Legacy browsers, direct `/favicon.ico` requests |
| `favicon-16.png` | 16x16 | Small browser tabs |
| `favicon-32.png` | 32x32 | Standard browser tabs |
| `apple-touch-icon.png` | 180x180 | iOS home screen (full-bleed, see below) |

There are no 192/512 PNGs: nothing references them (no PWA manifest). If a manifest is added later, generate them then, and make them full-bleed maskable icons, not pre-rounded.

## SVG Source

```svg
<svg width="512" height="512" viewBox="0 0 512 512" fill="none" xmlns="http://www.w3.org/2000/svg">
  <rect width="512" height="512" rx="32" fill="#171717"/>
  <g transform="translate(36, 20) scale(9)">
    <rect x="4" y="40" width="40" height="4" rx="1" fill="#c4b5fd"/>
    <rect x="8" y="12" width="7" height="28" rx="1" fill="#c4b5fd"/>
    <rect x="17" y="8" width="6" height="32" rx="1" fill="#c4b5fd" opacity="0.7"/>
    <rect x="25" y="16" width="8" height="24" rx="1" fill="#c4b5fd"/>
    <rect x="35" y="10" width="5" height="30" rx="1" fill="#c4b5fd" opacity="0.7"/>
  </g>
</svg>
```

**Key values:**
- Background: `#171717` (neutral-900)
- Icon color: `#c4b5fd` (violet-300)
- Border radius: `rx="32"` (subtle, not app-icon style)
- Transform: `translate(36, 20) scale(9)` centers the 48x48 icon

## Generation Process

**Critical rules (violating these reintroduces the white-corner bug):**

- The rounded-corner icons (`favicon-16.png`, `favicon-32.png`, and therefore `favicon.ico`) MUST have transparent corners. Screenshot with Playwright's `omitBackground: true` and a transparent page background. A plain screenshot bakes the white page background into the corners, which shows as light fringing on dark browser tab strips.
- `apple-touch-icon.png` MUST be a full-bleed opaque square with NO rounded corners (`rx="0"` variant of the SVG). iOS applies its own corner mask and renders transparency as black; pre-rounded corners leave visible slivers inside the iOS mask.
- Render the SVG directly at each target size (set both the viewport and the SVG's width/height to the target). Do NOT render large and downscale with ImageMagick; browser rasterization at native size is crisper at 16/32px.

Process:

1. **Render PNGs with Playwright** (run the script from the repo root, e.g. from a scratch file in `tmp/`, so `playwright` resolves from the repo's node_modules; all paths below assume cwd = repo root):
   ```js
   import { chromium } from "playwright";
   import { readFileSync } from "fs";

   const svg = readFileSync("public/favicon.svg", "utf8");
   const fullBleed = svg.replace('rx="32"', 'rx="0"');
   const targets = [
     { file: "public/favicon-16.png", size: 16, svg, transparent: true },
     { file: "public/favicon-32.png", size: 32, svg, transparent: true },
     { file: "public/apple-touch-icon.png", size: 180, svg: fullBleed, transparent: false },
   ];
   const browser = await chromium.launch();
   for (const t of targets) {
     const page = await browser.newPage({ viewport: { width: t.size, height: t.size }, deviceScaleFactor: 1 });
     const sized = t.svg.replace('width="512" height="512"', `width="${t.size}" height="${t.size}"`);
     await page.setContent(`<style>html,body{margin:0;padding:0;background:transparent}svg{display:block}</style>${sized}`);
     await page.screenshot({ path: t.file, omitBackground: t.transparent });
     await page.close();
   }
   await browser.close();
   ```
2. **Bundle the ICO with ImageMagick** (preserves alpha; still cwd = repo root):
   ```bash
   magick public/favicon-16.png public/favicon-32.png public/favicon.ico
   ```
3. **Verify transparency** before committing:
   ```bash
   # Corners of 16/32 PNGs and the ICO frames must be transparent or
   # semi-transparent dark (e.g. srgba(21,21,21,0.34) at 32px,
   # srgba(23,23,23,0.75) at 16px), never opaque white/gray.
   magick public/favicon-32.png -format "alpha: %A, TL: %[pixel:p{0,0}]\n" info:
   magick public/favicon-16.png -format "alpha: %A, TL: %[pixel:p{0,0}]\n" info:
   magick "public/favicon.ico[0]" -format "alpha: %A, TL: %[pixel:p{0,0}]\n" info:
   magick "public/favicon.ico[1]" -format "alpha: %A, TL: %[pixel:p{0,0}]\n" info:
   # apple-touch-icon must be opaque #171717 in all four corners (full-bleed).
   magick public/apple-touch-icon.png -format "TL: %[pixel:p{0,0}]\n" info:
   ```

## HTML References

In `index.html`:
```html
<link rel="icon" type="image/svg+xml" href="/favicon.svg" />
<link rel="icon" type="image/png" sizes="32x32" href="/favicon-32.png" />
<link rel="icon" type="image/png" sizes="16x16" href="/favicon-16.png" />
<link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png" />
```

## Adjusting Icon Position

The transform centers a 48x48 icon viewBox in a 512x512 canvas:
- **scale(N)**: Controls icon size (larger N = bigger icon, less padding)
- **translate(X, Y)**: Shifts position (positive X = right, positive Y = down)

To recenter after changing scale, calculate:
- Content width after scale: `40 * scale` (icon spans x=4 to x=44)
- Content height after scale: `36 * scale` (icon spans y=8 to y=44)
- X offset: `(512 - content_width) / 2 - (4 * scale)`
- Y offset: `(512 - content_height) / 2 - (8 * scale)`

## Files

All favicon files live in `public/`:
- `favicon.svg` - Editable source
- `favicon.ico`, `favicon-16.png`, `favicon-32.png`, `apple-touch-icon.png` - Generated
- `assets/favicon-gen.html` - Preview page for all sizes (dark background so corner fringing is visible)
