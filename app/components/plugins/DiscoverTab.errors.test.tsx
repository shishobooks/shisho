import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import { DiscoverTab } from "./DiscoverTab";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

beforeEach(() => {
  vi.restoreAllMocks();
  setAuth({ permissions: ["config:read", "config:write"] });
});

describe("DiscoverTab load failure", () => {
  it("shows the fallback for a server fault and fetches again on Retry", async () => {
    let calls = 0;
    vi.spyOn(API, "request").mockImplementation(async (_method, route) => {
      if (route === "/plugins/available") {
        calls += 1;
        if (calls === 1)
          throw new ShishoAPIError(
            "Internal Server Error",
            "internal_server_error",
            500,
          );
      }
      return [];
    });
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <MemoryRouter>
          <DiscoverTab canWrite />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /^Failed to load available plugins/,
    );
    expect(screen.queryByText(/Internal Server Error/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => expect(calls).toBe(2));
    await waitFor(() =>
      expect(screen.queryByRole("alert")).not.toBeInTheDocument(),
    );
  });
});
