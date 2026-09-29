import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { Permission } from "@/utils/permissions";

import { ShareLinkDialog } from "./ShareLinkDialog";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const LINKS_PATH = "/books/7/share-links";

const renderDialog = (open: boolean) => {
  const request = vi.spyOn(API, "request").mockResolvedValue([]);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <ShareLinkDialog
          bookId={7}
          bookTitle="Test Book"
          canList
          canManageSharing={false}
          canWrite
          onOpenChange={() => {}}
          open={open}
          requireExpiration={false}
          sharingEnabled
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return () => request.mock.calls.map((call) => call[1]);
};

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

describe("ShareLinkDialog link list requests", () => {
  afterEach(() => vi.restoreAllMocks());

  it.each<Permission[]>([["shares:read"], ["shares:write"]])(
    "lists the links with %s once open",
    async (permission) => {
      setAuth({ permissions: ["books:read", permission] });
      const paths = renderDialog(true);

      await waitFor(() => expect(paths()).toContain(LINKS_PATH));
    },
  );

  it("sends nothing while closed", async () => {
    setAuth({ permissions: ["books:read", "shares:read", "shares:write"] });
    const paths = renderDialog(false);
    await flush();

    expect(paths()).not.toContain(LINKS_PATH);
  });

  it("sends nothing for a role without a shares permission", async () => {
    setAuth({ permissions: ["books:read", "books:write"] });
    const paths = renderDialog(true);
    await flush();

    expect(paths()).not.toContain(LINKS_PATH);
  });
});
