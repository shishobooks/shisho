---
sidebar_position: 155
---

# Gallery Size

Pick how large book covers display in your gallery — Small, Medium, Large, or Extra Large. The choice applies everywhere you see covers (library home, series, genres, tags, people, lists).

## Using the Size popover

Click **Size** in the gallery toolbar to open the size popover. Pick S / M / L / XL — covers resize immediately and the gallery jumps to a page that contains the book you were last looking at, so you don't lose your place.

A dot on the Size button means your current size differs from your saved default.

The popover is hidden on small screens, where covers always render two per row regardless of size. Edit your saved default from User Settings on a phone, and it'll apply next time you're on a wider screen.

## Sizes

| Size | Cover width (desktop) | Books per page |
|------|----------------------|----------------|
| S    | 96px                 | 48 |
| M    | 128px (default)      | 24 |
| L    | 176px                | 16 |
| XL   | 224px                | 12 |

Items per page scale so screen density stays roughly constant — bigger covers, fewer per page.

## URL-addressable size

Non-default sizes live in the URL as `?size=s|m|l|xl`. You can share or bookmark a sized view and it loads at that size — for that recipient on that page only.

```
?size=l
```

When the URL has no `size` parameter, the gallery uses your saved default (or **M** if you haven't saved one).

## Saving a default

When your current size differs from your saved default, the size popover shows a **Save as my default everywhere** button. Clicking it:

1. Saves your current size as your new default for every gallery page.
2. Clears the `?size=` parameter from the URL — you're now viewing the default.

Defaults are per user — saving doesn't affect other users.

You can also edit the default from your **User Settings** page under Appearance.

## See also

- [Gallery Sort](./gallery-sort.md) for sorting books by author, series, date added, and more.
