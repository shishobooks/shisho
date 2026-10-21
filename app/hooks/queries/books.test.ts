import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";

import { useUpdateFile, useUploadFileCover } from "./books";
import { QueryKey as GenresQueryKey } from "./genres";
import { QueryKey as SeriesQueryKey } from "./series";
import { QueryKey as TagsQueryKey } from "./tags";

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

describe("useUpdateFile", () => {
  let requestSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    requestSpy = vi.spyOn(API, "request").mockResolvedValue({ id: 7 });
  });

  afterEach(() => {
    requestSpy.mockRestore();
  });

  const invalidatedKeys = async (payload: object) => {
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => useUpdateFile(), {
      wrapper: ({ children }) =>
        React.createElement(QueryClientProvider, { client }, children),
    });
    await result.current.mutateAsync({ id: 7, payload });
    return invalidate.mock.calls.map(([filters]) => filters?.queryKey?.[0]);
  };

  // Promotion scans the File, which can add series, genres, and tags.
  it("refreshes series, genres, and tags when a file becomes a main file", async () => {
    const keys = await invalidatedKeys({ file_role: "main" });
    expect(keys).toEqual(
      expect.arrayContaining([
        SeriesQueryKey.ListSeries,
        GenresQueryKey.ListGenres,
        TagsQueryKey.ListTags,
      ]),
    );
  });

  it("leaves series, genres, and tags alone for other edits", async () => {
    const keys = await invalidatedKeys({ name: "Edition" });
    expect(keys).not.toContain(SeriesQueryKey.ListSeries);
    expect(keys).not.toContain(GenresQueryKey.ListGenres);
    expect(keys).not.toContain(TagsQueryKey.ListTags);
  });
});
