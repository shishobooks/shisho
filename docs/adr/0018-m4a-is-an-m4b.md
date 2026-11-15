# An M4A file is an M4B, not its own File Type

Shisho scans `.m4a` audiobooks as File Type M4B, the way `.azw` and `.prc` files are MOBI. The two are the same format: ffmpeg writes byte-identical files for `.m4a` and `.m4b` output, with AAC and with ALAC, both carrying the `M4A ` brand, and Shisho already accepted `.m4b` files with that brand and parsed them without looking at the brand. An `.m4a` keeps its extension on disk, so **Download unmodified file** returns an `.m4a`. A Generated File is named `.m4b`, matching the audiobook media kind Shisho writes into it, and so is the original when a device or Share Link gets it in place of a Generated File, as an `.azw` is sent as `.mobi`.

## Considered options

- **M4A as its own File Type.** Labels and Generated Files would say M4A, but every place that treats M4B as "the audiobook" (narrators, cover shape and category, the player, the chapter editor, review completeness, delivery) would need to learn a second audiobook type. Users with customized Delivery Rules would have no M4A rule, so their M4A files would silently never be delivered, and plugin enrichers declaring `m4b` would not run on them without an alias. All of that buys a different label for the same bytes.

## Consequences

- The File Type filter, file badges, and Generated Files say M4B for a file named `.m4a`. Docs list `.m4a` as an M4B extension.
- Shisho has no notion of one audiobook split across several files. Each part of a split `.m4a` audiobook becomes its own M4B File of the Book, as the parts of a split `.m4b` audiobook already do. Multi-file audiobooks, expected to come with MP3 support, are a separate design.
- `.m4a` Files Shisho already tracked, as supplements or through a plugin parser, become M4B main Files on upgrade and keep their records, so an upgraded library matches what a fresh scan of the same files produces.
- Revisit this only if a real structural difference between M4A and M4B turns up that Shisho has to handle.
