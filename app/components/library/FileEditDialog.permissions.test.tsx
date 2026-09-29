import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render } from "@testing-library/react";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { FileRoleMain, FileTypeM4B, type File } from "@/types";

import { FileEditDialog } from "./FileEditDialog";

// Uses the real query hooks, so the People gating inside them is what keeps
// the narrator combobox quiet for a role without People Read.

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    demoMode: false,
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
    canWrite: (resource: string) => auth.permissions.has(`${resource}:write`),
  }),
}));

const file = {
  id: 1,
  book_id: 1,
  library_id: 1,
  filepath: "/test/file.m4b",
  display_name: "file.m4b",
  file_type: FileTypeM4B,
  file_role: FileRoleMain,
  filesize_bytes: 1000,
  created_at: "2024-01-01",
  updated_at: "2024-01-01",
  narrators: [],
  identifiers: [],
  is_preferred_cover: false,
} as unknown as File;

const renderDialog = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <FileEditDialog file={file} onOpenChange={vi.fn()} open={true} />
    </QueryClientProvider>,
  );
};

const requestedPaths = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.map((call: unknown[]) => call[1]);

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 250));
  });

describe("FileEditDialog narrator suggestions", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests no people for a role without People Read", async () => {
    auth.permissions = new Set(["books:read", "books:write"]);
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) =>
        path === "/plugins/identifier-types" || path.endsWith("/languages")
          ? []
          : { items: [], total: 0 },
      );

    renderDialog();
    await flush();

    expect(requestedPaths(request)).not.toContain("/people");
  });

  it("requests people for a role with People Read", async () => {
    auth.permissions = new Set(["books:read", "books:write", "people:read"]);
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) =>
        path === "/plugins/identifier-types" || path.endsWith("/languages")
          ? []
          : { items: [], total: 0 },
      );

    renderDialog();
    await flush();

    expect(requestedPaths(request)).toContain("/people");
  });
});
