import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";

import { useReflowableBlob } from "./reflowable";

// Query hooks check the role's permissions; this test grants them all.
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

setAuth({ permissions: ALL_PERMISSIONS });

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return React.createElement(QueryClientProvider, { client }, children);
};

describe("useReflowableBlob", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("fetches the EPUB from the download endpoint and resolves to a Blob", async () => {
    const blob = new Blob(["epub-bytes"], { type: "application/epub+zip" });
    fetchSpy.mockResolvedValue(
      new Response(blob, {
        status: 200,
        headers: { "Content-Type": "application/epub+zip" },
      }),
    );

    const { result } = renderHook(() => useReflowableBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/books/files/42/download",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(result.current.data).toBeInstanceOf(Blob);
  });

  it("surfaces the API's error", async () => {
    fetchSpy.mockResolvedValue(
      Response.json(
        { error: { code: "not_found", message: "File not found" } },
        { status: 404 },
      ),
    );

    const { result } = renderHook(() => useReflowableBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(ShishoAPIError);
    expect(result.current.error?.status).toBe(404);
    expect(result.current.error?.code).toBe("not_found");
    expect(result.current.error?.message).toBe("File not found");
  });

  it("reports a proxy's error page by its status", async () => {
    fetchSpy.mockResolvedValue(
      new Response("<html>Bad gateway</html>", {
        status: 502,
        statusText: "Bad Gateway",
      }),
    );

    const { result } = renderHook(() => useReflowableBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error?.message).toBe(
      "Request failed with status 502 (Bad Gateway)",
    );
  });

  it("loads the original file when no generated download is available", async () => {
    fetchSpy.mockImplementation(async (url: RequestInfo | URL) =>
      url === "/api/books/files/42/download"
        ? Response.json(
            {
              error: {
                code: "invalid_state",
                message: "Generated downloads are not supported for mobi files",
              },
            },
            { status: 422 },
          )
        : new Response("mobi-bytes", { status: 200 }),
    );

    const { result } = renderHook(() => useReflowableBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(fetchSpy).toHaveBeenLastCalledWith(
      "/api/books/files/42/download/original",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(await result.current.data?.text()).toBe("mobi-bytes");
  });

  it("surfaces an error from the original file too", async () => {
    fetchSpy.mockImplementation(async (url: RequestInfo | URL) =>
      url === "/api/books/files/42/download"
        ? Response.json(
            { error: { code: "invalid_state", message: "Not supported" } },
            { status: 422 },
          )
        : Response.json(
            { error: { code: "not_found", message: "File not found" } },
            { status: 404 },
          ),
    );

    const { result } = renderHook(() => useReflowableBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error?.status).toBe(404);
  });

  it("surfaces any other 422 instead of loading the original", async () => {
    fetchSpy.mockResolvedValue(
      Response.json(
        { error: { code: "validation_error", message: "Bad request" } },
        { status: 422 },
      ),
    );

    const { result } = renderHook(() => useReflowableBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(fetchSpy).toHaveBeenCalledTimes(1);
    expect(result.current.error?.code).toBe("validation_error");
  });
});
