import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { toast } from "sonner";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import {
  AUDIBLE,
  GOODREADS,
  MANGA,
  OPEN_LIBRARY,
} from "@/components/setup/pluginRecommendations";
import {
  GettingStartedPanelProvider,
  useGettingStartedPanel,
} from "@/contexts/GettingStarted";
import type {
  AvailablePlugin,
  PluginCapabilities,
} from "@/hooks/queries/plugins";
import { API, ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { DetectFileTypesResponse } from "@/types";

import SetupFlow from "./SetupFlow";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("sonner", async (importOriginal) => {
  const actual = await importOriginal<typeof import("sonner")>();
  return {
    ...actual,
    toast: Object.assign(vi.fn(), {
      error: vi.fn(),
      success: vi.fn(),
      warning: vi.fn(),
    }),
  };
});

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

// Two passes: React renders what the first pass's timers changed (the
// debounced folders) only once act returns, and that render starts requests
// the second pass lets settle.
const flush = async () => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(900);
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(50);
  });
};

let reducedMotion = false;

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

const enricher = (domains: string[]): PluginCapabilities => ({
  identifierTypes: [],
  metadataEnricher: { description: "", fields: [], fileTypes: ["epub"] },
  httpAccess: { description: "", domains },
});

const plugin = (
  id: string,
  name: string,
  domains: string[] = [`*.${id}.example`],
): AvailablePlugin => ({
  scope: "shisho",
  id,
  name,
  overview: `${name} overview`,
  description: "",
  homepage: "",
  imageUrl: "",
  is_official: true,
  compatible: true,
  versions: [
    {
      version: "1.2.0",
      compatible: true,
      capabilities: enricher(domains),
      changelog: "",
      downloadUrl: "",
      manifestVersion: 1,
      minShishoVersion: "0.1.0",
      releaseDate: "2026-01-01",
      sha256: "",
    },
  ],
});

const OFFICIAL = [
  plugin(OPEN_LIBRARY, "Open Library Enricher"),
  plugin(GOODREADS, "Goodreads Enricher", [
    "*.goodreads.com",
    "i.gr-assets.com",
    "m.media-amazon.com",
  ]),
  plugin(AUDIBLE, "Audible Enricher"),
  plugin(MANGA, "Manga Enricher"),
];

interface Server {
  available: AvailablePlugin[] | Error;
  detected: DetectFileTypesResponse;
  failInstall?: string;
  failCreate?: boolean;
  /** Installs wait on this before they answer. */
  holdInstalls?: Promise<void>;
  calls: { method: string; path: string; body: unknown }[];
}

const newServer = (overrides: Partial<Server> = {}): Server => ({
  available: OFFICIAL,
  detected: { ebooks: 12, audiobooks: 0, comics: 0, stopped_early: false },
  calls: [],
  ...overrides,
});

const stubAPI = (server: Server) =>
  vi.spyOn(API, "request").mockImplementation(async (method, path, body) => {
    server.calls.push({ method, path, body });
    if (path === "/plugins/available") {
      if (server.available instanceof Error) throw server.available;
      return server.available;
    }
    if (method === "GET" && path === "/plugins/installed") return [];
    if (method === "POST" && path === "/plugins/installed") {
      const { id, scope, name } = body as {
        id: string;
        scope: string;
        name: string;
      };
      await server.holdInstalls;
      if (id === server.failInstall) {
        throw new ShishoAPIError("Download failed.", "upstream_error", 502);
      }
      return { id, scope, name, status: "active", version: "1.2.0" };
    }
    if (path === "/filesystem/detect") return server.detected;
    if (method === "POST" && path === "/libraries") {
      if (server.failCreate) {
        throw new ShishoAPIError(
          "A library path doesn't exist.",
          "validation_error",
          422,
        );
      }
      return { id: 7, name: (body as { name: string }).name };
    }
    if (path === "/config") return {};
    if (path === "/getting-started") {
      return { enabled: true, dismissed: false, auto_open: false, steps: [] };
    }
    return { items: [], total: 0, entries: [] };
  });

