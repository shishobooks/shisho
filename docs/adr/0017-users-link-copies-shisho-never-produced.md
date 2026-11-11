# Users can link a KOReader copy Shisho never produced, and it syncs by percentage

ADR 0014 matches a KOReader copy to a File only by hashes of bytes Shisho produced, and Shisho only began recording the hashes of Generated Files in v0.7.0. Every copy downloaded earlier, and every copy from Calibre or edited elsewhere, is unknown to Shisho, which leaves most of an existing KOReader library unable to sync. So the **KOReader plugin** reports the hashes it holds data for that no signed-in server recognizes, along with what KOReader knows about each copy (title, authors, identifiers, and reading time). Each such server lists them on the Device's page as unrecognized books, suggests matching Books, and lets the user link one to a File or dismiss it. A link is a hash-to-File mapping made by a person, marked as such. Such a copy syncs Reading Sessions, status, Ratings, and a percentage, but never a locator. Its bytes weren't produced by Shisho, so its XPointers may not line up with the File, and KOReader jumps to a pulled position by percentage. The linked row suggests downloading the Book again for exact positions.

## Considered options

- **Trust a linked copy's XPointers.** Rejected because a wrong locator moves the user to the wrong place in the Shisho reader and on other devices, which is worse than an approximate one.
- **Match unrecognized copies automatically by title and author.** Rejected for the reason in ADR 0014: editions are ambiguous. Matching only suggests; the user decides.
- **List every hash a server doesn't recognize.** Rejected because with several servers, every Book from one server would appear as unrecognized on the others.

## Consequences

- **A hash is listed only when no signed-in server recognizes it or has it linked.** The plugin asks every server which hashes it knows before it reports any as unrecognized, and it waits until every signed-in server has answered since the hash first appeared, so a server that is down doesn't make its Books look unrecognized elsewhere. A link on one server is passed to the others at their next sync, and they drop the hash and its held data. Dismissing affects only the server it's done on.
- **The server holds data for an unrecognized hash** and applies it when the user links it, so the link takes effect without waiting for the device. The plugin treats held data as acknowledged. Dismissing deletes it.
- **A Reading Position can be a percentage with no locator.** Readers that open a File at such a position go to that percentage.
- **One copy is linked on one server.** Linking the same copy on a second server means downloading it from that server.
