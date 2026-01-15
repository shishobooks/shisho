---
name: favicon
description: Use when creating or updating favicon files, app icons, or PWA icons for Shisho
---

# Favicon Generation

## Overview

Shisho uses a bookshelf icon on a dark rounded square for its favicon. Generate from SVG source using Playwright screenshots and ImageMagick resizing.

## Required Sizes

| File | Size | Purpose |
|------|------|---------|
| `favicon.svg` | 512x512 viewBox | Vector source, modern browsers |
| `favicon.ico` | 16x16 + 32x32 | Legacy browsers |
| `favicon-16.png` | 16x16 | Small browser tabs |
| `favicon-32.png` | 32x32 | Standard browser tabs |
| `favicon-192.png` | 192x192 | Android/PWA |
| `favicon-512.png` | 512x512 | PWA splash, high-res |
| `apple-touch-icon.png` | 180x180 | iOS home screen |

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

1. **Create temp HTML** with the SVG at 512x512
2. **Screenshot with Playwright** at 512x512 viewport
3. **Resize with ImageMagick** for other sizes:
   ```bash
   magick favicon-512.png -resize 192x192 favicon-192.png
   magick favicon-512.png -resize 180x180 apple-touch-icon.png
   magick favicon-512.png -resize 32x32 favicon-32.png
   magick favicon-512.png -resize 16x16 favicon-16.png
   magick favicon-16.png favicon-32.png favicon.ico
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
- `favicon.ico`, `favicon-*.png`, `apple-touch-icon.png` - Generated
- `assets/favicon-gen.html` - Preview page for all sizes
