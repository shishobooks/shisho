import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import { queryClient } from "@/libraries/query-client";
import { RangeFile } from "@/libraries/rangeFile";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";
import { createRangeServer } from "@/testing/rangeServer";

import { useReflowableBook, type ReflowableBook } from "./reflowable";

// Query hooks check the role's permissions; this test grants them all.
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

setAuth({ permissions: ALL_PERMISSIONS });

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return React.createElement(QueryClientProvider, { client }, children);
};

const DOWNLOAD = "/api/books/files/42/download";
const ORIGINAL = "/api/books/files/42/download/original";
const book = new Uint8Array(4 * 1024 * 1024).fill(7);

const apiError = (status: number, code: string, message: string) =>
  Response.json({ error: { code, message } }, { status });

describe("useReflowableBook", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("reads the Generated File by part rather than downloading it whole", async () => {
    const server = createRangeServer(book);
    fetchSpy.mockImplementation(server.fetch);

    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBeInstanceOf(RangeFile);
    expect((result.current.data as RangeFile).size).toBe(book.length);
    expect(server.requests[0].url).toBe(DOWNLOAD);
    expect(server.bytesSent()).toBeLessThan(book.length / 4);
  });

  it.each([
    [
      422,
      "invalid_state",
      "Generated downloads are not supported for mobi files",
    ],
    [500, "internal", "Internal server error"],
  ])(
    "reads the original by part when the server answers %i for the Generated File",
    async (status, code, message) => {
      const original = createRangeServer(book);
      fetchSpy.mockImplementation(
        async (url: RequestInfo | URL, init?: RequestInit) =>
          url === DOWNLOAD
            ? apiError(status, code, message)
            : original.fetch(url, init),
      );

      const { result } = renderHook(() => useReflowableBook(42), {
        wrapper,
      });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toBeInstanceOf(RangeFile);
      expect((result.current.data as RangeFile).url).toBe(ORIGINAL);
    },
  );

  it("surfaces the API's error when the original fails too", async () => {
    fetchSpy.mockImplementation(async () =>
      apiError(404, "not_found", "File not found"),
    );

    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(ShishoAPIError);
    expect(result.current.error?.status).toBe(404);
    expect(result.current.error?.code).toBe("not_found");
    expect(result.current.error?.message).toBe("File not found");
  });

  it("surfaces the original's error, not the Generated File's", async () => {
    fetchSpy.mockImplementation(async (url: RequestInfo | URL) =>
      url === DOWNLOAD
        ? apiError(422, "invalid_state", "Not supported")
        : apiError(404, "not_found", "File not found"),
    );

    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error?.status).toBe(404);
  });

  it("reports a proxy's error page by its status", async () => {
    fetchSpy.mockImplementation(
      async () =>
        new Response("<html>Bad gateway</html>", {
          status: 502,
          statusText: "Bad Gateway",
        }),
    );

    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error?.message).toBe(
      "Request failed with status 502 (Bad Gateway)",
    );
  });

  it("doesn't download the original when the connection drops", async () => {
    fetchSpy.mockImplementation(async () => {
      throw new TypeError("Failed to fetch");
    });

    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it("does not fall back in Demo Mode, which denies the original", async () => {
    setAuth({ permissions: ALL_PERMISSIONS, demoMode: true });
    try {
      fetchSpy.mockImplementation(async () =>
        apiError(500, "internal", "Internal server error"),
      );

      const { result } = renderHook(() => useReflowableBook(42), {
        wrapper,
      });
      await waitFor(() => expect(result.current.isError).toBe(true));

      expect(fetchSpy).toHaveBeenCalledTimes(1);
      expect(result.current.error?.status).toBe(500);
    } finally {
      setAuth({ permissions: ALL_PERMISSIONS });
    }
  });

  it("keeps the book it loaded when the window regains focus", async () => {
    fetchSpy.mockImplementation(createRangeServer(book).fetch);
    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const loaded = result.current.data;
    const asked = fetchSpy.mock.calls.length;

    // Long after the book opened, the reader switches away and back. A new
    // book would make the reader close the book and open it again.
    vi.setSystemTime(Date.now() + 60 * 60 * 1000);
    try {
      act(() => {
        focusManager.setFocused(false);
        focusManager.setFocused(true);
      });
      await new Promise((resolve) => setTimeout(resolve, 0));

      expect(fetchSpy).toHaveBeenCalledTimes(asked);
      expect(result.current.isFetching).toBe(false);
      expect(result.current.data).toBe(loaded);
    } finally {
      focusManager.setFocused(undefined);
    }
  });

  it("hands a reopened reader one book, not a cached one and then a fresh one", async () => {
    fetchSpy.mockImplementation(createRangeServer(book).fetch);
    // The app's defaults, which refetch every query a reader mounts.
    const client = new QueryClient({
      defaultOptions: queryClient.getDefaultOptions(),
    });
    const appWrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client }, children);
    try {
      const first = renderHook(() => useReflowableBook(42), {
        wrapper: appWrapper,
      });
      await waitFor(() => expect(first.result.current.isSuccess).toBe(true));
      first.unmount();
      // Reopening comes later, by navigation, after the cache has collected.
      await new Promise((resolve) => setTimeout(resolve, 0));

      // Each book the reader is handed opens it again.
      const handed = new Set<ReflowableBook>();
      const second = renderHook(
        () => {
          const query = useReflowableBook(42);
          if (query.data) handed.add(query.data);
          return query;
        },
        { wrapper: appWrapper },
      );
      await waitFor(() => expect(second.result.current.isSuccess).toBe(true));
      await waitFor(() => expect(second.result.current.isFetching).toBe(false));

      expect(handed.size).toBe(1);
    } finally {
      client.clear();
    }
  });
});

