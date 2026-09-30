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

describe("AdvancedOrderSection save", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("enables Save Order only while the order differs from the saved one", async () => {
    setAuth({ permissions: ["config:read", "config:write", "books:read"] });
    vi.spyOn(API, "request").mockImplementation(async (_method, path) =>
      path === "/plugins/order/metadataEnricher"
        ? [
            { scope: "shisho", plugin_id: "first", mode: "enabled" },
            { scope: "shisho", plugin_id: "second", mode: "enabled" },
          ]
        : [],
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderSection();

    const save = await screen.findByRole("button", { name: "Save Order" });
    expect(save).toBeDisabled();

    const [firstMode] = screen.getAllByRole("combobox", {
      name: "When this plugin runs",
    });
    await user.click(firstMode);
    await user.click(screen.getByRole("option", { name: /^Never/ }));
    expect(save).toBeEnabled();

    // Setting the mode back matches the saved order again.
    await user.click(firstMode);
    await user.click(
      screen.getByRole("option", { name: /^For every new file/ }),
    );
    expect(save).toBeDisabled();
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
