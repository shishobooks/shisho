import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";

import { useEpubBlob } from "./epub";

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return React.createElement(QueryClientProvider, { client }, children);
};

describe("useEpubBlob", () => {
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

    const { result } = renderHook(() => useEpubBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/books/files/42/download",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(result.current.data).toBeInstanceOf(Blob);
  });

  it("surfaces fetch errors", async () => {
    fetchSpy.mockResolvedValue(new Response("nope", { status: 500 }));

    const { result } = renderHook(() => useEpubBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(ShishoAPIError);
    expect(result.current.error?.status).toBe(500);
    expect(result.current.error?.code).toBe("epub_download_failed");
  });
});
