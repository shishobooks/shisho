import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import LibraryRedirect from "./LibraryRedirect";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/library/TopNav", () => ({
  default: () => <div data-testid="top-nav" />,
}));

const LibraryPage = () => {
  const { libraryId } = useParams();
  return <div>library page {libraryId}</div>;
};

const renderRedirect = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route element={<LibraryRedirect />} path="/" />
          <Route element={<LibraryPage />} path="/libraries/:libraryId" />
          <Route element={<div>lists page</div>} path="/lists" />
          <Route
            element={<div>library settings page</div>}
            path="/admin/libraries"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const requestedPaths = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.map((call: unknown[]) => call[1]);

// Answers the two requests the redirect makes: the user's libraries and
// their settings. A settings value that is an Error rejects that request.
const mockRequests = (
  libraries: { id: number; name: string }[],
  settings: { default_library_id: number | null } | Error = {
    default_library_id: null,
  },
) =>
  vi.spyOn(API, "request").mockImplementation((_method, path) => {
    if (path === "/user/libraries") return Promise.resolve(libraries);
    if (path === "/settings/user") {
      return settings instanceof Error
        ? Promise.reject(settings)
        : Promise.resolve(settings);
    }
    return Promise.reject(new Error(`unexpected request ${path}`));
  });

describe("LibraryRedirect", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["books:read"] });
  });

  it("opens the first accessible library for a role with Books Read, without Libraries Read", async () => {
    const request = mockRequests([{ id: 4, name: "Fiction" }]);

    renderRedirect();

    expect(await screen.findByText("library page 4")).toBeInTheDocument();
    expect(requestedPaths(request)).toEqual(
      expect.arrayContaining(["/user/libraries", "/settings/user"]),
    );
  });

  it("opens the default library from user settings", async () => {
    mockRequests(
      [
        { id: 4, name: "Fiction" },
        { id: 7, name: "Comics" },
      ],
      { default_library_id: 7 },
    );

    renderRedirect();

    expect(await screen.findByText("library page 7")).toBeInTheDocument();
  });

  it("opens the first library when the default is no longer accessible", async () => {
    mockRequests([{ id: 4, name: "Fiction" }], { default_library_id: 7 });

    renderRedirect();

    expect(await screen.findByText("library page 4")).toBeInTheDocument();
  });

  it("opens the first library when user settings fail to load", async () => {
    mockRequests([{ id: 4, name: "Fiction" }], new Error("boom"));

    renderRedirect();

    expect(await screen.findByText("library page 4")).toBeInTheDocument();
  });

  it("sends a role with Libraries Read and no libraries to library settings", async () => {
    setAuth({ permissions: ["books:read", "libraries:read"] });
    mockRequests([]);

    renderRedirect();

    expect(
      await screen.findByText("library settings page"),
    ).toBeInTheDocument();
  });

  it("sends a role without Libraries Read and no libraries to lists", async () => {
    mockRequests([]);

    renderRedirect();

    expect(await screen.findByText("lists page")).toBeInTheDocument();
  });

  it("sends a role without Books Read to lists without requesting libraries", async () => {
    setAuth({ permissions: ["shares:read", "shares:write", "libraries:read"] });
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderRedirect();

    expect(await screen.findByText("lists page")).toBeInTheDocument();
    expect(requestedPaths(request)).toHaveLength(0);
  });
});
