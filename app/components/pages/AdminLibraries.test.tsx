import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { Permission } from "@/utils/permissions";

import AdminLibraries from "./AdminLibraries";

const navigate = vi.hoisted(() => vi.fn());

vi.mock("react-router-dom", async () => ({
  ...(await vi.importActual<typeof import("react-router-dom")>(
    "react-router-dom",
  )),
  useNavigate: () => navigate,
}));

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const renderPage = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <AdminLibraries />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const requestedPaths = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.map((call: unknown[]) => call[1]);

describe("AdminLibraries", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    navigate.mockReset();
  });

  it("does not request the server config for a role without Config Read", async () => {
    setAuth({ permissions: ["libraries:read"] });
    const request = vi.spyOn(API, "request").mockResolvedValue({
      items: [{ id: 1, name: "Fiction", library_paths: [] }],
      total: 1,
    });

    renderPage();

    expect(await screen.findByText("Fiction")).toBeInTheDocument();
    expect(requestedPaths(request)).toContain("/libraries");
    expect(requestedPaths(request)).not.toContain("/config");
  });

  it("requests the server config for a role with Config Read", async () => {
    setAuth({ permissions: ["libraries:read", "config:read"] });
    const request = vi.spyOn(API, "request").mockResolvedValue({
      items: [],
      total: 0,
    });

    renderPage();

    await waitFor(() => {
      expect(requestedPaths(request)).toContain("/config");
    });
  });

  describe("links a role cannot follow", () => {
    const fiction = {
      items: [{ id: 1, name: "Fiction", library_paths: [] }],
      total: 1,
    };

    it("shows the Settings button only with Libraries Read and Write", async () => {
      setAuth({ permissions: ["libraries:read", "books:read"] });
      vi.spyOn(API, "request").mockResolvedValue(fiction);
      const { unmount } = renderPage();

      expect(await screen.findByText("Fiction")).toBeInTheDocument();
      expect(screen.queryByRole("link", { name: "Settings" })).toBeNull();
      unmount();

      setAuth({
        permissions: ["libraries:read", "libraries:write", "books:read"],
      });
      renderPage();

      expect(
        await screen.findByRole("link", { name: "Settings" }),
      ).toHaveAttribute("href", "/libraries/1/settings");
    });

    it("links the library name only with Books Read", async () => {
      setAuth({ permissions: ["libraries:read"] });
      vi.spyOn(API, "request").mockResolvedValue(fiction);
      const { unmount } = renderPage();

      expect(await screen.findByText("Fiction")).toBeInTheDocument();
      expect(screen.getByText("Fiction").closest("a")).toBeNull();
      unmount();

      setAuth({ permissions: ["libraries:read", "books:read"] });
      renderPage();

      expect((await screen.findByText("Fiction")).closest("a")).toHaveAttribute(
        "href",
        "/libraries/1",
      );
    });

    it.each<[Permission[], boolean]>([
      [["libraries:read", "libraries:write", "config:read"], false],
      [
        ["libraries:read", "libraries:write", "config:read", "books:read"],
        true,
      ],
    ])(
      "after creating the default library with %j, opens it: %s",
      async (permissions, opensLibrary) => {
        setAuth({ permissions });
        vi.spyOn(API, "request").mockImplementation(async (method, path) => {
          if (method === "POST" && path === "/libraries")
            return { id: 9, name: "Main" };
          if (path === "/config") return { dev_library_path: "/media" };
          return { items: [], total: 0 };
        });
        const user = userEvent.setup();
        renderPage();

        await user.click(
          await screen.findByRole("button", {
            name: /Create default library/,
          }),
        );

        await waitFor(() =>
          expect(API.request).toHaveBeenCalledWith(
            "POST",
            "/libraries",
            expect.anything(),
            null,
          ),
        );

        await waitFor(() =>
          expect(screen.getByText("Libraries")).toBeVisible(),
        );
        if (opensLibrary) {
          await waitFor(() =>
            expect(navigate).toHaveBeenCalledWith("/libraries/9"),
          );
        } else {
          // The create handler refetches the list before it would navigate.
          await waitFor(() =>
            expect(
              vi
                .mocked(API.request)
                .mock.calls.filter(
                  (call) => call[0] === "GET" && call[1] === "/libraries",
                ),
            ).toHaveLength(2),
          );
          await new Promise((resolve) => setTimeout(resolve, 50));
          expect(navigate).not.toHaveBeenCalled();
        }
      },
    );
  });
});
