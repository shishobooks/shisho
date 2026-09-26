import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";

import { useUploadFileCover } from "./books";

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return React.createElement(QueryClientProvider, { client }, children);
};

const cover = () =>
  new File(["cover-bytes"], "cover.jpg", { type: "image/jpeg" });

describe("useUploadFileCover", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("resolves to the updated file on success", async () => {
    fetchSpy.mockResolvedValue(Response.json({ id: 7 }));

    const { result } = renderHook(() => useUploadFileCover(), { wrapper });

    await expect(
      result.current.mutateAsync({ id: 7, file: cover() }),
    ).resolves.toEqual({ id: 7 });
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/books/files/7/cover",
      expect.objectContaining({ method: "POST", body: expect.any(FormData) }),
    );
  });

  it("rejects a proxy's non-JSON 413 page with a status-based API error", async () => {
    fetchSpy.mockResolvedValue(
      new Response("<html><body>413 Request Entity Too Large</body></html>", {
        status: 413,
        statusText: "Payload Too Large",
        headers: { "content-type": "text/html" },
      }),
    );

    const { result } = renderHook(() => useUploadFileCover(), { wrapper });
    const error = await result.current
      .mutateAsync({ id: 7, file: cover() })
      .catch((caught: unknown) => caught);

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Request failed with status 413 (Payload Too Large)",
      code: undefined,
      status: 413,
    });
  });

  it("rejects a Shisho JSON error with its message and code", async () => {
    fetchSpy.mockResolvedValue(
      Response.json(
        {
          error: {
            code: "invalid_cover",
            message: "Cover must be an image",
            status_code: 422,
          },
        },
        { status: 422 },
      ),
    );

    const { result } = renderHook(() => useUploadFileCover(), { wrapper });
    const error = await result.current
      .mutateAsync({ id: 7, file: cover() })
      .catch((caught: unknown) => caught);

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect(error).toMatchObject({
      message: "Cover must be an image",
      code: "invalid_cover",
      status: 422,
    });
  });
});
