# Support MOBI and AZW3 with a hand-written built-in package

The plugin system can already declare a file parser for `mobi` and `azw3`, and the plugin docs use MOBI in their converter and generator examples, so a plugin looks like the obvious route. We build MOBI and AZW3 support into core instead, as a hand-written package beside the EPUB one, and reserve every extension of both file types so plugin parsers cannot claim them. A plugin-parsed type stops at scanning and metadata editing: downloads, OPDS, the web reader, cover selection, and supplement upgrades are wired to built-in file types, so matching EPUB meant core changes either way. No existing Go library could do the job: the MIT-licensed ones are unmaintained, write-only, or too new to trust, and the capable readers (Calibre, KindleUnpack, mobi-go) are GPL, which Shisho's MIT license cannot absorb.

## Considered options

- An official plugin built on the `fileParser` hook. Rejected because every feature beyond scanning needs core changes anyway, and the plugin runtime adds a timeout and a JS boundary to a binary format parser for no gain.
- A GPL library, or calling out to Calibre. Rejected for licensing and for the single-binary image (ADR 0007).

## Consequences

A plugin parser already installed for these types is superseded: its files keep working through the built-in parser, so operators have nothing to do. DRM-protected files are declined when the file is parsed, after plugin input converters run, so a third-party plugin can still decrypt a file into a sibling copy that the built-in parser then imports. Converting MOBI to EPUB is not part of this decision; it waits on a planned per-user redesign of format conversion.