// What the landing page sees of the Getting Started panel.
const Landing = () => {
  const { isOpen, pulse } = useGettingStartedPanel();
  return (
    <p>
      Library page, panel {isOpen ? "open" : "closed"}, pill{" "}
      {pulse ? "pulsing" : "still"}
    </p>
  );
};

// `retries` keeps TanStack Query's default retries, as the app has them.
const renderSetup = (path = "/setup/library", { retries = false } = {}) => {
  const router = createMemoryRouter(
    [
      { path: "/setup/:step", element: <SetupFlow /> },
      { path: "/libraries/:libraryId", element: <Landing /> },
      { path: "/", element: <p>The app</p> },
    ],
    { initialEntries: [path] },
  );
  const result = render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: retries
            ? {}
            : {
                queries: { retry: false },
                mutations: { retry: false },
              },
        })
      }
    >
      <GettingStartedPanelProvider>
        <RouterProvider router={router} />
      </GettingStartedPanelProvider>
    </QueryClientProvider>,
  );
  return { router, ...result };
};

const fillLibraryStep = async (
  user: ReturnType<typeof createUser>,
  { name = "Books", path = "/media/books" } = {},
) => {
  await user.type(screen.getByLabelText("Library name"), name);
  await user.type(screen.getByLabelText("Folder 1"), path);
};

const pluginCheckbox = (name: string) =>
  screen.getByRole("checkbox", { name: `Install ${name}` });

const storedDraft = () =>
  JSON.parse(window.localStorage.getItem("shisho:setup") ?? "null");

