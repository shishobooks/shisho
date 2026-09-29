import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { anyOf, type Permission, type Requirement } from "@/utils/permissions";

// Every query hook in this directory must gate its request on the permission
// its backend route requires, inside the hook (see "Query hooks gate their own
// permissions" in app/AGENTS.md). QUERY_HOOKS records each hook's expected
// requirement and arguments that enable it. The test renders every exported
// query hook and checks that:
//
// 1. It sends a request with exactly its required permissions (each
//    alternative alone for an anyOf requirement). This also proves the
//    arguments enable it, so the other checks are not vacuous.
// 2. It sends nothing with every permission except a required one (except all
//    the alternatives for anyOf), so a gate on the wrong permission fails.
// 3. With no permissions, it requests only authenticated-only paths.
// 4. In Demo Mode with every permission, it requests no route the server does
//    not register in Demo Mode.
//
// A new query hook fails until it is added to QUERY_HOOKS, and then until it
// gates itself on the recorded requirement. Mutation hooks are recognized by
// their `mutate` function and skipped, since they send nothing on render.

const auth = vi.hoisted(() => ({
  permissions: new Set<string>(),
  demoMode: false,
}));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    user: {
      id: 1,
      username: "reader",
      permissions: [...auth.permissions],
      library_access: null,
    },
    isAuthenticated: true,
    isLoading: false,
    demoMode: auth.demoMode,
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
    canWrite: (resource: string) => auth.permissions.has(`${resource}:write`),
    hasLibraryAccess: () => true,
  }),
}));

const ALL_PERMISSIONS = [
  "libraries",
  "books",
  "series",
  "people",
  "users",
  "jobs",
  "config",
  "shares",
].flatMap((resource) => [`${resource}:read`, `${resource}:write`]);

// Paths any signed-in user may request, so a hook may call them without a
// permission. /jobs/:id serves a Jobs Read role or the creator of a
// bulk-download job, so it checks no role permission. /share/:token is the
// anonymous Share Link recipient route.
const AUTHENTICATED_ONLY: RegExp[] = [
  /^\/jobs\/\d+$/,
  /^\/auth\//,
  /^\/settings\/user$/,
  /^\/lists$/,
  /^\/lists\/templates$/,
  /^\/lists\/\d+$/,
  /^\/lists\/\d+\/shares$/,
  /^\/user\/api-keys$/,
  /^\/user\/libraries$/,
  /^\/users\/directory$/,
  /^\/settings\/libraries\/\d+$/,
  /^\/events$/,
  /^\/share\/[^/]+$/,
];

// Paths the server does not register in Demo Mode (pkg/server/server.go)
// that signed-in pages call. The anonymous /share/:token route is also absent
// there, but only the recipient page calls it, and that page shows its
// unavailable state on the 404.
const DEMO_UNREGISTERED: RegExp[] = [
  /^\/plugins\//,
  /^\/libraries\/[^/]+\/plugins\//,
  /^\/users\/directory$/,
];

