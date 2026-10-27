import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { UpdateLibraryPayload } from "@/types/generated/libraries";

import LibrarySettings from "./LibrarySettings";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/library/LibraryPluginsTab", () => ({
  default: () => null,
}));

const library = {
  id: 1,
  name: "Doujin",
  organize_file_structure: true,
  cover_aspect_ratio: "book",
  library_paths: [{ id: 1, filepath: "/books" }],
  tag_namespaces: ["Circle", "Source"],
};

let saved: UpdateLibraryPayload | undefined;

const renderSettings = () => {
  saved = undefined;
  vi.spyOn(API, "request").mockImplementation(async (method, path, payload) => {
    if (method === "POST" && path === "/libraries/1") {
      saved = payload as UpdateLibraryPayload;
      return { ...library, ...saved };
    }
    if (path === "/libraries/1") return library;
    return {};
  });
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={["/libraries/1/settings"]}>
        <Routes>
          <Route
            element={<LibrarySettings />}
            path="/libraries/:libraryId/settings"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

beforeEach(() => {
  vi.restoreAllMocks();
  setAuth({ permissions: ["books:read", "libraries:read", "libraries:write"] });
});

describe("LibrarySettings tag namespaces", () => {
  it("saves the namespaces edited, reordered, trimmed, and without blanks", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderSettings();

    expect(await screen.findByLabelText("Tag namespace 1")).toHaveValue(
      "Circle",
    );
    expect(screen.getByLabelText("Tag namespace 2")).toHaveValue("Source");

    await user.click(screen.getByRole("button", { name: "Move Source up" }));
    await user.click(screen.getByRole("button", { name: "Add Namespace" }));
    await user.type(screen.getByLabelText("Tag namespace 3"), " Magazine ");
    await user.click(screen.getByRole("button", { name: "Add Namespace" }));
    await user.click(screen.getByRole("button", { name: "Save Settings" }));

    await waitFor(() => expect(saved).toBeDefined());
    expect(saved?.tag_namespaces).toEqual(["Source", "Circle", "Magazine"]);
  });

  it("saves an empty list when every namespace is removed", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderSettings();

    await user.click(
      await screen.findByRole("button", { name: "Remove Circle" }),
    );
    await user.click(screen.getByRole("button", { name: "Remove Source" }));
    await user.click(screen.getByRole("button", { name: "Save Settings" }));

    await waitFor(() => expect(saved).toBeDefined());
    expect(saved?.tag_namespaces).toEqual([]);
  });
});
