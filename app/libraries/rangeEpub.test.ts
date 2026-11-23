import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { openRangeEPUB } from "@/libraries/rangeEpub";
import { RangeFile } from "@/libraries/rangeFile";
import { createRangeServer, type RangeServer } from "@/testing/rangeServer";
import { storedZip } from "@/testing/zip";

const URL = "/api/books/files/42/download";
const PHOTO_SIZE = 3 * 1024 * 1024;
const PIE_SIZE = 1024 * 1024;
const FONT_SIZE = 300 * 1024;

const chapter = (title: string, body: string) => `<?xml version="1.0"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>${title}</title></head>
<body><h1>${title}</h1>${body}</body></html>`;

// An EPUB whose photo is most of the file, as in a cookbook.
const cookbook = () =>
  storedZip([
    ["mimetype", "application/epub+zip"],
    [
      "META-INF/container.xml",
      `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
    ],
    [
      "OEBPS/content.opf",
      `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="id">urn:uuid:1</dc:identifier>
    <dc:title>Dessert Book</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="intro" href="intro.xhtml" media-type="application/xhtml+xml"/>
    <item id="cake" href="cake.xhtml" media-type="application/xhtml+xml"/>
    <item id="photo" href="photo.jpg" media-type="image/jpeg"/>
    <item id="pies" href="text/pies.xhtml" media-type="application/xhtml+xml"/>
    <item id="pie1" href="images/pie%201.jpg" media-type="image/jpeg"/>
    <item id="pie2" href="images/pie2.jpg" media-type="image/jpeg"/>
    <item id="pie3" href="images/pie3.jpg" media-type="image/jpeg"/>
    <item id="tarts" href="text/tarts.xhtml" media-type="application/xhtml+xml"/>
    <item id="galettes" href="text/galettes.xhtml" media-type="application/xhtml+xml"/>
    <item id="style" href="css/style.css" media-type="text/css"/>
    <item id="font1" href="fonts/a.otf" media-type="font/otf"/>
    <item id="font2" href="fonts/b.otf" media-type="font/otf"/>
    <item id="font3" href="fonts/c.otf" media-type="font/otf"/>
  </manifest>
  <spine><itemref idref="intro"/><itemref idref="cake"/><itemref idref="pies"/><itemref idref="tarts"/><itemref idref="galettes"/></spine>
</package>`,
    ],
    ["OEBPS/intro.xhtml", chapter("Introduction", "<p>Bake often.</p>")],
    [
      "OEBPS/cake.xhtml",
      chapter("Cake", '<p>Layer cake.</p><img src="photo.jpg" alt=""/>'),
    ],
    ["OEBPS/photo.jpg", new Uint8Array(PHOTO_SIZE).fill(0xff)],
    [
      "OEBPS/text/pies.xhtml",
      chapter(
        "Pies",
        ["pie%201", "pie2", "pie3"]
          .map((name) => `<img src="../images/${name}.jpg" alt=""/>`)
          .join(""),
      ),
    ],
    ["OEBPS/images/pie 1.jpg", new Uint8Array(PIE_SIZE).fill(1)],
    ["OEBPS/images/pie2.jpg", new Uint8Array(PIE_SIZE).fill(2)],
    ["OEBPS/images/pie3.jpg", new Uint8Array(PIE_SIZE).fill(3)],
    [
      "OEBPS/text/galettes.xhtml",
      chapter(
        "Galettes",
        '<img src="../images/pie%201.jpg" alt=""/>' +
          `<div style="background-image: url('../images/pie2.jpg')"></div>` +
          '<div class="crust"></div>',
      ).replace(
        "<title>Galettes</title>",
        "<title>Galettes</title><style>.crust { background: url(../images/pie3.jpg); }</style>",
      ),
    ],
    [
      "OEBPS/text/tarts.xhtml",
      chapter("Tarts", "<p>Press in crust.</p>").replace(
        "<title>Tarts</title>",
        '<title>Tarts</title><link rel="stylesheet" href="../css/style.css"/>',
      ),
    ],
    [
      "OEBPS/css/style.css",
      ["a", "b", "c"]
        .map(
          (name) =>
            `@font-face { font-family: ${name}; src: url("../fonts/${name}.otf"); }`,
        )
        .join("\n"),
    ],
    ["OEBPS/fonts/a.otf", new Uint8Array(FONT_SIZE).fill(4)],
    ["OEBPS/fonts/b.otf", new Uint8Array(FONT_SIZE).fill(5)],
    ["OEBPS/fonts/c.otf", new Uint8Array(FONT_SIZE).fill(6)],
  ]);

describe("openRangeEPUB", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;
  let server: RangeServer;

  beforeEach(() => {
    server = createRangeServer(cookbook());
    fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation(server.fetch);
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("opens the book without fetching its photos", async () => {
    const book = await openRangeEPUB(await RangeFile.open(URL));

    expect(book.metadata.title).toBe("Dessert Book");
    expect(book.sections).toHaveLength(5);
    const doc = await book.sections[0].createDocument();
    expect(doc.body.textContent).toContain("Bake often.");
    expect(server.bytesSent()).toBeLessThan(PHOTO_SIZE / 4);
  });

  it("knows each section's size without reading it", async () => {
    const book = await openRangeEPUB(await RangeFile.open(URL));

    expect(book.sections[1].size).toBeGreaterThan(0);
  });

  it("reads a photo when a page needs it", async () => {
    const book = await openRangeEPUB(await RangeFile.open(URL));

    const photo = await book.loadBlob("OEBPS/photo.jpg");

    expect(photo?.size).toBe(PHOTO_SIZE);
  });

  // foliate's loader asks for a chapter's images one at a time, which costs
  // a round trip each when they come from the server.
  // Loads a chapter as foliate's paginator does, each answer taking a while
  // as over a network. Reports the most requests in flight at once and how
  // many were made.
  const loadChapter = async (index: number) => {
    let inFlight = 0;
    let peak = 0;
    fetchSpy.mockImplementation(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        inFlight++;
        peak = Math.max(peak, inFlight);
        await new Promise((resolve) => setTimeout(resolve, 50));
        inFlight--;
        return server.fetch(input, init);
      },
    );
    const book = await openRangeEPUB(await RangeFile.open(URL));
    peak = 0;
    const asked = server.requests.length;
    const createObjectURL = vi
      .spyOn(globalThis.URL, "createObjectURL")
      .mockReturnValue("blob:x");
    try {
      await book.sections[index].load();
    } finally {
      createObjectURL.mockRestore();
    }
    return { peak, requests: server.requests.length - asked };
  };

  it("fetches a chapter's photos all at once when it loads", async () => {
    const { peak, requests } = await loadChapter(2);

    expect(peak).toBeGreaterThanOrEqual(3);
    // foliate takes the photos fetched ahead rather than asking again: the
    // chapter, then one request per photo.
    expect(requests).toBeLessThanOrEqual(4);
  });

  // A cookbook's stylesheet can name thirty fonts, which foliate also asks
  // for one at a time.
  it("fetches a stylesheet's fonts all at once when it loads", async () => {
    const { peak, requests } = await loadChapter(3);

    expect(peak).toBeGreaterThanOrEqual(3);
    // The chapter, its stylesheet, then one request per font.
    expect(requests).toBeLessThanOrEqual(5);
  });

  // A chapter of a cookbook loads a dozen or more photos at once.
  it("reads a photo in one request", async () => {
    const book = await openRangeEPUB(await RangeFile.open(URL));
    const requests = server.requests.length;
    const sent = server.bytesSent();

    await book.loadBlob("OEBPS/photo.jpg");

    expect(server.requests.length - requests).toBe(1);
    // Not much past the photo's end into the next entry.
    expect(server.bytesSent() - sent).toBeLessThan(PHOTO_SIZE * 1.05);
  });

  it("fetches photos named in a chapter's own styles with the rest", async () => {
    const { peak, requests } = await loadChapter(4);

    expect(peak).toBeGreaterThanOrEqual(3);
    expect(requests).toBeLessThanOrEqual(4);
  });

  // foliate only logs a chapter it couldn't load.
  it("tells the reader when a chapter can't be fetched", async () => {
    const file = await RangeFile.open(URL);
    const book = await openRangeEPUB(file);
    const failed = vi.fn();
    file.onFailed(failed);
    server.failFor(1000);
    const createObjectURL = vi
      .spyOn(globalThis.URL, "createObjectURL")
      .mockReturnValue("blob:x");
    try {
      const loading = book.sections[2].load();
      const settled = expect(loading).rejects.toThrow();
      await vi.advanceTimersByTimeAsync(60_000);
      await settled;
    } finally {
      createObjectURL.mockRestore();
    }

    expect(failed).toHaveBeenCalledWith(expect.any(TypeError));
  });
});
