import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ShareLinkResponse } from "@/types";
import { formatDateTime } from "@/utils/format";

import { ShareLinkDialog } from "./ShareLinkDialog";

const mocks = vi.hoisted(() => ({
  links: [] as ShareLinkResponse[],
  listOptions: [] as Array<{ enabled?: boolean } | undefined>,
  create: vi.fn(),
  revoke: vi.fn(),
  remove: vi.fn(),
}));

vi.mock("@/hooks/queries/sharing", () => ({
  useBookShareLinks: (_bookId: number, options?: { enabled?: boolean }) => {
    mocks.listOptions.push(options);
    return { data: mocks.links, isLoading: false };
  },
  useCreateShareLink: () => ({ mutateAsync: mocks.create, isPending: false }),
  useRevokeShareLink: () => ({ mutateAsync: mocks.revoke, isPending: false }),
  useDeleteShareLink: () => ({ mutateAsync: mocks.remove, isPending: false }),
}));

const link = (overrides: Partial<ShareLinkResponse>): ShareLinkResponse => ({
  id: 1,
  created_at: "2026-09-20T00:00:00Z",
  updated_at: "2026-09-20T00:00:00Z",
  token: "tok-active",
  book_id: 7,
  created_by_user_id: 1,
  open_count: 0,
  download_count: 0,
  state: "active",
  created_by_username: "admin",
  ...overrides,
});

const renderDialog = (
  props: Partial<React.ComponentProps<typeof ShareLinkDialog>> = {},
) =>
  render(
    <ShareLinkDialog
      bookId={7}
      bookTitle="Test Book"
      canList
      canWrite
      onOpenChange={() => {}}
      open
      requireExpiration={false}
      {...props}
    />,
  );

const expirationOptions = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(screen.getByRole("combobox", { name: "Expires after" }));
  return screen.getAllByRole("option").map((option) => option.textContent);
};

afterEach(() => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: undefined,
  });
});

beforeEach(() => {
  mocks.links = [];
  mocks.listOptions.length = 0;
  mocks.create.mockReset().mockResolvedValue(link({}));
  mocks.revoke.mockReset().mockResolvedValue(link({ state: "revoked" }));
  mocks.remove.mockReset().mockResolvedValue(undefined);
});

