import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";
import type { Book } from "@/types";
import type { Permission } from "@/utils/permissions";

import BookDetailBody, { type ShareLinkContext } from "./BookDetailBody";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

beforeAll(() => {
  vi.stubGlobal("__APP_VERSION__", "test");
});

const SHARING_SETTINGS = "/settings/sharing";
const DELIVERY_RULES = "/user/delivery-rules";

// An unreviewed file and an author, so the review badge and every Related
// tab would ask for their data if they were allowed to.
const book = {
  id: 7,
  library_id: 1,
  title: "Test Book",
  filepath: "",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  authors: [{ id: 1, person_id: 10, person: { id: 10, name: "Ada Author" } }],
  files: [
    {
      id: 42,
      book_id: 7,
      library_id: 1,
      file_type: "epub",
      file_role: "main",
      filepath: "",
      filesize_bytes: 1,
      reviewed: false,
      reachable_file_types: ["epub"],
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    },
  ],
} as unknown as Book;

const shareLink: ShareLinkContext = {
  downloadUrl: (file) => `/api/share/tok/files/${file.id}/download`,
  downloadAsUrl: (file, fileType) =>
    `/api/share/tok/files/${file.id}/download/${fileType}`,
  bookCoverUrl: () => "/api/share/tok/cover",
  fileCoverUrl: (file) => `/api/share/tok/files/${file.id}/cover`,
};

// Renders the body with the real query hooks and returns the paths it asked
// for once its queries have started.
const requestedPaths = async (
  props: Partial<React.ComponentProps<typeof BookDetailBody>> = {},
) => {
  const request = vi
    .spyOn(API, "request")
    .mockImplementation(async (_method, path) =>
      path === SHARING_SETTINGS
        ? { enabled: true, require_expiration: false }
        : [],
    );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <BookDetailBody book={book} {...props} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  return request.mock.calls.map((call) => call[1]);
};

describe("BookDetailBody requests", () => {
  afterEach(() => vi.restoreAllMocks());

  it("sends no authenticated request in Share Link context", async () => {
    setAuth({ permissions: ALL_PERMISSIONS });

    const paths = await requestedPaths({ shareLink });

    // Not one: the recipient's page reads only its public share endpoint.
    expect(paths).toEqual([]);
  });

  it("loads the Related panel's tabs and the review criteria for Books Read", async () => {
    setAuth({ permissions: ["books:read"] });

    const paths = await requestedPaths();

    expect(paths).toEqual(
      expect.arrayContaining([
        "/books",
        "/books/7/lists",
        "/books/7/devices",
        "/settings/review-criteria",
      ]),
    );
  });

  it("loads the user's default Delivery Rules with no permission", async () => {
    setAuth({ permissions: ["books:read"] });

    expect(await requestedPaths()).toContain(DELIVERY_RULES);
  });

  it("skips the sharing settings without a shares permission or Config Read", async () => {
    setAuth({ permissions: ["books:read", "books:write"] });

    expect(await requestedPaths()).not.toContain(SHARING_SETTINGS);
  });

  it.each<Permission>(["shares:read", "shares:write", "config:read"])(
    "loads the sharing settings with %s",
    async (permission) => {
      setAuth({ permissions: ["books:read", permission] });

      expect(await requestedPaths()).toContain(SHARING_SETTINGS);
    },
  );
});