// Each query hook's expected requirement (the backend route's permission,
// from pkg/server/server.go and the route files) and arguments that enable it
// when the requirement is met. Hooks that take an `enabled` option get
// `enabled: true`, which must not bypass the permission check.
interface HookCase {
  requires: Requirement | null;
  args: unknown[];
}
const requires = (requirement: Requirement, args: unknown[]): HookCase => ({
  requires: requirement,
  args,
});
// The hook's routes are on the authenticated-only allowlist.
const authenticated = (args: unknown[]): HookCase => ({
  requires: null,
  args,
});
const on = { enabled: true };
const QUERY_HOOKS: Record<string, HookCase> = {
  // apiKeys
  useApiKeys: authenticated([]),
  // audnexus
  useAudnexusChapters: requires("books:write", ["B000000000", on]),
  // auth
  useAuthStatus: authenticated([on]),
  // books
  useBook: requires("books:read", ["1", on]),
  useBooks: requires("books:read", [{}, on]),
  useBooksByIds: requires("books:read", [[1, 2]]),
  // cache
  useCaches: requires("config:read", []),
  // chapters
  useFileChapters: requires("books:read", [1, on]),
  // config
  useConfig: requires("config:read", []),
  // entity-search
  usePeopleSearch: requires("people:read", [1, true, "a"]),
  useAuthorSearch: requires("people:read", [1, true, "a"]),
  useSeriesSearch: requires("series:read", [1, true, "a"]),
  usePublisherSearch: requires("books:read", [1, true, "a"]),
  useParentPublisherSearch: requires("books:read", [1, [], true, "a"]),
  useGenreSearch: requires("books:read", [1, true, "a"]),
  useTagSearch: requires("books:read", [1, true, "a"]),
  useGenreItemCounts: requires("books:read", [1, ["Fantasy"]]),
  useTagItemCounts: requires("books:read", [1, ["Favorite"]]),
  // epub
  useEpubBlob: requires("books:read", [1, on]),
  // filesystem
  useFilesystemBrowse: requires("libraries:write", [{}, on]),
  // genres
  useGenresList: requires("books:read", [{}, on]),
  useGenre: requires("books:read", [1, on]),
  useGenreBooks: requires("books:read", [1, {}, on]),
  // jobs
  useJob: authenticated(["1", on]),
  useJobs: requires("jobs:read", [{}, on]),
  useLatestScanJob: requires("jobs:read", [1]),
  useJobLogs: requires("jobs:read", ["1", {}, on]),
  // libraries
  useLibrary: requires("libraries:read", ["1", on]),
  useLibraries: requires(anyOf("libraries:read", "users:write"), [{}, on]),
  useLibraryLanguages: requires("books:read", [1, on]),
  useUserLibraries: authenticated([on]),
  useUserLibrary: authenticated(["1"]),
  // librarySettings
  useLibrarySettings: authenticated([1, on]),
  // lists
  useListLists: authenticated([{}, on]),
  useList: authenticated([1, on]),
  useListBooks: requires("books:read", [1, {}, on]),
  useListShares: authenticated([1, on]),
  useListTemplates: authenticated([on]),
  useBookLists: requires("books:read", [1, on]),
  // logs
  useLogs: requires("config:read", [{}, on]),
  // people
  usePeopleList: requires("people:read", [{}, on]),
  usePerson: requires("people:read", [1, on]),
  usePersonAuthoredBooks: requires("people:read", [1, {}, on]),
  usePersonNarratedFiles: requires("people:read", [1, {}, on]),
  // plugins
  usePluginsInstalled: requires("config:read", []),
  usePluginsAvailable: requires("config:read", []),
  usePluginOrder: requires("books:read", ["metadataEnricher"]),
  useAllPluginOrders: requires("books:read", [["metadataEnricher"]]),
  usePluginConfig: requires("config:read", ["local", "sample"]),
  usePluginManifest: requires("config:read", ["local", "sample", on]),
  usePluginRepositories: requires("config:read", []),
  usePluginIdentifierTypes: requires("books:read", [on]),
  useLibraryPluginOrder: requires("libraries:read", ["1", "metadataEnricher"]),
  usePluginSearch: requires("books:write", [{ query: "dune", bookId: 1 }]),
  // publishers
  usePublishersList: requires("books:read", [{}, on]),
  usePublisher: requires("books:read", [1, on]),
  usePublisherFiles: requires("books:read", [1, {}, on]),
  // review
  useReviewCriteria: requires(anyOf("books:read", "config:read"), []),
  // search
  useGlobalSearch: requires("books:read", [{ q: "dune", library_id: 1 }, on]),
  // series
  useSeriesList: requires("series:read", [{}, on]),
  useSeries: requires("series:read", [1, on]),
  useSeriesBooks: requires("series:read", [1, {}, on]),
  // settings
  useUserSettings: authenticated([on]),
  // sharing
  useSharingSettings: requires(
    anyOf("shares:read", "shares:write", "config:read"),
    [on],
  ),
  useBookShareLinks: requires(anyOf("shares:read", "shares:write"), [1, on]),
  useSharedBook: authenticated(["token"]),
  // tags
  useTagsList: requires("books:read", [{}, on]),
  useTag: requires("books:read", [1, on]),
  useTagBooks: requires("books:read", [1, {}, on]),
  // users
  useUser: requires("users:read", ["1", on]),
  useUsers: requires("users:read", [{}, on]),
  useRoles: requires("users:read", [on]),
  useUserDirectory: authenticated([on]),
};
type Hook = (...args: unknown[]) => unknown;

const modules = import.meta.glob<Record<string, unknown>>(
  ["./**/*.{ts,tsx}", "!./**/*.test.{ts,tsx}", "!./permissions.ts"],
  { eager: true },
);

const hooks = new Map<string, Hook>();
for (const mod of Object.values(modules)) {
  for (const [name, value] of Object.entries(mod)) {
    if (/^use[A-Z]/.test(name) && typeof value === "function") {
      hooks.set(name, value as Hook);
    }
  }
}

const renderInClient = (hook: Hook, args: unknown[]) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return renderHook(() => hook(...args), { wrapper });
};

