import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { Permission } from "@/utils/permissions";

import { DiscoverTab } from "./DiscoverTab";

// Uses the real query hooks, so their permission gates and the tab's own
// state decide which requests the recommendations send: listing the
// libraries and looking through their folders happen only for a role that
// can install plugins, and only until a metadata enricher is installed.

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const goodreads = {
  compatible: true,
  description: "Enriches book metadata from Goodreads",
  homepage: "",
  id: "goodreads-enricher",
  imageUrl: "",
  is_official: true,
  name: "Goodreads Enricher",
  overview: "",
  scope: "shisho",
  versions: [
    {
      capabilities: {
        identifierTypes: [],
        metadataEnricher: { description: "", fields: [], fileTypes: [] },
      },
      changelog: "",
      compatible: true,
      downloadUrl: "",
      manifestVersion: 1,
      minShishoVersion: "0.1.0",
      releaseDate: "2024-01-01",
      sha256: "",
      version: "1.0.0",
    },
  ],
};

const ADMIN: Permission[] = [
  "books:read",
  "config:read",
  "config:write",
  "libraries:read",
  "libraries:write",
];

const stubAPI = ({ enrichers = [] as unknown[] } = {}) =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    switch (path) {
      case "/plugins/available":
        return [goodreads];
      case "/plugins/order/metadataEnricher":
        return enrichers;
      case "/libraries":
        return {
          items: [
            {
              id: 1,
              library_paths: [{ filepath: "/books", id: 1 }],
              name: "Books",
            },
          ],
          total: 1,
        };
      case "/filesystem/detect":
        return { audiobooks: 0, comics: 0, ebooks: 12, stopped_early: false };
      default:
        return [];
    }
  });

const paths = (request: ReturnType<typeof stubAPI>) =>
  request.mock.calls.map((call) => call[1]);

const renderTab = (canWrite: boolean) =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter>
        <DiscoverTab canWrite={canWrite} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("DiscoverTab recommendation requests", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("lists the libraries and looks through their folders for an admin with no enricher", async () => {
    setAuth({ permissions: ADMIN });
    const request = stubAPI();
    renderTab(true);

    expect(
      await screen.findByText("We found about 12 ebooks in your libraries."),
    ).toBeInTheDocument();
    expect(paths(request)).toContain("/libraries");
    expect(request).toHaveBeenCalledWith(
      "POST",
      "/filesystem/detect",
      { paths: ["/books"] },
      null,
      expect.anything(),
    );
  });

  it("sends neither request once a metadata enricher is installed", async () => {
    setAuth({ permissions: ADMIN });
    const request = stubAPI({
      enrichers: [{ plugin_id: "goodreads-enricher", scope: "shisho" }],
    });
    renderTab(true);

    expect(await screen.findByText("Goodreads Enricher")).toBeInTheDocument();
    expect(paths(request)).toContain("/plugins/order/metadataEnricher");
    expect(paths(request)).not.toContain("/libraries");
    expect(paths(request)).not.toContain("/filesystem/detect");
  });

  it("sends neither request for a role that can't install plugins", async () => {
    setAuth({ permissions: ["books:read", "config:read", "libraries:read"] });
    const request = stubAPI();
    renderTab(false);

    expect(await screen.findByText("Goodreads Enricher")).toBeInTheDocument();
    expect(paths(request)).not.toContain("/libraries");
    expect(paths(request)).not.toContain("/filesystem/detect");
  });

  it("doesn't look through the folders for a role that can't browse them", async () => {
    setAuth({
      permissions: [
        "books:read",
        "config:read",
        "config:write",
        "libraries:read",
      ],
    });
    const request = stubAPI();
    renderTab(true);

    expect(
      await screen.findByRole("group", {
        name: "Recommended for your library",
      }),
    ).toBeInTheDocument();
    expect(paths(request)).toContain("/libraries");
    expect(paths(request)).not.toContain("/filesystem/detect");
  });
});
