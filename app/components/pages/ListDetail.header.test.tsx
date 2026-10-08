import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import { emulatePhoneWidth } from "@/testing/phoneWidth";

import ListDetail from "./ListDetail";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
  window.matchMedia = vi.fn(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/library/TopNav", () => ({ default: () => null }));

const respond = async (_method: string, path: string) => {
  if (path === "/lists/5") {
    return {
      id: 5,
      user_id: 1,
      name: "Queue",
      description: "",
      is_ordered: false,
      permission: "owner",
      book_count: 0,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    };
  }
  if (path === "/settings/user") return { gallery_size: "m" };
  return { items: [], total: 0 };
};

const renderList = () =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={["/lists/5"]}>
        <Routes>
          <Route element={<ListDetail />} path="/lists/:id" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("ListDetail header on a phone", () => {
  emulatePhoneWidth();

  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["books:read"] });
    vi.spyOn(API, "request").mockImplementation(respond as typeof API.request);
  });

  it("names the owner's icon-only actions", async () => {
    renderList();

    expect(
      await screen.findByRole("heading", { level: 1, name: "Queue" }),
    ).toBeInTheDocument();
    for (const name of ["Edit", "Share", "Delete"]) {
      const button = screen.getByRole("button", { name });
      expect(button).toBeVisible();
      expect(within(button).getByText(name)).not.toBeVisible();
    }
  });
});