// Paths the hook requested, without the /api prefix that raw fetch calls
// carry.
const requestedPaths = async (hook: Hook, args: unknown[]) => {
  const request = vi
    .spyOn(API, "request")
    .mockResolvedValue({ items: [], total: 0, chapters: [] });
  const fetchSpy = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(new Blob()));

  renderInClient(hook, args);
  // Let queries start: TanStack fetches after the observer subscribes.
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

  const paths = [
    ...request.mock.calls.map((call) => String(call[1])),
    ...fetchSpy.mock.calls.map((call) =>
      String(call[0])
        .split("?")[0]
        .replace(/^\/api(?=\/)/, ""),
    ),
  ];
  cleanup();
  request.mockRestore();
  fetchSpy.mockRestore();
  return paths;
};

// A query hook that needs arguments may throw when rendered without them;
// that still means it is not a mutation hook.
const isMutationHook = (hook: Hook) => {
  try {
    const { result } = renderInClient(hook, []);
    const value = result.current as { mutate?: unknown } | null;
    return typeof value?.mutate === "function";
  } catch {
    return false;
  } finally {
    cleanup();
  }
};

// Permission sets that should each let the hook send its request: none for an
// authenticated-only hook, every listed permission for an all-of requirement,
// and each alternative alone for anyOf.
const sufficientSets = (requirement: Requirement | null): Permission[][] => {
  if (requirement === null) return [[]];
  if (typeof requirement === "string") return [[requirement]];
  if ("anyOf" in requirement) return requirement.anyOf.map((p) => [p]);
  return [[...(requirement as readonly Permission[])]];
};

// Permissions to withhold from a role that holds everything else, where each
// set should leave the hook silent: each required permission in turn for an
// all-of requirement, and every alternative at once for anyOf.
const withheldSets = (requirement: Requirement): Permission[][] => {
  if (typeof requirement === "string") return [[requirement]];
  if ("anyOf" in requirement) return [[...requirement.anyOf]];
  return (requirement as readonly Permission[]).map((p) => [p]);
};

const hookNamed = (name: string) => {
  const hook = hooks.get(name);
  expect(hook, `${name} is not exported`).toBeDefined();
  return hook as Hook;
};

const cases = Object.entries(QUERY_HOOKS);
const gatedCases = cases.filter(([, { requires: r }]) => r !== null);

describe("query hook permission gating", () => {
  beforeEach(() => {
    auth.permissions = new Set();
    auth.demoMode = false;
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("lists every exported query hook", () => {
    const missing = [...hooks.entries()]
      .filter(([name]) => !(name in QUERY_HOOKS))
      .filter(([, hook]) => !isMutationHook(hook))
      .map(([name]) => name);
    expect(missing, "add these hooks to QUERY_HOOKS").toEqual([]);

    const unknown = Object.keys(QUERY_HOOKS).filter((name) => !hooks.has(name));
    expect(unknown, "QUERY_HOOKS names hooks that do not exist").toEqual([]);
  });

  it.each(cases)(
    "%s requests its route with exactly its required permissions",
    async (name, { requires: requirement, args }) => {
      for (const permissions of sufficientSets(requirement)) {
        auth.permissions = new Set(permissions);

        const paths = await requestedPaths(hookNamed(name), args);

        expect(
          paths.length,
          `${name} sent no request with [${permissions.join(", ")}]`,
        ).toBeGreaterThan(0);
      }
    },
  );

  it.each(gatedCases)(
    "%s sends nothing without its required permission",
    async (name, { requires: requirement, args }) => {
      for (const withheld of withheldSets(requirement as Requirement)) {
        auth.permissions = new Set(
          ALL_PERMISSIONS.filter((p) => !withheld.includes(p as Permission)),
        );

        const paths = await requestedPaths(hookNamed(name), args);

        expect(
          paths,
          `${name} sent a request without [${withheld.join(", ")}]`,
        ).toEqual([]);
      }
    },
  );

  it.each(cases)(
    "%s sends only authenticated-only requests without permissions",
    async (name, { args }) => {
      const paths = await requestedPaths(hookNamed(name), args);

      const gated = paths.filter(
        (path) => !AUTHENTICATED_ONLY.some((pattern) => pattern.test(path)),
      );
      expect(gated, `${name} requested a permission-bound route`).toEqual([]);
    },
  );

  it.each(cases)(
    "%s sends no request to a route Demo Mode does not register",
    async (name, { args }) => {
      auth.permissions = new Set(ALL_PERMISSIONS);
      auth.demoMode = true;

      const paths = await requestedPaths(hookNamed(name), args);

      const unregistered = paths.filter((path) =>
        DEMO_UNREGISTERED.some((pattern) => pattern.test(path)),
      );
      expect(unregistered, `${name} requested a Demo Mode 404`).toEqual([]);
    },
  );
});