describe("Setup", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
    reducedMotion = false;
    window.matchMedia = vi.fn((query: string) => ({
      matches: query.includes("prefers-reduced-motion") && reducedMotion,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })) as unknown as typeof window.matchMedia;
    setAuth({
      permissions: ["libraries:write", "config:read", "config:write"],
      user: { id: 1, username: "admin" },
    });
  });

  afterEach(() => {
    window.localStorage.clear();
  });

  describe("Library step", () => {
    it("won't go on with a folder the server can't open", async () => {
      const server = newServer({
        detected: {
          ebooks: 0,
          audiobooks: 0,
          comics: 0,
          stopped_early: false,
          missing_paths: ["/media/typo"],
        },
      });
      stubAPI(server);
      const user = createUser();
      const { router } = renderSetup();
      await fillLibraryStep(user, { path: "/media/typo" });

      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();

      expect(router.state.location.pathname).toBe("/setup/library");
      const folder = screen.getByLabelText("Folder 1");
      expect(folder).toHaveAttribute("aria-invalid", "true");
      expect(folder).toHaveAccessibleDescription(
        /can't find or open this folder/i,
      );

      // Fixing the path clears the error.
      server.detected = { ...server.detected, ebooks: 3, missing_paths: [] };
      await user.clear(folder);
      await user.type(folder, "/media/books");
      expect(screen.queryByText(/can't find or open this folder/i)).toBeNull();
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();
      expect(router.state.location.pathname).toBe("/setup/plugins");
    });

    it("asks for a name and folders, with organizing on by default", async () => {
      stubAPI(newServer());
      renderSetup();

      expect(
        screen.getByRole("heading", {
          level: 1,
          name: /where are your books/i,
        }),
      ).toBeInTheDocument();
      expect(screen.getByLabelText("Library name")).toBeInTheDocument();
      expect(screen.getByLabelText("Folder 1")).toBeInTheDocument();
      expect(
        screen.getByRole("checkbox", {
          name: "Organize file structure during scans",
        }),
      ).toBeChecked();
      expect(
        screen.getByRole("listitem", { current: "step" }),
      ).toHaveTextContent("Library");
    });

    it("shows the organize animation, or a still before and after with reduced motion", async () => {
      stubAPI(newServer());
      const { unmount } = renderSetup();
      expect(
        screen.getByRole("img", { name: /files in a messy folder move/i }),
      ).toBeInTheDocument();
      expect(screen.queryByRole("figure", { name: "Before" })).toBeNull();
      unmount();

      reducedMotion = true;
      renderSetup();
      expect(
        screen.queryByRole("img", { name: /files in a messy folder move/i }),
      ).toBeNull();
      const before = screen.getByRole("figure", { name: "Before" });
      const after = screen.getByRole("figure", { name: "After" });
      expect(before).toHaveTextContent("pride_and_prejudice_FINAL.epub");
      expect(after).toHaveTextContent("[Jane Austen] Pride and Prejudice");
    });

    it("needs a name and a folder before it continues", async () => {
      stubAPI(newServer());
      const user = createUser();
      const { router } = renderSetup();

      await user.click(screen.getByRole("button", { name: "Continue" }));

      expect(router.state.location.pathname).toBe("/setup/library");
      expect(screen.getByText("Give your library a name.")).toBeInTheDocument();
      expect(
        screen.getByText("Choose at least one folder."),
      ).toBeInTheDocument();
    });

    it("looks through the folders in the background once they're chosen", async () => {
      const server = newServer();
      stubAPI(server);
      const user = createUser();
      renderSetup();

      await fillLibraryStep(user);
      await flush();

      const detections = server.calls.filter(
        (c) => c.path === "/filesystem/detect",
      );
      expect(detections[detections.length - 1]?.body).toEqual({
        paths: ["/media/books"],
      });
    });

    it("continues to the enrichment step", async () => {
      stubAPI(newServer());
      const user = createUser();
      const { router } = renderSetup();

      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));

      expect(router.state.location.pathname).toBe("/setup/plugins");
    });
  });

  describe("resuming after a reload", () => {
    it("keeps the library choices", async () => {
      stubAPI(newServer());
      const user = createUser();
      const { unmount } = renderSetup();
      await fillLibraryStep(user);
      await user.click(
        screen.getByRole("checkbox", {
          name: "Organize file structure during scans",
        }),
      );
      unmount();

      renderSetup("/setup/library");

      expect(screen.getByLabelText("Library name")).toHaveValue("Books");
      expect(screen.getByLabelText("Folder 1")).toHaveValue("/media/books");
      expect(
        screen.getByRole("checkbox", {
          name: "Organize file structure during scans",
        }),
      ).not.toBeChecked();
    });

    it("keeps the plugin ticks on the enrichment step", async () => {
      stubAPI(newServer());
      const user = createUser();
      const { unmount } = renderSetup();
      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();
      await user.click(pluginCheckbox("Open Library Enricher"));
      unmount();

      renderSetup("/setup/plugins");
      await flush();

      expect(pluginCheckbox("Goodreads Enricher")).toBeChecked();
      expect(pluginCheckbox("Open Library Enricher")).toBeChecked();
    });

    it("sends the enrichment step back to the library step without a library to make", async () => {
      stubAPI(newServer());
      const { router } = renderSetup("/setup/plugins");
      await flush();

      expect(router.state.location.pathname).toBe("/setup/library");
    });
  });

  it("Skip setup forgets the choices and goes into the app", async () => {
    stubAPI(newServer());
    const user = createUser();
    const { router } = renderSetup();
    await fillLibraryStep(user);
    expect(storedDraft()).not.toBeNull();

    await user.click(screen.getByRole("button", { name: "Skip setup" }));

    expect(router.state.location.pathname).toBe("/");
    expect(storedDraft()).toBeNull();
  });

  it("offers Skip setup on the enrichment step too", async () => {
    stubAPI(newServer());
    const user = createUser();
    const { router } = renderSetup();
    await fillLibraryStep(user);
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await flush();

    await user.click(screen.getByRole("button", { name: "Skip setup" }));

    expect(router.state.location.pathname).toBe("/");
  });

  describe("Enrichment step", () => {
    const openEnrichment = async (detected: DetectFileTypesResponse) => {
      const server = newServer({ detected });
      stubAPI(server);
      const user = createUser();
      const result = renderSetup();
      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();
      return { server, user, ...result };
    };

    it.each([
      {
        outcome: "ebooks",
        detected: { ebooks: 40, audiobooks: 0, comics: 0 },
        summary: "We found about 40 ebooks.",
        ticked: ["Goodreads Enricher"],
        unticked: ["Open Library Enricher"],
        hidden: ["Audible Enricher", "Manga Enricher"],
      },
      {
        outcome: "audiobooks",
        detected: { ebooks: 0, audiobooks: 3, comics: 0 },
        summary: "We found about 3 audiobooks.",
        ticked: ["Audible Enricher"],
        unticked: [],
        hidden: [
          "Goodreads Enricher",
          "Open Library Enricher",
          "Manga Enricher",
        ],
      },
      {
        outcome: "comics",
        detected: { ebooks: 0, audiobooks: 0, comics: 1 },
        summary: "We found about 1 comic.",
        ticked: ["Manga Enricher"],
        unticked: [],
        hidden: [
          "Goodreads Enricher",
          "Open Library Enricher",
          "Audible Enricher",
        ],
      },
      {
        outcome: "everything",
        detected: { ebooks: 1200, audiobooks: 35, comics: 2 },
        summary: "We found about 1,200 ebooks, 35 audiobooks, and 2 comics.",
        ticked: ["Goodreads Enricher", "Audible Enricher", "Manga Enricher"],
        unticked: ["Open Library Enricher"],
        hidden: [],
      },
      {
        outcome: "nothing",
        detected: { ebooks: 0, audiobooks: 0, comics: 0 },
        summary: "We didn't find any books in those folders yet",
        ticked: ["Goodreads Enricher"],
        unticked: [
          "Open Library Enricher",
          "Audible Enricher",
          "Manga Enricher",
        ],
        hidden: [],
      },
    ])(
      "preselects plugins for $outcome",
      async ({ detected, summary, ticked, unticked, hidden }) => {
        await openEnrichment({ ...detected, stopped_early: false });

        expect(screen.getByText(summary, { exact: false })).toBeInTheDocument();
        const recommended = screen.getByRole("group", {
          name: "Recommended for your library",
        });
        for (const name of ticked) {
          expect(
            within(recommended).getByRole("checkbox", {
              name: `Install ${name}`,
            }),
          ).toBeChecked();
        }
        for (const name of unticked) {
          expect(
            within(recommended).getByRole("checkbox", {
              name: `Install ${name}`,
            }),
          ).not.toBeChecked();
        }
        for (const name of hidden) {
          expect(
            within(recommended).queryByRole("checkbox", {
              name: `Install ${name}`,
            }),
          ).toBeNull();
        }
      },
    );

    it("says when it only looked at part of the folders", async () => {
      await openEnrichment({
        ebooks: 4990,
        audiobooks: 10,
        comics: 0,
        stopped_early: true,
      });

      expect(
        screen.getByText(
          "Your folders are large, so we looked through part of them and found about 4,990 ebooks and 10 audiobooks.",
        ),
      ).toBeInTheDocument();
    });

    it("lists every other official plugin, unticked, under See all plugins", async () => {
      const { user } = await openEnrichment({
        ebooks: 3,
        audiobooks: 0,
        comics: 0,
        stopped_early: false,
      });
      expect(
        screen.queryByRole("checkbox", { name: "Install Audible Enricher" }),
      ).toBeNull();

      await user.click(screen.getByRole("button", { name: "See all plugins" }));

      const others = screen.getByRole("group", { name: "All plugins" });
      expect(
        within(others).getByRole("checkbox", {
          name: "Install Audible Enricher",
        }),
      ).not.toBeChecked();
      expect(
        within(others).getByRole("checkbox", {
          name: "Install Manga Enricher",
        }),
      ).not.toBeChecked();
    });

    it("summarizes what a plugin can reach and expands to the full list", async () => {
      const { user } = await openEnrichment({
        ebooks: 3,
        audiobooks: 0,
        comics: 0,
        stopped_early: false,
      });
      const card = screen.getByRole("article", { name: "Goodreads Enricher" });
      expect(card).toHaveTextContent(
        "Connects to goodreads.com, i.gr-assets.com, and 1 more site",
      );

      await user.click(
        within(card).getByRole("button", { name: "What it can access" }),
      );

      expect(within(card).getByText("Network Access")).toBeInTheDocument();
      expect(within(card).getByText("Metadata Enrichment")).toBeInTheDocument();
      expect(
        within(card).getByText(
          "*.goodreads.com, i.gr-assets.com, m.media-amazon.com",
        ),
      ).toBeInTheDocument();
    });

    it("says at once when the repository can't be reached, and still finishes", async () => {
      const server = newServer({
        available: new ShishoAPIError("Bad gateway", "upstream_error", 502),
      });
      stubAPI(server);
      const user = createUser();
      // Retrying the repository would keep the admin waiting for seconds.
      const { router } = renderSetup("/setup/library", { retries: true });
      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();

      expect(
        screen.getByText(/couldn't load plugins from the plugin repository/i),
      ).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();

      expect(router.state.location.pathname).toBe("/libraries/7");
      expect(
        server.calls.filter(
          (c) => c.method === "POST" && c.path === "/plugins/installed",
        ),
      ).toHaveLength(0);
    });

    it("treats a repository with no official plugins as unreachable", async () => {
      stubAPI(newServer({ available: [] }));
      const user = createUser();
      renderSetup();
      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();

      expect(
        screen.getByText(/couldn't load plugins from the plugin repository/i),
      ).toBeInTheDocument();
    });

    it("can be skipped, creating the library with no plugins", async () => {
      const { server, user, router } = await openEnrichment({
        ebooks: 3,
        audiobooks: 0,
        comics: 0,
        stopped_early: false,
      });

      await user.click(screen.getByRole("button", { name: "Skip this step" }));
      await flush();

      expect(router.state.location.pathname).toBe("/libraries/7");
      expect(
        server.calls.filter(
          (c) => c.method === "POST" && c.path === "/plugins/installed",
        ),
      ).toHaveLength(0);
    });
  });

  it("waits for the folders it was given before recommending, however fast Continue is", async () => {
    stubAPI(newServer());
    const user = createUser();
    renderSetup();
    await fillLibraryStep(user);

    // No time passes for the typed folder to settle.
    await user.click(screen.getByRole("button", { name: "Continue" }));

    // The step shows the typed folder's result or waits for it, never the
    // fallback for folders it hasn't looked in.
    expect(
      screen.queryByText(/couldn't look through|didn't find any books/i),
    ).toBeNull();
    await flush();
    expect(screen.getByText("We found about 12 ebooks.")).toBeInTheDocument();
  });

  it("doesn't tick a recommended plugin this version of Shisho can't install", async () => {
    const goodreads = plugin(GOODREADS, "Goodreads Enricher");
    goodreads.versions = goodreads.versions.map((v) => ({
      ...v,
      compatible: false,
    }));
    const server = newServer({
      available: [goodreads, plugin(OPEN_LIBRARY, "Open Library Enricher")],
    });
    stubAPI(server);
    const user = createUser();
    renderSetup();
    await fillLibraryStep(user);
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await flush();

    expect(pluginCheckbox("Goodreads Enricher")).not.toBeChecked();
    expect(pluginCheckbox("Goodreads Enricher")).toBeDisabled();
  });

  describe("Finish", () => {
    it("installs the ticked plugins, then creates the library, and lands on it", async () => {
      const server = newServer({
        detected: { ebooks: 5, audiobooks: 2, comics: 0, stopped_early: false },
      });
      stubAPI(server);
      const user = createUser();
      const { router } = renderSetup();
      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();

      await user.click(screen.getByRole("button", { name: "Finish setup" }));
      await flush();

      const writes = server.calls.filter((c) => c.method === "POST");
      expect(
        writes
          .filter((c) => c.path !== "/filesystem/detect")
          .map((c) => [c.path, (c.body as { id?: string }).id]),
      ).toEqual([
        ["/plugins/installed", GOODREADS],
        ["/plugins/installed", AUDIBLE],
        ["/libraries", undefined],
      ]);
      expect(writes.find((c) => c.path === "/plugins/installed")?.body).toEqual(
        {
          scope: "shisho",
          id: GOODREADS,
          name: "Goodreads Enricher",
          version: "1.2.0",
        },
      );
      expect(writes.find((c) => c.path === "/libraries")?.body).toEqual({
        name: "Books",
        library_paths: ["/media/books"],
        organize_file_structure: true,
        cover_aspect_ratio: "book",
      });

      expect(router.state.location.pathname).toBe("/libraries/7");
      expect(
        screen.getByText("Library page, panel closed, pill pulsing"),
      ).toBeInTheDocument();
      expect(storedDraft()).toBeNull();
    });

    it("doesn't create the library until every install has finished", async () => {
      let release!: () => void;
      const server = newServer({
        detected: { ebooks: 5, audiobooks: 0, comics: 0, stopped_early: false },
        holdInstalls: new Promise<void>((resolve) => (release = resolve)),
      });
      stubAPI(server);
      const user = createUser();
      const { router } = renderSetup();
      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();

      await user.click(screen.getByRole("button", { name: "Finish setup" }));
      await flush();

      const progress = screen.getByRole("list", { name: "Setup progress" });
      expect(progress).toHaveTextContent(
        "Install Goodreads Enricher, in progress",
      );
      expect(progress).toHaveTextContent("Create your library, waiting");
      expect(server.calls.some((c) => c.path === "/libraries")).toBe(false);

      release();
      await flush();
      expect(server.calls.some((c) => c.path === "/libraries")).toBe(true);
      expect(router.state.location.pathname).toBe("/libraries/7");
    });

    it("keeps the choices and offers Try again when the library can't be created", async () => {
      const server = newServer({ failCreate: true });
      stubAPI(server);
      const user = createUser();
      const { router } = renderSetup();
      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();
      await user.click(screen.getByRole("button", { name: "Skip this step" }));
      await flush();

      expect(screen.getByRole("alert")).toHaveTextContent(
        "A library path doesn't exist.",
      );
      expect(router.state.location.pathname).toBe("/setup/plugins");
      expect(storedDraft()).not.toBeNull();

      server.failCreate = false;
      await user.click(screen.getByRole("button", { name: "Try again" }));
      await flush();
      expect(router.state.location.pathname).toBe("/libraries/7");
    });

    it("reports a failed install and still creates the library", async () => {
      const server = newServer({
        detected: { ebooks: 5, audiobooks: 2, comics: 0, stopped_early: false },
        failInstall: GOODREADS,
      });
      stubAPI(server);
      const user = createUser();
      const { router } = renderSetup();
      await fillLibraryStep(user);
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await flush();

      await user.click(screen.getByRole("button", { name: "Finish setup" }));
      await flush();

      expect(toast.error).toHaveBeenCalledWith(
        "Download failed.",
        expect.anything(),
      );
      expect(
        server.calls.filter(
          (c) => c.method === "POST" && c.path === "/plugins/installed",
        ),
      ).toHaveLength(2);
      expect(router.state.location.pathname).toBe("/libraries/7");
    });

    it("finishes from the library step for a role that can't install plugins", async () => {
      setAuth({
        permissions: ["libraries:write"],
        user: { id: 1, username: "librarian" },
      });
      const server = newServer();
      stubAPI(server);
      const user = createUser();
      const { router } = renderSetup();
      await fillLibraryStep(user);

      await user.click(screen.getByRole("button", { name: "Finish setup" }));
      await waitFor(() =>
        expect(router.state.location.pathname).toBe("/libraries/7"),
      );
      expect(server.calls.some((c) => c.path.startsWith("/plugins"))).toBe(
        false,
      );
    });
  });
});
