---
name: splash
description: Use when creating or updating the README splash image for Shisho
---

# Splash Image Generation

## Overview

Shisho's README splash is a 900x600 PNG with the logo on a dark gradient background. Generated from HTML using Playwright screenshots.

## Dimensions

- **Size**: 900x600 pixels
- **Output**: `assets/splash.png`
- **Source**: `assets/splash.html`

## Design Specs

| Element | Value |
|---------|-------|
| Background | Radial gradient: `#3d3d3d` center → `#1a1a1a` edge |
| Purple glow | `rgba(139, 92, 246, 0.15)` ellipse behind logo |
| Font | Noto Sans, 700 weight |
| Text color | `#fafafa` (white) |
| Font size | 108px |
| Letter spacing | 0.03em |
| Kanji size | 64.8px (0.6 × main font) |
| Kanji color | `#c4b5fd` (violet-300) |
| Icon size | 96x96px |
| Icon-text gap | 16px |

## HTML Source

The splash is rendered from `assets/splash.html`:
- Uses Google Fonts (Noto Sans)
- Flexbox centered layout
- Shelf icon inline SVG
- "SHISHO" in uppercase with "司書" superscript

## Generation Process

1. Start HTTP server: `python3 -m http.server 8770`
2. Navigate Playwright to `http://localhost:8770/assets/splash.html`
3. Set viewport to 900x600
4. Take screenshot, save to `assets/splash.png`

## Key Differences from Logo Component

| Property | Logo.tsx | splash.html |
|----------|----------|-------------|
| Letter spacing | 0.05em (`tracking-wider`) | 0.03em |
| Icon margin | `mr-1` (4px) | 16px gap |
| Font | System font stack | Noto Sans |

The splash uses tighter letter spacing and larger icon gap for the larger display size.

## Files

- `assets/splash.html` - Editable HTML source
- `assets/splash.png` - Generated PNG for README
