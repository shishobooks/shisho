import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { API } from "@/libraries/api";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";
import type { Book } from "@/types";

import { IdentifyBookDialog } from "./IdentifyBookDialog";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

const book = {
  id: 1,
  library_id: 1,
  title: "Dune",
  filepath: "/library/dune",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  files: [],
} as unknown as Book;

const renderDialog = (open: boolean) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <IdentifyBookDialog book={book} onOpenChange={vi.fn()} open={open} />
    </QueryClientProvider>,
  );
};

describe("IdentifyBookDialog requests", () => {
  beforeEach(() => setAuth({ permissions: ALL_PERMISSIONS }));

  afterEach(() => vi.restoreAllMocks());

  it("sends nothing while closed", async () => {
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderDialog(false);
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(request).not.toHaveBeenCalled();
  });

  // The enricher list now loads on open, alongside the automatic search, so
  // an empty search can land first.
  it("waits for the enricher list before saying none are installed", async () => {
    let resolveOrder!: (order: unknown[]) => void;
    vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
      if (path === "/plugins/order/metadataEnricher") {
        return new Promise((resolve) => (resolveOrder = resolve));
      }
      if (path === "/plugins/search") return { results: [], total_plugins: 0 };
      return [];
    });

    renderDialog(true);
    await waitFor(() => expect(resolveOrder).toBeDefined());
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    expect(screen.queryByText(/No metadata enricher plugins/)).toBeNull();

    await act(async () => resolveOrder([]));

    expect(
      await screen.findByText(/No metadata enricher plugins/),
    ).toBeInTheDocument();
  });

  it("loads the identifier types and enrichers once open", async () => {
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderDialog(true);

    await waitFor(() => {
      const paths = request.mock.calls.map((call) => call[1]);
      expect(paths).toContain("/plugins/identifier-types");
      expect(paths).toContain("/plugins/order/metadataEnricher");
    });
  });
});
