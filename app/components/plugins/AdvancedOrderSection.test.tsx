import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import { AdvancedOrderSection } from "./AdvancedOrderSection";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const renderSection = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AdvancedOrderSection />
    </QueryClientProvider>,
  );
};

describe("AdvancedOrderSection", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("explains that the order needs Books Read for a role with only Config Read", async () => {
    setAuth({ permissions: ["config:read"] });
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderSection();

    expect(
      await screen.findByText(/Viewing the plugin order requires Books Read/),
    ).toBeInTheDocument();
    expect(
      request.mock.calls.filter((call) =>
        String(call[1]).startsWith("/plugins/order/"),
      ),
    ).toHaveLength(0);
  });

  it("loads the order for a role with Books Read", async () => {
    setAuth({ permissions: ["config:read", "books:read"] });
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderSection();

    expect(await screen.findByText("Hook Type")).toBeInTheDocument();
    expect(request.mock.calls.map((call) => call[1])).toContain(
      "/plugins/order/metadataEnricher",
    );
  });
});

describe("AdvancedOrderSection load failure", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("shows the fallback for a server fault and retries on request", async () => {
    setAuth({ permissions: ["config:read", "books:read"] });
    let orderCalls = 0;
    vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
      if (path === "/plugins/order/metadataEnricher") {
        orderCalls += 1;
        if (orderCalls === 1)
          throw new ShishoAPIError(
            "Internal Server Error",
            "internal_server_error",
            500,
          );
      }
      return [];
    });

    renderSection();

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/^Failed to load plugin order/);
    expect(screen.queryByText(/Internal Server Error/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("Hook Type")).toBeInTheDocument();
    expect(orderCalls).toBe(2);
  });
});
