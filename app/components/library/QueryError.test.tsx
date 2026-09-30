import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";

import QueryError from "./QueryError";

const FALLBACK = "Failed to load plugins";

const failedQuery = (
  error: unknown,
  overrides: Partial<{ isFetching: boolean; isEnabled: boolean }> = {},
) => ({
  error,
  isFetching: false,
  isEnabled: true,
  refetch: vi.fn(),
  ...overrides,
});

describe("QueryError", () => {
  it("shows the server's message in an alert", () => {
    render(
      <QueryError
        fallback={FALLBACK}
        query={failedQuery(new ShishoAPIError("boom", "x", 400))}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("boom");
    expect(screen.queryByText(FALLBACK)).not.toBeInTheDocument();
  });

  it.each([
    [
      "the server's generic internal error",
      new ShishoAPIError("Internal Server Error", "internal_server_error", 500),
    ],
    ["a network failure", new TypeError("Failed to fetch")],
    [
      "a status-only proxy error",
      new ShishoAPIError(
        "Request failed with status 502 (Bad Gateway)",
        undefined,
        502,
      ),
    ],
  ])("shows the fallback for %s", (_, error) => {
    render(<QueryError fallback={FALLBACK} query={failedQuery(error)} />);

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent(FALLBACK);
    expect(alert).not.toHaveTextContent(String(error.message));
  });

  it("refetches when Retry is clicked", async () => {
    const query = failedQuery(new TypeError("Failed to fetch"));
    render(<QueryError fallback={FALLBACK} query={query} />);

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(query.refetch).toHaveBeenCalledTimes(1);
  });

  it("offers no Retry for a disabled query", () => {
    // refetch() runs even a disabled query, which would bypass its gate, so
    // there is nothing for Retry to do.
    const query = failedQuery(new TypeError("Failed to fetch"), {
      isEnabled: false,
    });
    render(<QueryError fallback={FALLBACK} query={query} />);

    expect(screen.getByRole("alert")).toHaveTextContent(FALLBACK);
    expect(
      screen.queryByRole("button", { name: "Retry" }),
    ).not.toBeInTheDocument();
    expect(query.refetch).not.toHaveBeenCalled();
  });

  it("disables Retry while the query is fetching", () => {
    render(
      <QueryError
        fallback={FALLBACK}
        query={failedQuery(new TypeError("Failed to fetch"), {
          isFetching: true,
        })}
      />,
    );

    expect(screen.getByRole("button", { name: "Retry" })).toBeDisabled();
  });
});
