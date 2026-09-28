import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  PermissionEReaderBrowser,
  PermissionKoboSync,
  type APIKey,
} from "@/types/generated/apikeys";
import { copyText } from "@/utils/clipboard";

import SecuritySettings from "./SecuritySettings";

vi.mock("@/utils/clipboard", () => ({ copyText: vi.fn() }));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/components/library/TopNav", () => ({ default: () => null }));

const auth = vi.hoisted(() => ({ canReadLibraries: true }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    user: { must_change_password: false },
    refetch: vi.fn(),
    hasPermission: (resource: string, operation: string) =>
      resource === "libraries" && operation === "read" && auth.canReadLibraries,
  }),
}));

const apiKey = (id: string, permission: string): APIKey => ({
  id,
  userId: 1,
  name: `Device ${id}`,
  key: `key-${id}`,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  permissions: [
    {
      id: `p-${id}`,
      apiKeyId: id,
      permission,
      createdAt: "2026-01-01T00:00:00Z",
    },
  ],
});

const mutation = () => ({
  mutate: vi.fn(),
  mutateAsync: vi.fn(),
  isPending: false,
});

vi.mock("@/hooks/queries/apiKeys", () => ({
  useApiKeys: () => ({
    isLoading: false,
    data: [
      apiKey("ereader", PermissionEReaderBrowser),
      apiKey("kobo", PermissionKoboSync),
    ],
  }),
  useAddApiKeyPermission: () => mutation(),
  useClearKoboSync: () => mutation(),
  useCreateApiKey: () => mutation(),
  useDeleteApiKey: () => mutation(),
  useGenerateShortUrl: () => ({
    isPending: false,
    mutateAsync: vi.fn().mockResolvedValue({ shortCode: "abc123" }),
  }),
}));

const librariesOptions = vi.hoisted(
  () => [] as Array<{ enabled?: boolean } | undefined>,
);

vi.mock("@/hooks/queries/libraries", () => ({
  useLibraries: (_query?: unknown, options?: { enabled?: boolean }) => {
    librariesOptions.push(options);
    return {
      data:
        options?.enabled === false
          ? undefined
          : { items: [{ id: 1, name: "Fiction" }], total: 1 },
    };
  },
}));
vi.mock("@/hooks/queries/lists", () => ({
  useListLists: () => ({ data: { items: [], total: 0 } }),
}));
vi.mock("@/hooks/queries/users", () => ({
  useResetPassword: () => mutation(),
}));

const renderPage = () =>
  render(
    <MemoryRouter>
      <SecuritySettings />
    </MemoryRouter>,
  );

// Each row has a Setup button; the eReader row renders first.
const openSetup = async (index: number) => {
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getAllByRole("button", { name: "Setup" })[index]);
  return user;
};

describe("SecuritySettings copy buttons", () => {
  beforeEach(() => {
    auth.canReadLibraries = true;
    librariesOptions.length = 0;
    vi.mocked(copyText).mockReset();
    vi.mocked(toast.success).mockReset();
    vi.mocked(toast.error).mockReset();
  });

  it("copies the eReader setup URL through copyText", async () => {
    vi.mocked(copyText).mockResolvedValue(true);
    const user = await openSetup(0);

    await user.click(
      await screen.findByRole("button", { name: "Copy setup URL" }),
    );

    expect(copyText).toHaveBeenCalledWith(`${window.location.origin}/e/abc123`);
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Copied to clipboard"),
    );
  });

  it("toasts an error when the eReader setup URL copy fails", async () => {
    vi.mocked(copyText).mockResolvedValue(false);
    const user = await openSetup(0);

    await user.click(
      await screen.findByRole("button", { name: "Copy setup URL" }),
    );

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Could not copy the URL"),
    );
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("copies the Kobo sync URL through copyText", async () => {
    vi.mocked(copyText).mockResolvedValue(true);
    const user = await openSetup(1);

    await user.click(
      await screen.findByRole("button", { name: "Copy sync URL" }),
    );

    expect(copyText).toHaveBeenCalledWith(
      `${window.location.origin}/kobo/key-kobo/all`,
    );
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Copied to clipboard"),
    );
  });

  it("toasts an error when the Kobo sync URL copy fails", async () => {
    vi.mocked(copyText).mockResolvedValue(false);
    const user = await openSetup(1);

    await user.click(
      await screen.findByRole("button", { name: "Copy sync URL" }),
    );

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Could not copy the URL"),
    );
    expect(toast.success).not.toHaveBeenCalled();
  });
});

describe("SecuritySettings Kobo sync scope", () => {
  beforeEach(() => {
    auth.canReadLibraries = true;
    librariesOptions.length = 0;
  });

  it("offers the Library scope to a role that can read libraries", async () => {
    await openSetup(1);

    expect(
      await screen.findByRole("button", { name: "Library" }),
    ).toBeInTheDocument();
  });

  it("hides the Library scope and requests no libraries without Libraries Read", async () => {
    auth.canReadLibraries = false;
    await openSetup(1);

    expect(
      await screen.findByRole("button", { name: "All Libraries" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Library" }),
    ).not.toBeInTheDocument();
    expect(librariesOptions.length).toBeGreaterThan(0);
    for (const options of librariesOptions) {
      expect(options?.enabled).toBe(false);
    }
  });
});