// A server or proxy that answers Range with the whole file sends the reader
// to download the book whole, as it did before reading by part.
describe("useReflowableBook from a server that ignores Range", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  const whole = (body: BodyInit) =>
    new Response(body, { status: 200, headers: { "Accept-Ranges": "bytes" } });
  const asksForPart = (init?: RequestInit) =>
    new Headers(init?.headers).has("Range");

  it("downloads the book whole", async () => {
    fetchSpy.mockImplementation(async () => whole("book-bytes"));

    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBeInstanceOf(Blob);
    expect(await (result.current.data as Blob).text()).toBe("book-bytes");
  });

  it("loads the original file when the Generated File's download fails", async () => {
    fetchSpy.mockImplementation(
      async (url: RequestInfo | URL, init?: RequestInit) => {
        if (url !== DOWNLOAD) return whole("original-bytes");
        if (asksForPart(init)) return whole("generated-bytes");
        throw new TypeError("Failed to fetch");
      },
    );

    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(await (result.current.data as Blob).text()).toBe("original-bytes");
  });

  it("loads the original file when the Generated File arrives truncated", async () => {
    fetchSpy.mockImplementation(
      async (url: RequestInfo | URL, init?: RequestInit) => {
        if (url !== DOWNLOAD) return whole("original-bytes");
        if (asksForPart(init)) return whole("generated-bytes");
        // A body that errors partway, as a cut connection does.
        const body = new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(new TextEncoder().encode("first bytes"));
            controller.error(new TypeError("network error"));
          },
        });
        return whole(body);
      },
    );

    const { result } = renderHook(() => useReflowableBook(42), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(fetchSpy).toHaveBeenLastCalledWith(
      ORIGINAL,
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(await (result.current.data as Blob).text()).toBe("original-bytes");
  });

  it("does not fall back in Demo Mode, which denies the original", async () => {
    setAuth({ permissions: ALL_PERMISSIONS, demoMode: true });
    try {
      fetchSpy.mockImplementation(
        async (_url: RequestInfo | URL, init?: RequestInit) =>
          asksForPart(init)
            ? whole("generated-bytes")
            : apiError(500, "internal", "Internal server error"),
      );

      const { result } = renderHook(() => useReflowableBook(42), {
        wrapper,
      });
      await waitFor(() => expect(result.current.isError).toBe(true));

      expect(result.current.error?.status).toBe(500);
      expect(fetchSpy).not.toHaveBeenCalledWith(ORIGINAL, expect.anything());
    } finally {
      setAuth({ permissions: ALL_PERMISSIONS });
    }
  });
});
