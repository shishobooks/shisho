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
    ).toBe("Shisho web reader");
    expect(
      sessionSourceLabel({
        source: "web_reader",
        kind: "listening",
        measured: true,
      }),
    ).toBe("Shisho web player");
    expect(
      sessionSourceLabel({
        source: "koreader",
        kind: "reading",
        measured: false,
      }),
    ).toBe("KOReader (estimated)");
  });

  it("names the Shisho app's reader and player apart", () => {
    expect(
      sessionSourceLabel({
        source: "shisho_app",
        kind: "reading",
        measured: true,
      }),
    ).toBe("Shisho app reader");
    expect(
      sessionSourceLabel({
        source: "shisho_app",
        kind: "listening",
        measured: true,
      }),
    ).toBe("Shisho app player");
  });
});

describe("placeRows", () => {
  it("splits the web into the reader and the player, most time first", () => {
    expect(
      placeRows([
        {
          source: "web_reader",
          device_id: null,
          device_name: null,
          reading_seconds: 600,
          listening_seconds: 3000,
        },
        {
          source: "kobo",
          device_id: null,
          device_name: "Bedside Kobo",
          reading_seconds: 1200,
          listening_seconds: 0,
        },
      ]),
    ).toEqual([
      {
        label: "Shisho web player",
        reading_seconds: 0,
        listening_seconds: 3000,
      },
      { label: "Bedside Kobo", reading_seconds: 1200, listening_seconds: 0 },
      {
        label: "Shisho web reader",
        reading_seconds: 600,
        listening_seconds: 0,
      },
    ]);
  });

  it("leaves out the half of the web with no time, and names a removed device by its kind", () => {
    expect(
      placeRows([
        {
          source: "web_reader",
          device_id: null,
          device_name: null,
          reading_seconds: 600,
          listening_seconds: 0,
        },
        {
          source: "koreader",
          device_id: null,
          device_name: null,
          reading_seconds: 60,
          listening_seconds: 0,
        },
      ]),
    ).toEqual([
      {
        label: "Shisho web reader",
        reading_seconds: 600,
        listening_seconds: 0,
      },
      { label: "KOReader", reading_seconds: 60, listening_seconds: 0 },
    ]);
  });

  it("names a Shisho app Device by name, and a removed one by its reader and player", () => {
    expect(
      placeRows([
        {
          source: "shisho_app",
          device_id: null,
          device_name: "iPhone 17 Pro",
          reading_seconds: 60,
          listening_seconds: 600,
        },
        {
          source: "shisho_app",
          device_id: null,
          device_name: null,
          reading_seconds: 120,
          listening_seconds: 300,
        },
      ]),
    ).toEqual([
      { label: "iPhone 17 Pro", reading_seconds: 60, listening_seconds: 600 },
      {
        label: "Shisho app player",
        reading_seconds: 0,
        listening_seconds: 300,
      },
      {
        label: "Shisho app reader",
        reading_seconds: 120,
        listening_seconds: 0,
      },
    ]);
  });
});
