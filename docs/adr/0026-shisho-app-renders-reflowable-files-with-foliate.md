# The Shisho app renders Reflowable Files with foliate under a native page-turn layer

The **Shisho app** lays out **Reflowable** Files with foliate-js, the engine the web reader already uses, inside WebViews. Page turns and gestures are native. Three WebViews hold the current page and the pages on either side, already rendered, and a native drag moves them with the finger and finishes the turn with a spring. Taps queue instead of being dropped. Long presses, text selection, and taps on or near links go to the page. Positions stay EPUB CFIs, so the web reader, the app, and the KOReader and Kobo translators keep one form for a place in the text.

That native layer is one pager shared by every File the app reads. Reflowable pages come from foliate, and Page-based pages from the image pipeline (ADR 0025). Turns, tap zones, chrome, which way pages turn, and the rules for when a tap is a turn and when it is a link are written once, so a fix to one kind of File reaches the others. Only zoom differs: text reflows, and images zoom.

## Considered options

- **foliate as the web reader runs it.** Rejected on the device: it drags pages in JavaScript, and turns felt laggy, as if at a lower refresh rate.
- **Readium's navigator.** It ships selection, highlights, search, and read-aloud. After two rounds of fixes in a prototype it opened faster than foliate, but fast swipes still felt sluggish and dropped input, and rapid taps could leave a page stuck between two pages. Its paging view is where both come from, so native turns would mean keeping a fork of it. Its positions also aren't CFIs, so every source would need one more translation.
- **foliate for Page-based Files too**, so even comics go through one engine. Rejected because the image pager decodes pages at screen size outside the WebView, which keeps memory low and zoom sharp, and foliate's PDF support is experimental.

## Consequences

- **Every fix to the native layer has to keep WebKit's own gestures working.** Selection, the edit menu, and links all depend on the page receiving touches the native layer doesn't claim.
- **Three WebViews each hold a copy of the book.** In the prototype the total stayed below Readium's single navigator, but a very large book needs watching.
- **Entering a very large chapter costs its layout**, up to about half a second in the prototype, so a flick right at that moment turns late.
- **Highlights, search, and read-aloud are built on foliate's own modules**, not on a second engine.
