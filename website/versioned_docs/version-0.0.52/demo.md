# Public Demo

The Public Demo at [demo.shishobooks.com](https://demo.shishobooks.com) is a read-only Shisho instance with a small library of freely redistributable ebooks, audiobooks, and comics. Use it to see what a deployed Shisho looks like before you install anything. When you are ready to run your own, continue to [Getting Started](./getting-started.md).

## Sign In

Every visitor shares one account:

| Field | Value |
|-------|-------|
| Username | `demo` |
| Password | `shishodemo` |

The sign-in page pre-fills both fields; select **Sign in**. The demo runs on a small server that suspends when nobody is using it, so the first page load after a quiet period can take a few seconds.

## What Works

- Browse the library by series, author, genre, tag, and publisher, and search books, series, and people from the header.
- Read EPUB, CBZ, and PDF files in the in-app readers and listen to M4B audiobooks with the player. See [Reading and Playback](./reading-and-playback.md).
- Change the gallery size, default sort, and reader preferences. The demo keeps them in your browser, so they do not affect other visitors.

## What Is Disabled

The demo rejects every change on the server, so nothing you do can affect the library or the next visitor. Controls that would make a change stay visible; using one shows **This action is unavailable in the demo.**

- Editing books, files, metadata, series, and lists, including cover uploads, rescans, and deletion.
- Downloading files: the download buttons are hidden, and the original, KePub, and bulk download routes are blocked.
- Administration: libraries, users, roles, plugins, and security settings.
- Integrations: [OPDS](./opds.md), [Kobo Sync](./kobo-sync.md), and the [eReader Browser](./ereader-browser.md) are not available.
- Account changes such as the password and server-side preferences.

## About the Library

The demo library contains only works that may be freely redistributed: openly licensed books and public-domain texts, recordings, and comics. Each book's description ends with its credit and license, and the full credits, sources, and modifications are listed in [`CORPUS.md`](https://github.com/shishobooks/demo-corpus/blob/master/CORPUS.md) in the `shishobooks/demo-corpus` repository.

## Next Steps

- [Getting Started](./getting-started.md) walks through the first deployment with Docker Compose.
- [Supported Formats](./supported-formats.md) lists what Shisho can import, read, and generate.
