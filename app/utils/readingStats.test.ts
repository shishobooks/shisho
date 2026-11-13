import { describe, expect, it } from "vitest";

import { placeRows, sessionSourceLabel } from "@/utils/readingStats";

describe("sessionSourceLabel", () => {
  it("names the web reader and the web player apart, and marks estimates", () => {
    expect(
      sessionSourceLabel({
        source: "web_reader",
        kind: "reading",
        measured: true,
      }),
    ).toBe("Web reader");
    expect(
      sessionSourceLabel({
        source: "web_reader",
        kind: "listening",
        measured: true,
      }),
    ).toBe("Web player");
    expect(
      sessionSourceLabel({
        source: "koreader",
        kind: "reading",
        measured: false,
      }),
    ).toBe("KOReader (estimated)");
  });
});

describe("placeRows", () => {
  it("splits the web into the reader and the player, most time first", () => {
    expect(
      placeRows([
        {
          source: "web_reader",
          device_name: null,
          reading_seconds: 600,
          listening_seconds: 3000,
        },
        {
          source: "kobo",
          device_name: "Bedside Kobo",
          reading_seconds: 1200,
          listening_seconds: 0,
        },
      ]),
    ).toEqual([
      { label: "Web player", reading_seconds: 0, listening_seconds: 3000 },
      { label: "Bedside Kobo", reading_seconds: 1200, listening_seconds: 0 },
      { label: "Web reader", reading_seconds: 600, listening_seconds: 0 },
    ]);
  });

  it("leaves out the half of the web with no time, and names a removed device by its kind", () => {
    expect(
      placeRows([
        {
          source: "web_reader",
          device_name: null,
          reading_seconds: 600,
          listening_seconds: 0,
        },
        {
          source: "koreader",
          device_name: null,
          reading_seconds: 60,
          listening_seconds: 0,
        },
      ]),
    ).toEqual([
      { label: "Web reader", reading_seconds: 600, listening_seconds: 0 },
      { label: "KOReader", reading_seconds: 60, listening_seconds: 0 },
    ]);
  });
});
