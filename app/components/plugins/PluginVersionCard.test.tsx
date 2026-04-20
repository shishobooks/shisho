import { render, screen } from "@testing-library/react";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";

import type { PluginVersion } from "@/hooks/queries/plugins";

import { PluginVersionCard } from "./PluginVersionCard";

const makeVersion = (
  overrides: Partial<PluginVersion> = {},
): PluginVersion => ({
  capabilities: undefined,
  changelog: "",
  compatible: true,
  downloadUrl: "",
  manifestVersion: 1,
  minShishoVersion: "0.0.0",
  releaseDate: "",
  sha256: "",
  version: "1.0.0",
  ...overrides,
});

describe("PluginVersionCard date formatting", () => {
  beforeAll(() => {
    // Pin to a known "now" so the "relative" string is deterministic.
    vi.setSystemTime(new Date("2026-04-20T12:00:00Z"));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it("treats a date-only releaseDate as local, not UTC", () => {
    // "2026-04-14" must render as "Apr 14, 2026" in any timezone —
    // parsing it as UTC midnight and formatting with toLocaleDateString
    // would show "Apr 13, 2026" west of UTC.
    render(
      <PluginVersionCard
        state="latest"
        version={makeVersion({ releaseDate: "2026-04-14" })}
      />,
    );
    const released = screen.getByText(/released/i);
    expect(released.textContent).toMatch(/Apr 14, 2026/);
  });

  it("still handles RFC3339 timestamps", () => {
    render(
      <PluginVersionCard
        state="latest"
        version={makeVersion({ releaseDate: "2026-04-14T12:00:00Z" })}
      />,
    );
    const released = screen.getByText(/released/i);
    expect(released.textContent).toMatch(/Apr 14, 2026/);
  });

  it("omits the Released line when releaseDate is empty", () => {
    render(
      <PluginVersionCard
        state="latest"
        version={makeVersion({ releaseDate: "" })}
      />,
    );
    expect(screen.queryByText(/released/i)).toBeNull();
  });
});
