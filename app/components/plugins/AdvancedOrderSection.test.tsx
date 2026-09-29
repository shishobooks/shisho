import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import { AdvancedOrderSection } from "./AdvancedOrderSection";

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    demoMode: false,
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
  }),
}));

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
    auth.permissions = new Set(["config:read"]);
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
    auth.permissions = new Set(["config:read", "books:read"]);
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderSection();

    expect(await screen.findByText("Hook Type")).toBeInTheDocument();
    expect(request.mock.calls.map((call) => call[1])).toContain(
      "/plugins/order/metadataEnricher",
    );
  });
});
