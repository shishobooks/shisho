# KOReader sync identifies Files by hashes of the exact bytes Shisho delivered

**Context.** KOReader's progress sync protocol names a document by a partial MD5 of the file's bytes, which is its default matching mode. Shisho rarely serves an Original File's bytes. Instead it delivers Generated Files that carry current metadata, often after a Conversion chosen by the Device's Delivery Rules (ADR 0011). Their bytes change whenever metadata, a Conversion, or the generator changes.

**Decision.** Shisho computes KOReader's partial MD5 of each Original File during a Scan, and of every Generated File the download cache publishes (ADR 0010). It keeps a lasting map from each hash to the File and the delivered File Type it came from. One hash can map to several Files when identical bytes exist more than once, such as the same EPUB in two libraries, so a lookup resolves through the user's library access.

**Result.** A copy on a device keeps syncing after later metadata edits, because its old hash still maps to the File. The delivered File Type tells Shisho which variant the device holds; a KePub's element paths differ from its EPUB's. A hash Shisho never produced gets KOReader's "no progress" response and is logged.

## Considered options

- **Matching by filename**, KOReader's other mode. Rejected because download names aren't unique across Files or stable across metadata edits, and users rename files on their devices. This mode is not supported.
- **Matching by the optional title and author metadata KOReader can send.** Rejected as the primary key because it's off by default and ambiguous across editions. It may still help explain a "no progress" in the logs.
- **Hashing only what Shisho would generate now.** Rejected because any copy downloaded before a metadata edit or generator change would stop syncing.
- **Hashing while a download streams.** Rejected because cache hits are served from the published file, and bulk downloads zip cached files, so most deliveries never pass through a stream. Hashing at publish covers every delivery path, because each delivered Generated File is published to the cache.

## Consequences

- **The map grows** by one small row per distinct generated output, and is never pruned while the File exists.
- **A copy whose bytes Shisho never produced doesn't sync.** That includes a copy that differs by even one byte, such as one edited elsewhere and copied over by hand. KOReader caches a document's hash in its own sidecar, so a file replaced in place on the device keeps reporting the hash of the copy it replaced.
