// Package reflowable translates positions in reflowable books between the
// forms each reader keeps: the web reader's CFI, KOReader's XPointer, and
// Kobo's span. Every form becomes a Place, a spine item plus how far through
// its text, in one variant of a File, and a Place becomes any form in another
// variant.
package reflowable
