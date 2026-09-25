---
name: splash
description: Use when creating or updating the README splash image for Shisho
---

# Splash Image Generation

## Overview

Shisho's README splash is a 900x600 PNG with the logo on a dark gradient background. Generated from HTML using Playwright screenshots. The same source also drives the docs site social card and the Patreon cover.

## Dimensions

- **Size**: 900x600 pixels
- **Output**: `assets/splash.png`
- **Source**: `assets/splash.html`

## Design Specs

| Element | Value |
|---------|-------|
| Background | Radial gradient `ellipse 90% 80% at 50% 45%`: `#2c2c30` center, `#1a1a1c` at 55%, `#121214` edge |
| Purple glow | `rgba(139, 92, 246, 0.16)` 640x360 ellipse centered behind the logo, fading to transparent at 70% |
| Font | Noto Sans, 700 weight, `line-height: 1` |
| Text color | `#fafafa` (white) |
| Font size | 108px |
| Letter spacing | 0.04em |
| Kanji size | 60px, 400 weight, letter spacing 0.08em |
| Kanji color | `#c4b5fd` (violet-300) |
| Kanji position | Top edge flush with the cap height of SHISHO (`position: relative; top: -29px`) |
| Icon size | 92x92px |
| Icon position | Shelf sits on the text baseline (`align-items: baseline` plus `translateY(9px)`) |
| Icon-text gap | 18px |

The kanji offset was measured, not eyeballed: after rendering, the top row of the white text and the top row of the violet kanji must match. Check with ImageMagick:

```bash
magick assets/splash.png -fuzz 4% -fill black +opaque "#fafafa" -trim -format "%wx%h+%X+%Y\n" info:
magick assets/splash.png -gravity East -crop 35%x100%+0+0 +repage -fuzz 8% -fill black +opaque "#c4b5fd" -trim -format "%wx%h+%X+%Y\n" info:
```

Both `+Y` values must be equal. If you change the font size, re-measure and adjust `top` on `.kanji`.

## HTML Source

The splash is rendered from `assets/splash.html`:
- Uses Google Fonts (Noto Sans)
- Flexbox layout, baseline aligned
- Shelf icon inline SVG (the refined mark with highlights and shadows; see the `favicon` skill for the construction and the list of copies to keep in sync)
- "SHISHO" in uppercase with "司書" superscript

## Generation Process

Run from the repo root so `playwright` resolves from the repo's node_modules. Fonts load from Google, so wait for `document.fonts.ready` before the screenshot.

```js
import { chromium } from "playwright";
import { pathToFileURL } from "url";
import { resolve } from "path";

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 900, height: 600 }, deviceScaleFactor: 1 });
await page.goto(pathToFileURL(resolve("assets/splash.html")).href);
await page.evaluate(() => document.fonts.ready);
await page.waitForTimeout(300);
await page.screenshot({ path: "assets/splash.png" });
await browser.close();
```

Then copy the result to the docs site social card, which is the same image:

```bash
cp assets/splash.png website/static/img/shisho-social-card.png
```

## Patreon Assets

`assets/patreon/` holds the Patreon page images. They are built from the same pieces:

| File | Size | Source |
|------|------|--------|
| `cover.png` | 2500x625 | `cover.html`, the splash layout scaled up (150px wordmark, 128px icon, 84px kanji at `top: 19px`) with a wider gradient and the logo centered for the mobile crop |
| `profile.png` | 1024x1024 | `public/favicon.svg` with `rx="0"` and the mark shrunk to `translate(76, 61) scale(7.5)`, rendered full-bleed and opaque. Patreon applies its own circular mask, and the extra padding keeps the shelf clear of the circle edge |
| `profile-rounded.png` | 1024x1024 | `public/favicon.svg` as-is, transparent outside the rounded corners |

Render the cover the same way as the splash with a 2500x625 viewport, and the profile images with the favicon skill's SVG-to-PNG approach at size 1024. Re-measure the kanji alignment on the cover after any typography change.

## Key Differences from Logo Component

| Property | Logo.tsx | splash.html |
|----------|----------|-------------|
| Letter spacing | 0.05em (`tracking-wider`) | 0.04em |
| Icon alignment | Vertically centered (`items-center`) | Sits on the baseline |
| Kanji alignment | `align-super` | Top flush with cap height |
| Icon margin | `mr-1` (4px) | 18px gap |
| Font | System font stack | Noto Sans |

## Files

- `assets/splash.html` - Editable HTML source
- `assets/splash.png` - Generated PNG for README (also copied to `website/static/img/shisho-social-card.png`)
- `assets/patreon/` - Patreon cover and profile images with the cover source
