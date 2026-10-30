import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { toast, Toaster } from "sonner";
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
import { setAuth } from "@/testing/auth";

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

const respond = async (method: string, path: string) => {
  if (method === "POST" && path === "/lists/5/copy-to-to-read") {
    return { added: 2 };
  }
  if (path === "/lists/5") {
    return {
      id: 5,
      user_id: 2,
      name: "Shared Queue",
      description: "",
      is_ordered: true,
      permission: "viewer",
      book_count: 3,
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
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("ListDetail Copy to To Read", () => {
  beforeEach(() => {
    setAuth({ permissions: ["books:read"] });
  });
  afterEach(() => {
    toast.dismiss();
    vi.restoreAllMocks();
  });

  it("copies even a list the user can only view, and says how many it added", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(respond as typeof API.request);
    renderList();

    await userEvent
      .setup({ advanceTimers: vi.advanceTimersByTime })
      .click(await screen.findByRole("button", { name: "Copy to To Read" }));

    expect(request).toHaveBeenCalledWith("POST", "/lists/5/copy-to-to-read");
    expect(
      await screen.findByText("Added 2 books to To Read"),
    ).toBeInTheDocument();
  });

  it("renders disabled in Demo Mode, with the demo's explanation", async () => {
    setAuth({ permissions: ["books:read"], demoMode: true });
    vi.spyOn(API, "request").mockImplementation(respond as typeof API.request);
    renderList();

    expect(
      await screen.findByRole("button", { name: "Copy to To Read" }),
    ).toBeDisabled();
    expect(
      screen.getByText("This action is unavailable in the demo."),
    ).toBeInTheDocument();
  });
});
