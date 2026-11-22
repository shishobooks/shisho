import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { RangeFile } from "@/libraries/rangeFile";
import { openRangeMOBI } from "@/libraries/rangeMobi";
import { kf8Book, kf8Image } from "@/testing/kf8";
import { createRangeServer, type RangeServer } from "@/testing/rangeServer";

const URL = "/api/books/files/42/download";

// A two-chapter cookbook with four photos.
const AZW3 = kf8Book({
  title: "Small Cookbook",
  sections: [
    {
      title: "Introduction",
      body: "<p>Bake often, and share what you bake.</p>",
    },
    {
      title: "Cakes",
      body: [0, 1, 2, 3]
        .map(
          (n) =>
            `<p>Cake number ${n + 1}: whisk, fold, bake.</p><img src="${kf8Image(n)}" alt=""/>`,
        )
        .join(""),
    },
  ],
  images: [0, 1, 2, 3].map((n) => new Uint8Array(2000).fill(n)),
});

describe("openRangeMOBI", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;
  let server: RangeServer;

  beforeEach(() => {
    server = createRangeServer(AZW3);
    fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation(server.fetch);
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("opens the book from its parts", async () => {
    const book = await openRangeMOBI(await RangeFile.open(URL));

    expect(book.metadata.title).toBe("Small Cookbook");
    const texts = await Promise.all(
      book.sections.map(
        async (s) => (await s.createDocument()).body.textContent,
      ),
    );
    expect(texts.join(" ")).toContain("Cake number 4: whisk, fold, bake.");
  });

  it("loads a chapter's photos", async () => {
    const book = await openRangeMOBI(await RangeFile.open(URL));
    const created: Blob[] = [];
    const createObjectURL = vi
      .spyOn(globalThis.URL, "createObjectURL")
      .mockImplementation((blob) => {
        created.push(blob as Blob);
        return `blob:${created.length}`;
      });
    try {
      await Promise.all(book.sections.map((s) => s.load()));
    } finally {
      createObjectURL.mockRestore();
    }

    const photos = created.filter((blob) => blob.type === "image/jpeg");
    expect(photos).toHaveLength(4);
  });

  // foliate reads four bytes of every resource record at open, looking
  // for two that books like this don't have.
  it("opens a book of many photos in a few requests", async () => {
    const photos = Array.from({ length: 150 }, (_, n) =>
      new Uint8Array(70_000).fill(n),
    );
    server = createRangeServer(
      kf8Book({
        title: "Photo Book",
        sections: photos.map((_, n) => ({
          title: `Recipe ${n + 1}`,
          body: `<img src="${kf8Image(n)}" alt=""/>`,
        })),
        images: photos,
      }),
    );
    fetchSpy.mockImplementation(server.fetch);

    const book = await openRangeMOBI(await RangeFile.open(URL));

    expect(book.sections).toHaveLength(150);
    expect(server.requests.length).toBeLessThanOrEqual(10);
  });

  it("fetches a chapter's photos all at once", async () => {
    const photos = [0, 1, 2, 3].map((n) => new Uint8Array(400_000).fill(n));
    server = createRangeServer(
      kf8Book({
        title: "Big Photos",
        sections: [
          {
            title: "Pies",
            body: photos
              .map((_, n) => `<img src="${kf8Image(n)}" alt=""/>`)
              .join(""),
          },
        ],
        images: photos,
      }),
    );
    let inFlight = 0;
    let peak = 0;
    fetchSpy.mockImplementation(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        inFlight++;
        peak = Math.max(peak, inFlight);
        // Each answer takes a while, as over a network.
        await new Promise((resolve) => setTimeout(resolve, 50));
        inFlight--;
        return server.fetch(input, init);
      },
    );
    const book = await openRangeMOBI(await RangeFile.open(URL));
    peak = 0;
    const createObjectURL = vi
      .spyOn(globalThis.URL, "createObjectURL")
      .mockReturnValue("blob:x");
    try {
      await book.sections[0].load();
    } finally {
      createObjectURL.mockRestore();
    }

    expect(peak).toBeGreaterThanOrEqual(3);
  });
});
