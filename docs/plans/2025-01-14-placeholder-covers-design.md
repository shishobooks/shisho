# Placeholder Cover Design

## Problem

Books and audiobooks without cover images display poorly - just "no cover" text that's hard to read and visually jarring.

## Solution

SVG placeholder images with:
- Simple outline icons (book for books, headphones for audiobooks)
- Subtle gradient backgrounds using muted primary color tones
- Light and dark mode variants

## Assets

Four SVG files in `app/assets/placeholders/`:

| File | Aspect Ratio | Mode |
|------|--------------|------|
| `placeholder-book-light.svg` | 2/3 (200×300 viewBox) | Light |
| `placeholder-book-dark.svg` | 2/3 (200×300 viewBox) | Dark |
| `placeholder-audiobook-light.svg` | Square (300×300 viewBox) | Light |
| `placeholder-audiobook-dark.svg` | Square (300×300 viewBox) | Dark |

### Gradient Colors

- **Light mode:** `oklch(0.95 0.03 280)` → `oklch(0.90 0.05 280)` (pale lavender, top to bottom)
- **Dark mode:** `oklch(0.25 0.05 280)` → `oklch(0.20 0.03 280)` (dark muted purple, top to bottom)

### Icon Specifications

- **Style:** Outline/stroke, 2px stroke width, rounded caps/joins
- **Size:** ~60px wide, centered
- **Colors:**
  - Light mode: `oklch(0.65 0.08 280)`
  - Dark mode: `oklch(0.55 0.08 280)`

The 60px icon in the 300×300 square placeholder occupies ~20% of width, so it won't be cut off when displayed in a 2/3 aspect ratio container.

## Component

New `CoverPlaceholder.tsx` component:

```tsx
interface CoverPlaceholderProps {
  variant: "book" | "audiobook";
  className?: string;
}

function CoverPlaceholder({ variant, className }: CoverPlaceholderProps) {
  // Detects theme and renders appropriate SVG
}
```

## Integration

Update these components to use `CoverPlaceholder`:

1. **BookItem.tsx** - Gallery thumbnails
2. **BookDetail.tsx** - Large cover display
3. **SeriesList.tsx** - Series thumbnails
4. **GlobalSearch.tsx** - Search result thumbnails
5. **FileEditDialog.tsx** - Cover edit dialog

### Pattern

Replace current `onError` hide-and-show-text with state-based rendering:

```tsx
const [coverError, setCoverError] = useState(false);

{!coverError ? (
  <img src={coverUrl} onError={() => setCoverError(true)} ... />
) : (
  <CoverPlaceholder variant={isAudiobook ? "audiobook" : "book"} />
)}
```

## Edge Cases

- **No cover URL:** Show placeholder immediately (no img tag needed)
- **Cover load failure:** `onError` triggers placeholder display
- **Borders:** Apply same border styling as regular covers via className
