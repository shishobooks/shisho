import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  API,
  firstFailedQuery,
  isDemoModeError,
  isLoadFailure,
  isNotFoundError,
  requestErrorMessage,
  ShishoAPIError,
  toastRequestError,
} from "./api";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
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

describe("ShishoAPI Demo Mode errors", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the Demo Mode toast before rejecting with the API error", async () => {
    const error = await rejection(demoModeResponse());

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      code: "demo_mode",
      message: "This action is unavailable in the demo.",
      status: 403,
    });
    expect(isDemoModeError(error)).toBe(true);
    // Reported synchronously, so callers never need to wait for it.
    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(toast.error).toHaveBeenCalledWith(
      "This action is unavailable in the demo.",
      { id: "demo-mode" },
    );
  });

  it("reuses one toast id for concurrent rejections", async () => {
    await Promise.all([
      API.checkStatus(demoModeResponse()).catch(() => undefined),
      API.checkStatus(demoModeResponse()).catch(() => undefined),
    ]);

    // sonner replaces a toast that shares an id, so both calls render one toast.
    expect(toast.error).toHaveBeenCalledTimes(2);
    for (const call of vi.mocked(toast.error).mock.calls) {
      expect(call).toEqual([
        "This action is unavailable in the demo.",
        { id: "demo-mode" },
      ]);
    }
  });

  it("does not treat other 403s as Demo Mode rejections", async () => {
    const error = await rejection(
      Response.json(
        { error: { code: "forbidden", message: "Nope" } },
        { status: 403 },
      ),
    );

    expect(isDemoModeError(error)).toBe(false);
    expect(toast.error).not.toHaveBeenCalled();
  });
});

describe("requestErrorMessage", () => {
  it("prefers the error's message and falls back otherwise", () => {
    expect(
      requestErrorMessage(new ShishoAPIError("Gone", "not_found", 404), "x"),
    ).toBe("Gone");
    expect(requestErrorMessage(undefined, "Fallback")).toBe("Fallback");
    expect(
      requestErrorMessage(new ShishoAPIError("", "internal", 500), "Fallback"),
    ).toBe("Fallback");
  });

  it("falls back when the API error carries only a status", () => {
    // checkStatus builds this when a proxy answers instead of the server, so
    // it says nothing about which action failed.
    expect(
      requestErrorMessage(
        new ShishoAPIError(
          "Request failed with status 502 (Bad Gateway)",
          undefined,
          502,
        ),
        "Failed to delete book",
      ),
    ).toBe("Failed to delete book");
  });

  it("falls back for the server's generic internal error", () => {
    // An unhandled server fault renders as this code with a message that
    // does not say which action failed.
    expect(
      requestErrorMessage(
        new ShishoAPIError(
          "Internal Server Error",
          "internal_server_error",
          500,
        ),
        "Failed to delete book",
      ),
    ).toBe("Failed to delete book");
  });

  it("falls back for errors that did not come from the API", () => {
    // A network failure or a bug surfaces as a plain Error whose text means
    // nothing to the user.
    expect(
      requestErrorMessage(new TypeError("Failed to fetch"), "Fallback"),
    ).toBe("Fallback");
  });
});

describe("isNotFoundError", () => {
  it("is true only for an API 404", () => {
    expect(
      isNotFoundError(new ShishoAPIError("Book not found", "not_found", 404)),
    ).toBe(true);
    // A proxy's 404 page has no Shisho body but is still a 404.
    expect(
      isNotFoundError(
        new ShishoAPIError("Request failed with status 404", undefined, 404),
      ),
    ).toBe(true);
    expect(
      isNotFoundError(
        new ShishoAPIError(
          "Internal Server Error",
          "internal_server_error",
          500,
        ),
      ),
    ).toBe(false);
    expect(isNotFoundError(new TypeError("Failed to fetch"))).toBe(false);
    expect(isNotFoundError(null)).toBe(false);
  });
});

describe("query failure helpers", () => {
  const serverFault = new ShishoAPIError(
    "Internal Server Error",
    "internal_server_error",
    500,
  );
  const notFound = new ShishoAPIError("Book not found", "not_found", 404);

  it("isLoadFailure is true only for a non-404 error with no data", () => {
    expect(isLoadFailure({ data: undefined, error: serverFault })).toBe(true);
    expect(isLoadFailure({ data: undefined, error: notFound })).toBe(false);
    // A failed background refetch keeps its data on screen.
    expect(isLoadFailure({ data: { id: 1 }, error: serverFault })).toBe(false);
    // A query waiting on its enabled gate has neither.
    expect(isLoadFailure({ data: undefined, error: null })).toBe(false);
  });

  it("firstFailedQuery returns the first query that failed without data", () => {
    const loaded = { data: [1], error: serverFault };
    const failed = { data: undefined, error: notFound };
    const idle = { data: undefined, error: null };

    expect(firstFailedQuery(idle, loaded, failed)).toBe(failed);
    expect(firstFailedQuery(idle, loaded)).toBeUndefined();
  });
});

describe("toastRequestError", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the error's own message when it has one", () => {
    toastRequestError(
      new ShishoAPIError("Name already taken", "conflict", 409),
      "Failed to create list",
    );

    expect(toast.error).toHaveBeenCalledWith("Name already taken", undefined);
  });

  it("falls back to the caller's message for a non-Error rejection", () => {
    toastRequestError("nope", "Failed to create list");

    expect(toast.error).toHaveBeenCalledWith(
      "Failed to create list",
      undefined,
    );
  });

  it("falls back to the caller's message for an API error with a blank message", () => {
    toastRequestError(
      new ShishoAPIError("  ", "internal", 500),
      "Failed to create list",
    );

    expect(toast.error).toHaveBeenCalledWith(
      "Failed to create list",
      undefined,
    );
  });

  it("stays silent for a Demo Mode rejection", () => {
    toastRequestError(
      new ShishoAPIError(
        "This action is unavailable in the demo.",
        "demo_mode",
        403,
      ),
      "Failed to save",
    );

    expect(toast.error).not.toHaveBeenCalled();
  });
});

describe("ShishoAPI error responses", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
