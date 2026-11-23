import { describe, expect, it } from "vitest";

import { EPUB } from "@/libraries/foliate/epub.js";

// A package whose metadata has no Dublin Core elements, as a converter that
// wrote `dc="..."` instead of `xmlns:dc="..."` leaves it: the title is in
// the OPF namespace, and nothing is in Dublin Core's.
const files: Record<string, string> = {
  "META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
  "OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="uid">
  <metadata dc="http://purl.org/dc/elements/1.1/" opf="http://www.idpf.org/2007/opf">
    <title>A Book</title>
    <meta name="cover" content="cover"/>
  </metadata>
  <manifest>
    <item id="one" href="one.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="one"/></spine>
</package>`,
  "OEBPS/one.xhtml": `<?xml version="1.0"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>One</title></head><body><p>One</p></body></html>`,
};

describe("vendored epub.js patches", () => {
  it("opens a book whose metadata has no Dublin Core elements", async () => {
    const book = await new EPUB({
      loadText: async (name: string) => files[name] ?? null,
      loadBlob: async () => new Blob(),
      getSize: () => 0,
    }).init();
    expect(book.sections).toHaveLength(1);
  });
});
