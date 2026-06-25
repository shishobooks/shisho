---
sidebar_position: 160
---

# Fetch Chapters from Audible

For M4B audiobook files, Shisho can look up chapter titles and timestamps from Audible's catalog and stage them into the chapter editor. This is useful for files ripped from Audible that have missing or wrong chapter metadata.

## Requirements

- File must be an M4B audiobook.
- You need an Audible ID (ASIN) for the book. You can copy it from the URL of the book's Audible page. For example, in `https://www.audible.com/pd/Example-Audiobook/B0036UC2LO`, the 10-character code at the end is the ASIN.
- Your user must have **books:write** permission (Editor or Admin role).

## How it works

1. Open the chapter edit view for an M4B file. Click **Edit** to enter edit mode, then click **Fetch from Audible** next to the "Add Chapter" button. If the file has no chapters yet, you can click **Fetch from Audible** directly from the empty state.
2. Paste or confirm the ASIN. If the file already has an Audible identifier set, it is prefilled.
3. Click **Fetch**.
4. Shisho shows the Audible runtime, your file's duration, and the chapter count on each side.
5. Shisho auto-detects whether your file has the Audible intro removed (as some ripping tools like Libation do by default). The resulting offset is reflected in a checkbox that you can flip if the detection is wrong.
6. Choose how to apply:
   - **Apply titles only** replaces chapter titles in place but keeps your existing timestamps. Available only when the chapter counts match.
   - **Apply titles + timestamps** replaces everything with Audible data, respecting the offset checkbox.
7. The dialog closes and the new data is staged in the edit form. Use the per-chapter play buttons to spot-check timestamps, adjust anything that's off, and click **Save** to commit. Click **Cancel** to discard everything, including the fetched data.

## Duration mismatch

If the Audible runtime and your file duration differ by more than 2 seconds (even with intro/outro offsets applied), Shisho shows a mismatch warning. The chapter data may still be usable, but timestamps are likely off. This often means you're looking at a different edition of the audiobook on Audible.

## Data source

Chapter data comes from [Audnexus](https://audnex.us), a community-run proxy of Audible chapter data. Shisho caches each ASIN response for 24 hours to minimize upstream calls.
