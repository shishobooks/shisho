import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ShareLinkResponse } from "@/types";

import { ShareLinkDialog } from "./ShareLinkDialog";

const mocks = vi.hoisted(() => ({
  links: [] as ShareLinkResponse[],
  listOptions: [] as Array<{ enabled?: boolean } | undefined>,
  create: vi.fn(),
}));

vi.mock("@/hooks/queries/sharing", () => ({
  useBookShareLinks: (_bookId: number, options?: { enabled?: boolean }) => {
    mocks.listOptions.push(options);
    return { data: mocks.links, isLoading: false };
  },
  useCreateShareLink: () => ({ mutateAsync: mocks.create, isPending: false }),
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
      canCreate
      canList
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
    renderDialog({ canCreate: false });

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
});
