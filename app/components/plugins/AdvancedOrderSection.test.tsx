import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
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
