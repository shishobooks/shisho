import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API, markErrorDisplayed, ShishoAPIError } from "./api";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    getToasts: vi.fn(() => []),
  },
}));

const demoModeResponse = () =>
  Response.json(
    {
      error: {
        code: "demo_mode",
        message: "This action is unavailable in the demo.",
        status_code: 403,
      },
    },
    { status: 403 },
  );

const rejection = (response: Response) =>
  API.checkStatus(response).then(
    () => {
      throw new Error("expected checkStatus to reject");
    },
    (caught: unknown) => caught,
  );

const visibleDemoToast = {
  id: 1,
  title: "This action is unavailable in the demo.",
  type: "error" as const,
};

describe("ShishoAPI Demo Mode errors", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(toast.getToasts).mockReturnValue([]);
  });
  it("shows the Demo Mode toast once and still rejects with the API error", async () => {
    const error = await rejection(demoModeResponse());

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      code: "demo_mode",
      message: "This action is unavailable in the demo.",
      status: 403,
    });
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(toast.error).toHaveBeenCalledWith(
      "This action is unavailable in the demo.",
      { id: "demo-mode" },
    );
  });

  it("does not add a fallback when the caller already showed the message", async () => {
    vi.mocked(toast.getToasts).mockReturnValue([visibleDemoToast]);

    await API.checkStatus(demoModeResponse()).catch(() => undefined);
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).not.toHaveBeenCalled();
  });

  it("does not add a fallback when the caller showed the message with a prefix", async () => {
    vi.mocked(toast.getToasts).mockReturnValue([
      {
        ...visibleDemoToast,
        title: "Failed to save: This action is unavailable in the demo.",
      },
    ]);

    await API.checkStatus(demoModeResponse()).catch(() => undefined);
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).not.toHaveBeenCalled();
  });

  it("does not add a fallback when the caller displays the error inline", async () => {
    await API.checkStatus(demoModeResponse()).catch(markErrorDisplayed);
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).not.toHaveBeenCalled();
  });

  it("coalesces concurrent Demo Mode fallback toasts", async () => {
    vi.mocked(toast.getToasts).mockImplementation(() =>
      vi.mocked(toast.error).mock.calls.length > 0 ? [visibleDemoToast] : [],
    );

    await Promise.all([
      API.checkStatus(demoModeResponse()).catch(() => undefined),
      API.checkStatus(demoModeResponse()).catch(() => undefined),
    ]);
    await vi.runOnlyPendingTimersAsync();

    expect(toast.error).toHaveBeenCalledTimes(1);
  });
});

describe("ShishoAPI error responses", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(toast.getToasts).mockReturnValue([]);
  });

  it("rejects a 504 HTML proxy page with a status-based API error", async () => {
    const error = await rejection(
      new Response("<html><body><h1>504 Gateway Time-out</h1></body></html>", {
        status: 504,
        statusText: "Gateway Timeout",
        headers: { "content-type": "text/html" },
      }),
    );

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Request failed with status 504 (Gateway Timeout)",
      code: undefined,
      status: 504,
    });
  });

  it("rejects a 502 plain-text body without a content-type", async () => {
    const response = new Response(new TextEncoder().encode("Bad Gateway"), {
      status: 502,
      statusText: "Bad Gateway",
    });
    expect(response.headers.get("content-type")).toBeNull();

    const error = await rejection(response);

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Request failed with status 502 (Bad Gateway)",
      code: undefined,
      status: 502,
    });
  });

  it("rejects a 502 with an empty body and no content-type", async () => {
    const error = await rejection(
      new Response(null, { status: 502, statusText: "Bad Gateway" }),
    );

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Request failed with status 502 (Bad Gateway)",
      code: undefined,
      status: 502,
    });
  });

  it("omits the reason phrase when the response has no status text", async () => {
    const error = await rejection(
      new Response("upstream request timeout", { status: 504 }),
    );

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Request failed with status 504",
      code: undefined,
      status: 504,
    });
  });

  it("maps a Shisho JSON error to its message and code", async () => {
    const error = await rejection(
      Response.json(
        {
          error: {
            code: "not_found",
            message: "Book not found",
            status_code: 404,
          },
        },
        { status: 404 },
      ),
    );

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Book not found",
      code: "not_found",
      status: 404,
    });
  });

  it("rejects a JSON error body that is not a Shisho error with a status-based API error", async () => {
    const error = await rejection(
      Response.json(
        { message: "Not Found" },
        { status: 404, statusText: "Not Found" },
      ),
    );

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Request failed with status 404 (Not Found)",
      code: undefined,
      status: 404,
    });
  });

  it("rejects a 200 with a non-JSON body instead of returning it", async () => {
    const error = await rejection(
      new Response("<!doctype html><html></html>", {
        status: 200,
        headers: { "content-type": "text/html" },
      }),
    );

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Received a non-JSON response with status 200",
      code: undefined,
      status: 200,
    });
  });

  it("does not show the Demo Mode toast for a non-JSON 403", async () => {
    const error = await rejection(
      new Response("Forbidden", {
        status: 403,
        statusText: "Forbidden",
        headers: { "content-type": "text/plain" },
      }),
    );
    await vi.runOnlyPendingTimersAsync();

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({ code: undefined, status: 403 });
    expect(toast.error).not.toHaveBeenCalled();
  });
});

describe("ShishoAPI successful responses", () => {
  it("returns the parsed body of a 2xx JSON response", async () => {
    await expect(
      API.checkStatus(Response.json({ id: 1, name: "Book" })),
    ).resolves.toEqual({ id: 1, name: "Book" });
  });

  it("returns undefined for a 204 No Content response", async () => {
    await expect(
      API.checkStatus(new Response(null, { status: 204 })),
    ).resolves.toBeUndefined();
  });

  it("returns undefined for a 2xx response with a whitespace-only body", async () => {
    await expect(
      API.checkStatus(new Response("\n", { status: 200 })),
    ).resolves.toBeUndefined();
  });

  it("returns undefined for a 2xx response with an empty body", async () => {
    await expect(
      API.checkStatus(new Response(null, { status: 200 })),
    ).resolves.toBeUndefined();
  });
});
