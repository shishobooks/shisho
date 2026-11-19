import { describe, expect, it } from "vitest";

import type { AvailablePlugin } from "@/hooks/queries/plugins";

import {
  AUDIBLE,
  GOODREADS,
  MANGA,
  OPEN_LIBRARY,
  pluginChoices,
  recommendPlugins,
} from "./pluginRecommendations";

const detected = (ebooks: number, audiobooks: number, comics: number) => ({
  ebooks,
  audiobooks,
  comics,
  stopped_early: false,
});

describe("recommendPlugins", () => {
  it("preselects Goodreads and offers Open Library for ebooks", () => {
    expect(recommendPlugins(detected(4, 0, 0))).toEqual({
      preselected: [GOODREADS],
      offered: [OPEN_LIBRARY],
    });
  });

  it("preselects Audible for audiobooks", () => {
    expect(recommendPlugins(detected(0, 2, 0))).toEqual({
      preselected: [AUDIBLE],
      offered: [],
    });
  });

  it("preselects Manga for comics", () => {
    expect(recommendPlugins(detected(0, 0, 9))).toEqual({
      preselected: [MANGA],
      offered: [],
    });
  });

  it("combines every category found", () => {
    expect(recommendPlugins(detected(1, 1, 1))).toEqual({
      preselected: [GOODREADS, AUDIBLE, MANGA],
      offered: [OPEN_LIBRARY],
    });
  });

  it("preselects Goodreads and offers the other main enrichers when nothing is found", () => {
    const nothing = {
      preselected: [GOODREADS],
      offered: [OPEN_LIBRARY, AUDIBLE, MANGA],
    };
    expect(recommendPlugins(detected(0, 0, 0))).toEqual(nothing);
    expect(recommendPlugins(undefined)).toEqual(nothing);
  });
});

const plugin = (id: string, is_official = true): AvailablePlugin => ({
  scope: is_official ? "shisho" : "community",
  id,
  name: id,
  overview: "",
  description: "",
  homepage: "",
  imageUrl: "",
  is_official,
  compatible: true,
  versions: [
    {
      version: "1.0.0",
      compatible: true,
      changelog: "",
      downloadUrl: "",
      manifestVersion: 1,
      minShishoVersion: "0.1.0",
      releaseDate: "2026-01-01",
      sha256: "",
    },
  ],
});

describe("pluginChoices", () => {
  it("lists recommended plugins first and every other official plugin after", () => {
    const choices = pluginChoices(
      [
        plugin(OPEN_LIBRARY),
        plugin("zeta-parser"),
        plugin(GOODREADS),
        plugin("alpha-converter"),
        plugin(AUDIBLE),
        plugin("community-thing", false),
      ],
      recommendPlugins(detected(3, 0, 0)),
    );

    expect(choices.recommended.map((p) => p.id)).toEqual([
      GOODREADS,
      OPEN_LIBRARY,
    ]);
    expect(choices.others.map((p) => p.id)).toEqual([
      "alpha-converter",
      AUDIBLE,
      "zeta-parser",
    ]);
    expect(choices.preselectedKeys).toEqual([`shisho/${GOODREADS}`]);
  });

  it("leaves out a recommended plugin the repository doesn't list", () => {
    const choices = pluginChoices(
      [plugin(OPEN_LIBRARY)],
      recommendPlugins(detected(3, 0, 0)),
    );

    expect(choices.recommended.map((p) => p.id)).toEqual([OPEN_LIBRARY]);
    expect(choices.preselectedKeys).toEqual([]);
  });
});