describe("ShareLinkDialog", () => {
  it("preselects 7 days and offers Never last when expiration is optional", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDialog();

    expect(
      screen.getByRole("combobox", { name: "Expires after" }),
    ).toHaveTextContent("7 days");
    expect(await expirationOptions(user)).toEqual([
      "1 day",
      "7 days",
      "30 days",
      "Never",
    ]);
  });

  it("does not offer Never when the admin requires expiration", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDialog({ requireExpiration: true });

    expect(await expirationOptions(user)).toEqual([
      "1 day",
      "7 days",
      "30 days",
    ]);
  });

  it("creates a link with the trimmed label and an absolute expiry", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDialog();

    await user.type(screen.getByLabelText("Label (optional)"), "  for Alice ");
    await user.click(screen.getByRole("combobox", { name: "Expires after" }));
    await user.click(screen.getByRole("option", { name: "30 days" }));
    const before = Date.now();
    await user.click(screen.getByRole("button", { name: "Create link" }));

    expect(mocks.create).toHaveBeenCalledTimes(1);
    const { bookId, payload } = mocks.create.mock.calls[0][0];
    expect(bookId).toBe(7);
    expect(payload.label).toBe("for Alice");
    const expires = new Date(payload.expires_at).getTime();
    const thirtyDays = 30 * 24 * 60 * 60 * 1000;
    expect(expires).toBeGreaterThanOrEqual(before + thirtyDays - 1000);
    expect(expires).toBeLessThanOrEqual(Date.now() + thirtyDays + 1000);
    expect(screen.getByLabelText("Label (optional)")).toHaveValue("");
  });

  it("sends no expiry and no label for a Never link without a label", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDialog();

    await user.click(screen.getByRole("combobox", { name: "Expires after" }));
    await user.click(screen.getByRole("option", { name: "Never" }));
    await user.click(screen.getByRole("button", { name: "Create link" }));

    expect(mocks.create).toHaveBeenCalledWith({
      bookId: 7,
      payload: { label: undefined, expires_at: undefined },
    });
  });

  it("lists every link with its creator, state, and expiry", () => {
    mocks.links = [
      link({
        id: 1,
        label: "for Alice",
        expires_at: "2026-10-04T12:00:00Z",
      }),
      link({
        id: 2,
        token: "tok-expired",
        state: "expired",
        created_by_username: "editor",
        expires_at: "2026-09-21T12:00:00Z",
      }),
      link({ id: 3, token: "tok-never", label: "standing" }),
    ];
    renderDialog();

    const rows = screen.getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("for Alice");
    expect(rows[0]).toHaveTextContent("active");
    expect(rows[0]).toHaveTextContent("Created by admin");
    expect(rows[0]).toHaveTextContent("Expires");
    expect(rows[1]).toHaveTextContent("No label");
    expect(rows[1]).toHaveTextContent("expired");
    expect(rows[1]).toHaveTextContent("Created by editor");
    expect(rows[1]).toHaveTextContent("Expired");
    expect(rows[2]).toHaveTextContent("Never expires");
    expect(
      within(rows[1]).getByRole("button", { name: /Copy link/ }),
    ).toBeDisabled();
  });

  it("copies a URL built from the browser origin", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    mocks.links = [link({ label: "for Alice" })];
    renderDialog();

    await user.click(
      screen.getByRole("button", { name: "Copy link for Alice" }),
    );

    expect(writeText).toHaveBeenCalledWith(
      `${window.location.origin}/share/tok-active`,
    );
  });

  it("shows only the list for Shares Read without Shares Write", () => {
    mocks.links = [link({ label: "for Alice" })];
    renderDialog({ canWrite: false });

    expect(screen.queryByLabelText("Label (optional)")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Create link" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("for Alice")).toBeInTheDocument();
  });

  it("does not request the list without Shares Read", () => {
    renderDialog({ canList: false });

    for (const options of mocks.listOptions) {
      expect(options?.enabled).toBe(false);
    }
    expect(screen.queryByText("Links")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Create link" }),
    ).toBeInTheDocument();
  });

  it("shows opens, downloads, and last used for each link", () => {
    mocks.links = [
      link({
        id: 1,
        label: "used",
        open_count: 3,
        download_count: 1,
        last_accessed_at: "2026-09-25T15:30:00Z",
      }),
      link({ id: 2, label: "one open", open_count: 1, download_count: 2 }),
      link({ id: 3, label: "unused" }),
    ];
    renderDialog();

    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("3 opens · 1 download");
    expect(rows[0]).toHaveTextContent(
      `Last used ${formatDateTime("2026-09-25T15:30:00Z")}`,
    );
    expect(rows[1]).toHaveTextContent("1 open · 2 downloads");
    expect(rows[2]).toHaveTextContent("0 opens · 0 downloads · Never used");
  });

  it("revokes an active link after confirmation", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mocks.links = [link({ id: 4, label: "for Alice" })];
    renderDialog();

    await user.click(screen.getByRole("button", { name: "Revoke for Alice" }));
    const confirm = screen.getByRole("dialog", { name: "Revoke Link" });
    expect(confirm).toHaveTextContent('The link "for Alice" stops working');
    expect(confirm).toHaveTextContent("cannot be undone");
    expect(mocks.revoke).not.toHaveBeenCalled();
    await user.click(within(confirm).getByRole("button", { name: "Revoke" }));

    expect(mocks.revoke).toHaveBeenCalledWith({ bookId: 7, linkId: 4 });
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Revoke Link" }),
      ).not.toBeInTheDocument(),
    );
  });

  it("closes the confirmation when a revoke or delete fails", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mocks.revoke.mockRejectedValue(new Error("Share Link not found"));
    mocks.remove.mockRejectedValue(new Error("Share Link not found"));
    mocks.links = [link({ id: 4, label: "for Alice" })];
    renderDialog();

    // Another sharer may have deleted the link already; the list refetches
    // after any outcome, so the confirmation does not linger on it.
    await user.click(screen.getByRole("button", { name: "Revoke for Alice" }));
    await user.click(
      within(screen.getByRole("dialog", { name: "Revoke Link" })).getByRole(
        "button",
        { name: "Revoke" },
      ),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Revoke Link" }),
      ).not.toBeInTheDocument(),
    );

    await user.click(screen.getByRole("button", { name: "Delete for Alice" }));
    await user.click(
      within(screen.getByRole("dialog", { name: "Delete Link" })).getByRole(
        "button",
        { name: "Delete" },
      ),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Delete Link" }),
      ).not.toBeInTheDocument(),
    );
  });

  it("offers no revoke on a link that is not active", () => {
    mocks.links = [
      link({ id: 1, label: "revoked", state: "revoked" }),
      link({ id: 2, label: "expired", state: "expired" }),
    ];
    renderDialog();

    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("revoked");
    for (const row of rows) {
      expect(
        within(row).queryByRole("button", { name: /Revoke/ }),
      ).not.toBeInTheDocument();
      expect(
        within(row).getByRole("button", { name: /Copy link/ }),
      ).toBeDisabled();
      expect(
        within(row).getByRole("button", { name: /Delete/ }),
      ).toBeInTheDocument();
    }
  });

  it("deletes a link in any state after confirmation", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mocks.links = [
      link({ id: 1, label: "active" }),
      link({ id: 2, label: "revoked", state: "revoked" }),
    ];
    renderDialog();

    await user.click(screen.getByRole("button", { name: "Delete revoked" }));
    const confirm = screen.getByRole("dialog", { name: "Delete Link" });
    expect(confirm).toHaveTextContent('The link "revoked" is removed');
    await user.click(within(confirm).getByRole("button", { name: "Delete" }));
    expect(mocks.remove).toHaveBeenCalledWith({ bookId: 7, linkId: 2 });

    await user.click(screen.getByRole("button", { name: "Delete active" }));
    await user.click(
      within(screen.getByRole("dialog", { name: "Delete Link" })).getByRole(
        "button",
        { name: "Delete" },
      ),
    );
    expect(mocks.remove).toHaveBeenCalledWith({ bookId: 7, linkId: 1 });
  });

  it("shows counts but no revoke or delete without Shares Write", () => {
    mocks.links = [link({ label: "for Alice", open_count: 2 })];
    renderDialog({ canWrite: false });

    expect(screen.getByRole("listitem")).toHaveTextContent("2 opens");
    expect(
      screen.queryByRole("button", { name: /Revoke/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Delete/ }),
    ).not.toBeInTheDocument();
  });

  it("names an unlabeled link by its creator in the confirmation", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mocks.links = [link({ id: 5, created_by_username: "editor" })];
    renderDialog();

    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(
      screen.getByRole("dialog", { name: "Delete Link" }),
    ).toHaveTextContent(
      "The unlabeled link created by editor stops working immediately",
    );
  });

  it("shows when a revoked link was revoked instead of its expiry", () => {
    mocks.links = [
      link({
        state: "revoked",
        expires_at: "2026-10-04T12:00:00Z",
        revoked_at: "2026-09-26T09:15:00Z",
      }),
    ];
    renderDialog();

    const row = screen.getByRole("listitem");
    expect(row).toHaveTextContent(
      `Revoked ${formatDateTime("2026-09-26T09:15:00Z")}`,
    );
    expect(row).not.toHaveTextContent("Expires");
  });

  it("marks an active link paused by its creator and says why", () => {
    mocks.links = [
      link({
        id: 1,
        label: "deactivated",
        created_by_username: "editor",
        paused_reason: "creator_deactivated",
      }),
      link({
        id: 2,
        label: "lost access",
        created_by_username: "editor",
        paused_reason: "creator_no_library_access",
      }),
    ];
    renderDialog();

    const [deactivated, lostAccess] = screen.getAllByRole("listitem");
    expect(deactivated).toHaveTextContent("paused");
    expect(deactivated).not.toHaveTextContent("active");
    expect(deactivated).toHaveTextContent(
      "Paused because editor was deactivated",
    );
    expect(lostAccess).toHaveTextContent(
      "Paused because editor no longer has access to this library",
    );
    for (const row of [deactivated, lostAccess]) {
      // A paused link would 404 for the recipient, so it cannot be copied,
      // but it can still be revoked so it does not come back.
      expect(
        within(row).getByRole("button", { name: /Copy link/ }),
      ).toBeDisabled();
      expect(
        within(row).getByRole("button", { name: /Revoke/ }),
      ).toBeInTheDocument();
    }
  });

  it("does not tell the sharer a paused link will stop working", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mocks.links = [
      link({ id: 6, label: "paused", paused_reason: "creator_deactivated" }),
    ];
    renderDialog();

    await user.click(screen.getByRole("button", { name: "Revoke paused" }));
    const revoke = screen.getByRole("dialog", { name: "Revoke Link" });
    expect(revoke).toHaveTextContent(
      "will not work again when its creator's access returns",
    );
    expect(revoke).not.toHaveTextContent("stops working immediately");
    await user.click(within(revoke).getByRole("button", { name: "Cancel" }));

    await user.click(screen.getByRole("button", { name: "Delete paused" }));
    expect(
      screen.getByRole("dialog", { name: "Delete Link" }),
    ).not.toHaveTextContent("stops working immediately");
  });
});
