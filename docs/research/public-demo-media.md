# Redistributable representative media for a Public Demo

## Answer

A useful 8 to 12 Book demo corpus is technically easy to assemble, but the researched sources do **not** support calling a four-format corpus globally redistribution-safe without two follow-ups:

1. The strongest real CBZ files found are under Creative Commons NonCommercial licenses. Creative Commons says whether a use is NonCommercial depends on the particular use and its purpose, not merely whether the operator is a nonprofit. It recommends asking the rights holder when the answer is unclear. A Public Demo that promotes Shisho therefore should not rely on NC without permission or a legal decision ([CC FAQ, "Does my use violate the NonCommercial clause?"](https://creativecommons.org/faq/#does-my-use-violate-the-noncommercial-clause-of-the-licenses)).
2. The strongest small, genuine M4B files found are LibriVox recordings. LibriVox says its recordings are public domain "definitely in the USA" but not necessarily elsewhere, and tells users outside the USA to check local status ([LibriVox public-domain policy](https://librivox.org/pages/public-domain/)). A globally reachable service should not silently turn that United States conclusion into a worldwide one.

Three Books are green candidates now: **Open Advice**, **Software Engineering: Standing on the Shoulders of Giants**, and **Made with Creative Commons**. They supply small, first-party EPUB and PDF files under explicit worldwide CC BY-SA licenses. The remaining candidates below form a realistic ten-Book manifest, but are deliberately marked amber where a territorial or NC decision remains.

This is media research, not legal advice. No media file was added to the worktree or retained. Format and size checks use official page descriptions, first-party release metadata, and direct-response headers. A corpus-building task should still pin hashes and run `epubcheck`, ZIP/CBZ integrity checks, PDF validation, and `ffprobe` before publication.

## License handling rules

- CC BY 4.0 permits sharing and adaptation, including commercial use, but requires appropriate credit, a license link, and a change indication ([CC BY 4.0 legal code](https://creativecommons.org/licenses/by/4.0/legalcode.en)).
- CC BY-SA 4.0 adds ShareAlike for adapted material ([CC BY-SA 4.0 legal code](https://creativecommons.org/licenses/by-sa/4.0/legalcode.en)). CC BY-SA 3.0 has the same practical attribution and ShareAlike pattern for the Open Advice files ([Open Advice license](https://github.com/Open-Advice/Open-Advice/blob/master/LICENSE)).
- CC BY-NC-SA permits copying and adaptation only for uses that satisfy the license's NonCommercial condition, and adapted material must remain under the required compatible license ([CC BY-NC-SA 4.0 legal code](https://creativecommons.org/licenses/by-nc-sa/4.0/legalcode.en), [CC BY-NC-SA 3.0 legal code](https://creativecommons.org/licenses/by-nc-sa/3.0/legalcode.en), [CC BY-NC-SA 2.5 legal code](https://creativecommons.org/licenses/by-nc-sa/2.5/legalcode.en)).
- CC licenses do not grant trademark rights. In particular, Creative Commons permits descriptive use of its marks but not use that implies endorsement or association ([Creative Commons trademark policy](https://creativecommons.org/policies/#trademark)). The demo should display titles and source credits neutrally and should not describe itself as endorsed by any creator or project.

## Candidate manifest

Sizes below are decimal MB rounded from the byte counts reported by the official host, except where an official storefront reports only a rounded size.

### A. Open Advice: FOSS: What We Wish We Had Known When We Started

| Field | Finding |
| --- | --- |
| Exact files | [`Open-Advice.epub`](http://open-advice.org/Open-Advice.epub), 0.42 MB; [`Open-Advice.pdf`](http://open-advice.org/Open-Advice.pdf), 2.48 MB. The project offers both formats from its official site and publishes the source repository ([official site](http://open-advice.org/), [source repository](https://github.com/Open-Advice/Open-Advice)). |
| Rights basis | The book and its content are CC BY-SA 3.0 ([project license](https://github.com/Open-Advice/Open-Advice/blob/master/LICENSE)). This is an explicit worldwide license, not a public-domain inference. |
| Notice | Credit the title, editor Lydia Pintscher, and the listed contributors; link the source and CC BY-SA 3.0; state whether Shisho changed the files. Keep adaptations under CC BY-SA 3.0. The repository supplies a full bibliographic citation and contributor list ([project README](https://github.com/Open-Advice/Open-Advice#referencing-this-book)). |
| Technical and representative value | The server identifies the exact files as EPUB and PDF, and the project documents building all formats from the same source ([project README](https://github.com/Open-Advice/Open-Advice#building-a-pdf)). One Book can therefore demonstrate two Files, many authors, a publisher/editor, and a substantial nonfiction table of contents at only about 2.9 MB combined. |
| Caveats | The official downloads are HTTP-only and the HTTPS certificate does not match the host. Acquisition should occur once in a controlled environment, then be hashed and vendored into the immutable corpus. Do not fetch these files dynamically at demo startup. |
| Status | **Green. Recommended.** |

### B. Software Engineering: Standing on the Shoulders of Giants, generic edition 1.0b16

| Field | Finding |
| --- | --- |
| Exact files | [`swebook-generic.epub`](https://github.com/tghastings/open-swe-book/releases/download/1.0b16/swebook-generic.epub), 5.11 MB; [`swebook-generic.pdf`](https://github.com/tghastings/open-swe-book/releases/download/1.0b16/swebook-generic.pdf), 12.55 MB. These byte sizes and immutable tag URLs come from the project's first-party [1.0b16 release](https://github.com/tghastings/open-swe-book/releases/tag/1.0b16). |
| Rights basis | Prose, figures, diagrams, and other non-code content are CC BY-SA 4.0; code snippets are MIT ([repository license](https://github.com/tghastings/open-swe-book/blob/1.0b16/LICENSE)). Both allow redistribution and modification, including commercial use, under their notice conditions. |
| Notice | Use the project's suggested attribution, "Software Engineering: Standing on the Shoulders of Giants, by the open contributors of this repository, licensed under CC BY-SA 4.0"; link the tagged source and licenses; note modifications. Preserve the MIT copyright and permission notice for substantial code portions ([repository license](https://github.com/tghastings/open-swe-book/blob/1.0b16/LICENSE)). |
| Technical and representative value | The project publishes matching EPUB/PDF editions for each tagged release and describes fifteen chapters, an appendix, figures, exercises, and code examples ([project README](https://github.com/tghastings/open-swe-book/tree/1.0b16#software-engineering-standing-on-the-shoulders-of-giants)). The generic edition gives one Book two Files and exercises long-document navigation, code formatting, and technical metadata. |
| Caveats | The release is labeled beta (`1.0b16`) and the project says its prose was drafted with AI assistance under author direction and review ([project README](https://github.com/tghastings/open-swe-book/tree/1.0b16#how-this-book-was-made-ai-assistance)). Pin the tag rather than `latest`, and present that provenance without implying Shisho endorsement. |
| Status | **Green. Recommended.** |

### C. Made with Creative Commons

| Field | Finding |
| --- | --- |
| Exact file | [`made-with-cc.pdf`](https://creativecommons.org/wp-content/uploads/2017/04/made-with-cc.pdf), 5.29 MB, served as `application/pdf` by Creative Commons. |
| Rights basis | The book by Paul Stacey and Sarah Hinchliff Pearson states that it is CC BY-SA 4.0 ([official PDF](https://creativecommons.org/wp-content/uploads/2017/04/made-with-cc.pdf)). |
| Notice | Credit Paul Stacey and Sarah Hinchliff Pearson, title, 2017, Creative Commons; link CC BY-SA 4.0; indicate changes; ShareAlike applies to adaptations ([CC BY-SA 4.0 legal code](https://creativecommons.org/licenses/by-sa/4.0/legalcode.en)). |
| Technical and representative value | It is a compact, professionally laid out book with chapters, case studies, figures, and a strong cover, useful for testing PDF rendering and organization metadata ([official PDF](https://creativecommons.org/wp-content/uploads/2017/04/made-with-cc.pdf)). |
| Caveats | Creative Commons names and logos remain trademarks. Reproduce the unmodified book and use marks only descriptively, without suggesting that Creative Commons sponsors the Public Demo ([trademark policy](https://creativecommons.org/policies/#trademark)). |
| Status | **Green. Recommended.** |

### D. Alice's Adventures in Wonderland

| Field | Finding |
| --- | --- |
| Exact files | Standard Ebooks compatible EPUB, [`lewis-carroll_alices-adventures-in-wonderland_john-tenniel.epub`](https://standardebooks.org/ebooks/lewis-carroll/alices-adventures-in-wonderland/john-tenniel/downloads/lewis-carroll_alices-adventures-in-wonderland_john-tenniel.epub?source=download), 10.64 MB; LibriVox version 8 M4B, [`AliceWonderland8_librivox.m4b`](https://archive.org/download/aliceinwonderland_2106_librivox/AliceWonderland8_librivox.m4b), 102.70 MB. Both official catalog pages identify the work and format ([Standard Ebooks edition](https://standardebooks.org/ebooks/lewis-carroll/alices-adventures-in-wonderland/john-tenniel), [LibriVox version 8](https://librivox.org/alices-adventures-in-wonderland-by-lewis-carroll-8/), [LibriVox host metadata](https://archive.org/metadata/aliceinwonderland_2106_librivox)). |
| Rights basis | Standard Ebooks dedicates its contributors' work to the worldwide public domain through CC0, but says the underlying text and artwork are only believed to be public domain in the United States and may remain copyrighted elsewhere ([edition uncopyright](https://standardebooks.org/ebooks/lewis-carroll/alices-adventures-in-wonderland/john-tenniel/text/uncopyright)). LibriVox donates recordings to the public domain but expressly guarantees that conclusion only in the United States ([LibriVox policy](https://librivox.org/pages/public-domain/)). |
| Notice | Neither project requires attribution for United States public-domain reuse, though LibriVox asks to be credited. Good practice is to credit Lewis Carroll, John Tenniel, Standard Ebooks and its producers, LibriVox, and the named readers, with links to both catalog pages. Modifications are allowed where the public-domain conclusions apply ([Standard Ebooks uncopyright](https://standardebooks.org/ebooks/lewis-carroll/alices-adventures-in-wonderland/john-tenniel/text/uncopyright), [LibriVox policy](https://librivox.org/pages/public-domain/)). |
| Technical and representative value | This is the best multi-format candidate: the illustrated EPUB exercises images and navigation, while the genuine 102.7 MB M4B exercises audiobook playback and chapter metadata. The LibriVox page identifies it as children's/fantastic fiction and the official host lists the exact M4B file ([Standard Ebooks edition](https://standardebooks.org/ebooks/lewis-carroll/alices-adventures-in-wonderland/john-tenniel), [LibriVox page](https://librivox.org/alices-adventures-in-wonderland-by-lewis-carroll-8/), [host metadata](https://archive.org/metadata/aliceinwonderland_2106_librivox)). |
| Caveats | Do not call this worldwide-cleared based only on these sources. Verify the underlying text, Tenniel illustrations, particular recording, embedded cover, and neighboring rights in the deployment's required territories. Avoid Disney-derived art and branding; these files use the cited Standard Ebooks/LibriVox editions, not Disney material. |
| Status | **Amber, territorial review. Strongest public-domain multi-format candidate.** |

### E. The Importance of Being Earnest

| Field | Finding |
| --- | --- |
| Exact file | Standard Ebooks compatible EPUB, [`oscar-wilde_the-importance-of-being-earnest.epub`](https://standardebooks.org/ebooks/oscar-wilde/the-importance-of-being-earnest/downloads/oscar-wilde_the-importance-of-being-earnest.epub?source=download), 0.33 MB ([official edition page](https://standardebooks.org/ebooks/oscar-wilde/the-importance-of-being-earnest)). |
| Rights basis | Standard Ebooks dedicates its contributors' work worldwide through CC0, but describes the source text and artwork as believed public domain in the United States and warns about other jurisdictions ([edition uncopyright](https://standardebooks.org/ebooks/oscar-wilde/the-importance-of-being-earnest/text/uncopyright)). |
| Notice | No attribution is required for rights covered by CC0 or the applicable public domain. Still credit Oscar Wilde, Standard Ebooks, and the edition's producers from the [colophon](https://standardebooks.org/ebooks/oscar-wilde/the-importance-of-being-earnest/text/colophon). Modifications are allowed where the public-domain conclusion applies. |
| Technical and representative value | The host identifies the exact compatible EPUB as `application/epub+zip`. This very small play adds drama, different subject metadata, and a short reading experience ([official edition page](https://standardebooks.org/ebooks/oscar-wilde/the-importance-of-being-earnest)). |
| Caveats | The project's rights determination is United States-specific. Complete a territory check before global publication. |
| Status | **Amber, territorial review.** |

### F. A Christmas Carol

| Field | Finding |
| --- | --- |
| Exact file | Standard Ebooks compatible EPUB, [`charles-dickens_a-christmas-carol.epub`](https://standardebooks.org/ebooks/charles-dickens/a-christmas-carol/downloads/charles-dickens_a-christmas-carol.epub?source=download), 0.47 MB ([official edition page](https://standardebooks.org/ebooks/charles-dickens/a-christmas-carol)). |
| Rights basis | Standard Ebooks dedicates its contributions worldwide through CC0, while limiting its source-text/art public-domain belief to the United States and warning that other countries can differ ([edition uncopyright](https://standardebooks.org/ebooks/charles-dickens/a-christmas-carol/text/uncopyright)). |
| Notice | No attribution is required where CC0/public domain applies. Still credit Charles Dickens, Standard Ebooks, and the producers in the [colophon](https://standardebooks.org/ebooks/charles-dickens/a-christmas-carol/text/colophon). Modifications are allowed where the public-domain conclusion applies. |
| Technical and representative value | The exact download is served as `application/epub+zip`; the compact novella adds fiction, holiday subject matter, and a recognizable cover at under 0.5 MB ([official edition page](https://standardebooks.org/ebooks/charles-dickens/a-christmas-carol)). |
| Caveats | The project's rights determination is United States-specific. Complete a territory check before global publication. Use only the cited edition's artwork, not later film or character adaptations. |
| Status | **Amber, territorial review.** |

### G. The Velveteen Rabbit

| Field | Finding |
| --- | --- |
| Exact file | LibriVox M4B, [`velveteen_rabbit_librivox.m4b`](https://archive.org/download/velveteen_rabbit_librivox/velveteen_rabbit_librivox.m4b), 13.91 MB. The official catalog identifies the 1922 work by Margery Williams, and its official download host metadata lists the exact M4B ([LibriVox page](https://librivox.org/the-velveteen-rabbit-by-margery-williams/), [host metadata](https://archive.org/metadata/velveteen_rabbit_librivox)). |
| Rights basis | LibriVox says recordings and associated catalog materials are donated to the public domain, definitely in the United States but not necessarily in other countries ([LibriVox policy](https://librivox.org/pages/public-domain/)). |
| Notice | LibriVox says no credit is required where its public-domain conclusion applies, but asks for a link. Credit Margery Williams, narrator Marlo Dianne, and LibriVox; link the catalog page. Modifications are allowed where the public-domain conclusion applies ([LibriVox policy](https://librivox.org/pages/public-domain/)). |
| Technical and representative value | At 13.9 MB, this is the smallest complete real M4B located. It is a practical playback candidate and a short children's audiobook, though less useful than Alice for testing a long chapter list ([LibriVox page](https://librivox.org/the-velveteen-rabbit-by-margery-williams/), [host metadata](https://archive.org/metadata/velveteen_rabbit_librivox)). |
| Caveats | Verify text, sound-recording/performer, embedded cover, and neighboring rights for required territories. "Public domain in the USA" is not enough by itself for a worldwide clearance statement. |
| Status | **Amber, territorial review. Preferred M4B for size.** |

### H. Electric Puppet Theatre, Chapter 1: Round Midnight

| Field | Finding |
| --- | --- |
| Exact files | [`EPT001.cbz`](https://eptcomic.com/EPT001.cbz), 5.37 MB, served as `application/vnd.comicbook+zip`; [`EPT001.pdf`](https://eptcomic.com/EPT001.pdf), 4.00 MB, served as `application/pdf`. The creator's official extras page labels both as Chapter 1 downloads ([official extras](https://eptcomic.com/extras.htm)). |
| Rights basis | The creator says all comics are CC BY-NC-SA 3.0 and explicitly permits modification and distribution under that license ([creator FAQ](https://eptcomic.com/faq.htm#Licensing)). |
| Notice | Credit Electric Puppet Theatre and the credited creators, link the source and CC BY-NC-SA 3.0, indicate changes, and ShareAlike adaptations. The license does not allow uses primarily intended for commercial advantage. |
| Technical and representative value | This is the strongest small, real CBZ candidate: it is a creator-published `.cbz`, not a renamed ZIP or third-party conversion, and the same Book also has a PDF. The creator warns that its vector PDF renders badly in PDF.js and recommends the CBZ as an alternative, which is useful for demonstrating format-specific behavior ([official extras](https://eptcomic.com/extras.htm)). |
| Caveats | Whether a product-promotional Public Demo is NonCommercial is unresolved. The creator invites requests for commercial or otherwise uncovered uses ([creator FAQ](https://eptcomic.com/faq.htm#Licensing)); obtain written permission before moving this candidate to green. |
| Status | **Amber, NC permission needed. Preferred CBZ candidate.** |

### I. Plague Rat, Chapter 2: Culture Shock

| Field | Finding |
| --- | --- |
| Exact file | `Plague Rat 02 - CBZ`, 20 MB. The creator's itch.io page identifies PDF, CBR, and CBZ editions, lists the CBZ as a 20 MB buyer download, and sets a US$3 minimum price ([creator storefront](https://rabbitdance.itch.io/plague-rat-c2-culture-shock)). |
| Rights basis | The creator storefront labels the work CC BY-NC-SA 4.0 ([creator storefront](https://rabbitdance.itch.io/plague-rat-c2-culture-shock)). The license permits redistribution and adaptation subject to attribution, NonCommercial, and ShareAlike ([CC BY-NC-SA 4.0 legal code](https://creativecommons.org/licenses/by-nc-sa/4.0/legalcode.en)). |
| Notice | Credit RabbitDance and the work, link the creator page and CC BY-NC-SA 4.0, indicate changes, and license adaptations compatibly. |
| Technical and representative value | It is a genuine creator-supplied CBZ with a complete black-and-white manga-style chapter and extras, useful alongside the color Electric Puppet Theatre candidate. Its 20 MB storefront size remains modest ([creator storefront](https://rabbitdance.itch.io/plague-rat-c2-culture-shock)). |
| Caveats | Purchase does not remove the NC condition. Obtain written permission for the Public Demo and retain proof of purchase and permission. Use title/character branding only to identify the work, without suggesting creator endorsement. |
| Status | **Amber, purchase plus NC permission needed. Good CBZ alternate.** |

### J. Tales from the Public Domain: Bound by Law?

| Field | Finding |
| --- | --- |
| Exact file | Screen PDF, [`cspdcomicscreen.pdf`](https://web.law.duke.edu/cspd/comics/pdf/cspdcomicscreen.pdf), 8.49 MB. Duke also offers a 17.70 MB high-resolution PDF, but the screen edition better fits the corpus size target ([official downloads](https://web.law.duke.edu/cspd/comics/digital/)). |
| Rights basis | Duke's Center for the Study of the Public Domain publishes the comic under CC BY-NC-SA 2.5 ([official work page](https://web.law.duke.edu/cspd/comics/)). |
| Notice | Credit Keith Aoki, James Boyle, and Jennifer Jenkins, Duke's Center for the Study of the Public Domain, and the title; link the source and CC BY-NC-SA 2.5; indicate changes; ShareAlike adaptations ([official publication listing](https://web.law.duke.edu/cspd/publications/), [license](https://creativecommons.org/licenses/by-nc-sa/2.5/legalcode.en)). |
| Technical and representative value | The official source describes it as a full downloadable comic and provides separate screen/high-resolution PDFs and remix assets. It gives the corpus an illustrated educational PDF with different layout behavior from prose books ([official downloads](https://web.law.duke.edu/cspd/comics/digital/)). |
| Caveats | The NC question is the same as for both CBZ candidates. Obtain permission or an explicit legal decision before use. Do not infer that Duke endorses Shisho. |
| Status | **Amber, NC permission needed.** |

## Recommendation-ready shortlist

Use the following ten-Book layout as the working manifest, with only green rows approved for immediate packaging:

| Book | Files | Why it earns a place | Gate |
| --- | --- | --- | --- |
| Open Advice | EPUB + PDF | Tiny multi-file Book, many contributors, long nonfiction metadata | Green |
| Software Engineering: Standing on the Shoulders of Giants | EPUB + PDF | Modern technical layout, code, diagrams, pinned release | Green |
| Made with Creative Commons | PDF | Strong visual PDF and explicit CC BY-SA provenance | Green |
| Alice's Adventures in Wonderland | EPUB + M4B | Best cross-format Book and richly illustrated EPUB | Territory review |
| The Importance of Being Earnest | EPUB | Very small drama and distinct metadata | Territory review |
| A Christmas Carol | EPUB | Very small familiar novella | Territory review |
| The Velveteen Rabbit | M4B | Smallest complete genuine M4B found | Territory review |
| Electric Puppet Theatre, Chapter 1 | CBZ + PDF | Best small first-party real CBZ | Written NC permission |
| Plague Rat, Chapter 2 | CBZ | Contrasting modern manga-style real CBZ | Purchase and written NC permission |
| Bound by Law? | PDF | Illustrated educational PDF | Written NC permission |

If the corpus must be globally cleared with no legal interpretation and no bespoke permissions, stop at the three green Books. To reach the map's 8 to 12 Book goal safely, the next planning step should require worldwide redistribution permission for at least one real CBZ and one real M4B, or replace those formats with newly found files under CC BY, CC BY-SA, or CC0. This research does not justify weakening that gate.

## Rejected or weak leads

- **Pepper&Carrot:** David Revoy's official project clearly licenses the comic CC BY 4.0 and provides source/raster downloads ([official source page](https://www.peppercarrot.com/en/webcomic-sources/ep26_Books-Are-Great.html), [CC BY 4.0 legal code](https://creativecommons.org/licenses/by/4.0/legalcode.en)). I found third-party CBZ packaging but no exact first-party CBZ download. It is therefore an excellent content source but not a verified real-CBZ candidate for this ticket.
- **The Vacinator:** the creator page labels the project CC BY-SA and offers `The Vacinator.zip`, not a `.cbz` ([creator page](https://art-de-poubelle.itch.io/the-vacinator)). Renaming a generic ZIP would violate the requirement to find a real CBZ, so it is rejected.
- **Fodongo editions:** the creator provides clear CC BY-SA edition-level terms, with item-specific exceptions, but the downloads are PDF rather than CBZ ([Fodongo: A Free Culture Comics Zine #3](https://jectoons.itch.io/fodongo-a-free-culture-comics-zine-3), [Fodongo: A Free-Culture Comics Zine Issue #12](https://jectoons.itch.io/fodongo-issue-12)). They are possible PDF alternates, not CBZ answers.
- **Random Internet Archive or comic-aggregator CBZ records:** a download badge or uploader-supplied CC tag does not prove that the uploader owns the comic rights. Candidates whose license could not be traced to the creator or responsible project were rejected rather than treated as redistributable.
- **Other Standard Ebooks and Project Gutenberg titles:** free download is not worldwide clearance. Standard Ebooks itself warns that its underlying works may remain protected outside the United States ([example uncopyright](https://standardebooks.org/ebooks/lewis-carroll/alices-adventures-in-wonderland/john-tenniel/text/uncopyright)). The manifest keeps only a few old, useful candidates and preserves the territorial gate.
- **MP3-only Creative Commons audiobooks:** they may be excellent audio, but converting or merely renaming one does not produce a pre-existing representative M4B candidate. They were excluded from this ticket.

## Newly surfaced fog

1. Is the Public Demo's purpose considered NonCommercial when it is free to use but promotes adoption of Shisho? If that is not an unambiguous yes, seek direct creator permission rather than relying on NC.
2. Which territories must be cleared: only the server's hosting jurisdiction, every visitor's jurisdiction, or a documented launch-country set? The answer controls whether LibriVox and Standard Ebooks candidates can move from amber to green.
3. Is the project willing to request and retain creator permissions for an immutable corpus? One permission each for Electric Puppet Theatre and a LibriVox recording could be simpler and safer than continued searching.
4. Where will attribution and license notices live so they remain visible when original-file downloads are disabled? The corpus specification should include a machine-readable manifest plus a visible per-Book notice.
5. The immutable corpus build needs a provenance record containing source URL, retrieval date, cryptographic hash, exact license text/version, attribution string, permission evidence where applicable, and validation output. None of those should depend on live third-party URLs at runtime.
