import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import LibraryListPicker from "./LibraryListPicker";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const stubRequests = () =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/user/libraries") return [{ id: 1, name: "Fiction" }];
    return { items: [], total: 0 };
  });

const renderPicker = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1/books/7"]}>
        <Routes>
          <Route
            element={<LibraryListPicker />}
            path="/libraries/:libraryId/books/:id"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("LibraryListPicker", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));

  afterEach(() => vi.restoreAllMocks());

  it("shows the current library for a role with Books Read, without Libraries Read", async () => {
    stubRequests();
    renderPicker();

    expect(
      await screen.findByRole("button", { name: /Fiction/ }),
    ).toBeInTheDocument();
  });

  it("renders nothing and requests no libraries for a role without Books Read", async () => {
    setAuth({ permissions: ["shares:read"] });
    const request = stubRequests();
    const { container } = renderPicker();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(container).toBeEmptyDOMElement();
    expect(request.mock.calls.map((call) => call[1])).not.toContain(
      "/user/libraries",
    );
  });
});
