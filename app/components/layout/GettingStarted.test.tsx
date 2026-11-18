import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GettingStartedPanelProvider } from "@/contexts/GettingStarted";
import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type {
  GettingStartedResponse,
  GettingStartedStep,
  GettingStartedStepKey,
} from "@/types/generated/gettingstarted";

import GettingStarted from "./GettingStarted";
import UserMenu from "./UserMenu";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const KEYS: GettingStartedStepKey[] = [
  "library",
  "plugins",
  "invite",
  "device",
  "to_read",
  "start_reading",
];

const steps = (
  applies: GettingStartedStepKey[],
  done: GettingStartedStepKey[] = [],
  notForMe: GettingStartedStepKey[] = [],
): GettingStartedStep[] =>
  KEYS.map((key) => ({
    key,
    applies: applies.includes(key),
    done: done.includes(key),
    not_for_me: notForMe.includes(key),
  }));

const ADMIN_STEPS: GettingStartedStepKey[] = [
  "library",
  "invite",
  "device",
  "to_read",
  "start_reading",
];
const PERSONAL_STEPS: GettingStartedStepKey[] = [
  "device",
  "to_read",
  "start_reading",
];

/**
 * Serves the Getting Started routes from one in-memory list, as the server
 * would for the signed-in user.
 */
const serve = (initial: Partial<GettingStartedResponse>) => {
  const state: GettingStartedResponse = {
    enabled: true,
    dismissed: false,
    auto_open: false,
    steps: steps(ADMIN_STEPS),
    ...initial,
  };
  const request = vi
    .spyOn(API, "request")
    .mockImplementation(async (method, path) => {
      const notForMe = path.match(
        /^\/getting-started\/steps\/(\w+)\/not-for-me$/,
      );
      if (notForMe) {
        state.steps = state.steps.map((s) =>
          s.key === notForMe[1] ? { ...s, not_for_me: method === "POST" } : s,
        );
      } else if (path === "/getting-started/dismiss") {
        state.dismissed = true;
      } else if (path === "/getting-started/restore") {
        state.enabled = true;
        state.dismissed = false;
      } else if (path === "/getting-started/auto-open") {
        state.auto_open = false;
      } else if (path !== "/getting-started") {
        throw new Error(`unexpected request ${method} ${path}`);
      }
      return structuredClone(state);
    });
  return { state, request };
};

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

const newClient = () =>
  new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });

const renderNav = (client = newClient()) => {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/"]}>
        <GettingStartedPanelProvider>
          <Routes>
            <Route
              element={
                <>
                  <GettingStarted />
                  <UserMenu />
                </>
              }
              path="*"
            />
          </Routes>
        </GettingStartedPanelProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const pill = () => screen.findByRole("button", { name: /^Getting Started/ });

const panel = () => screen.findByRole("dialog", { name: "Getting Started" });

const setPhoneWidth = (phone: boolean) => {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: phone,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
};

describe("GettingStarted", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["books:read"] });
  });

  afterEach(() => {
    // jsdom has no matchMedia; tests that add one take it away again.
    delete (window as { matchMedia?: unknown }).matchMedia;
  });

  it("shows the progress of the steps that apply in the pill", async () => {
    serve({ steps: steps(ADMIN_STEPS, ["library", "device"]) });
    renderNav();

    expect(await pill()).toHaveAccessibleName("Getting Started, 2 of 5 done");
  });

  it("lists only the steps that apply, each linking to where it gets done", async () => {
    serve({ steps: steps(PERSONAL_STEPS) });
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    const list = within(await panel()).getByRole("list");
    const items = within(list).getAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(
      within(list).getByRole("link", { name: "Connect a reading device" }),
    ).toHaveAttribute("href", "/user/devices");
    expect(within(list).queryByText("Set up your first library")).toBeNull();
    expect(within(list).queryByText("Invite someone")).toBeNull();
  });

  it("links the admin steps to today's pages", async () => {
    serve({ steps: steps([...ADMIN_STEPS, "plugins"]) });
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    const dialog = await panel();
    expect(
      within(dialog).getByRole("link", { name: "Set up your first library" }),
    ).toHaveAttribute("href", "/libraries/create");
    expect(
      within(dialog).getByRole("link", { name: "Add metadata plugins" }),
    ).toHaveAttribute("href", "/admin/plugins/discover");
    expect(
      within(dialog).getByRole("link", { name: "Invite someone" }),
    ).toHaveAttribute("href", "/admin/users/create");
  });

  it("refetches the list each time the panel opens", async () => {
    const { request } = serve({});
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    await panel();
    await waitFor(() =>
      expect(
        request.mock.calls.filter(([, path]) => path === "/getting-started"),
      ).toHaveLength(2),
    );
  });

  it("marks a step Not for me, which drops it from the count, and undoes it", async () => {
    const { request } = serve({ steps: steps(ADMIN_STEPS) });
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    const dialog = await panel();
    await user.click(
      within(dialog).getByRole("button", {
        name: "Not for me: Invite someone",
      }),
    );

    expect(request).toHaveBeenCalledWith(
      "POST",
      "/getting-started/steps/invite/not-for-me",
    );
    expect(await within(dialog).findByText("0 of 4 done")).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole("button", {
        name: "Undo Not for me: Invite someone",
      }),
    );
    expect(request).toHaveBeenCalledWith(
      "DELETE",
      "/getting-started/steps/invite/not-for-me",
    );
    expect(await within(dialog).findByText("0 of 5 done")).toBeInTheDocument();
  });

  it("dismisses the list with no confirmation and restores it from the user menu", async () => {
    const { request } = serve({});
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    await user.click(
      within(await panel()).getByRole("button", { name: "Dismiss this list" }),
    );
    expect(request).toHaveBeenCalledWith("POST", "/getting-started/dismiss");
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: /^Getting Started/ }),
      ).toBeNull(),
    );

    await user.click(screen.getByRole("button", { name: "Account menu" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Getting Started" }),
    );
    expect(request).toHaveBeenCalledWith("POST", "/getting-started/restore");
    expect(await panel()).toBeInTheDocument();
    expect(await pill()).toBeInTheDocument();
  });

  it("shows nothing when the list is off, dismissed, or has no steps for the role", async () => {
    for (const state of [
      { enabled: false },
      { dismissed: true },
      { steps: steps([]) },
    ]) {
      vi.restoreAllMocks();
      const { request } = serve(state);
      const { unmount } = renderNav();
      await waitFor(() =>
        expect(request).toHaveBeenCalledWith(
          "GET",
          "/getting-started",
          null,
          null,
          expect.anything(),
        ),
      );
      expect(
        screen.queryByRole("button", { name: /^Getting Started/ }),
      ).toBeNull();
      unmount();
    }
  });

  it("opens on its own once on the first visit and clears the flag", async () => {
    const { request } = serve({ auto_open: true });
    renderNav();

    expect(await panel()).toBeInTheDocument();
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "DELETE",
        "/getting-started/auto-open",
      ),
    );
  });

  // Focus on the first "Not for me" would let one Enter skip a step.
  it("puts focus on the panel's title when it opens", async () => {
    serve({ auto_open: true });
    renderNav();

    const dialog = await panel();
    await waitFor(() =>
      expect(
        within(dialog).getByRole("heading", { name: "Getting Started" }),
      ).toHaveFocus(),
    );
  });

  it("puts focus on the sheet's title when it opens on a phone", async () => {
    setPhoneWidth(true);
    serve({ steps: steps(PERSONAL_STEPS) });
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    const dialog = await panel();
    await waitFor(() =>
      expect(
        within(dialog).getByRole("heading", { name: "Getting Started" }),
      ).toHaveFocus(),
    );
  });

  it("opens on its own only once, even when another page mounts before the flag clears", async () => {
    const { request } = serve({ auto_open: true });
    const original = request.getMockImplementation()!;
    // The clear never answers, as if the user moved on mid-request.
    request.mockImplementation((method, path, ...rest) =>
      path === "/getting-started/auto-open"
        ? new Promise(() => {})
        : original(method, path, ...rest),
    );
    const client = newClient();
    const first = renderNav(client);
    expect(await panel()).toBeInTheDocument();
    first.unmount();

    renderNav(client);
    await pill();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      request.mock.calls.filter(
        ([, path]) => path === "/getting-started/auto-open",
      ),
    ).toHaveLength(1);
  });

  it("does not open on its own without the flag", async () => {
    const { request } = serve({ auto_open: false });
    renderNav();

    await pill();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(request).not.toHaveBeenCalledWith(
      "DELETE",
      "/getting-started/auto-open",
    );
  });

  it("shows You're all set once every step is done or Not for me, then hides the pill", async () => {
    const { request } = serve({
      steps: steps(PERSONAL_STEPS, ["device", "to_read"]),
    });
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    const dialog = await panel();
    await user.click(
      within(dialog).getByRole("button", {
        name: "Not for me: Start reading something",
      }),
    );
    expect(
      await within(dialog).findByText("You're all set"),
    ).toBeInTheDocument();

    // A step marked by mistake can still be undone from there.
    expect(
      within(dialog).getByRole("button", {
        name: "Undo Not for me: Start reading something",
      }),
    ).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    expect(request).toHaveBeenCalledWith("POST", "/getting-started/dismiss");
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: /^Getting Started/ }),
      ).toBeNull(),
    );
  });

  it("dismisses the list when You're all set is closed any other way", async () => {
    const { request } = serve({
      steps: steps(PERSONAL_STEPS, ["device", "to_read", "start_reading"]),
    });
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    expect(
      await within(await panel()).findByText("You're all set"),
    ).toBeInTheDocument();
    const success = vi.spyOn(toast, "success");
    await user.keyboard("{Escape}");

    expect(request).toHaveBeenCalledWith("POST", "/getting-started/dismiss");
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: /^Getting Started/ }),
      ).toBeNull(),
    );
    // Closing it hides the pill, so it says where to find the list again.
    expect(success).toHaveBeenCalledWith(
      "Getting Started is hidden",
      expect.objectContaining({
        description: "Bring it back from your account menu.",
      }),
    );
  });

  it("says nothing was finished when every step is Not for me", async () => {
    serve({
      steps: steps(PERSONAL_STEPS, [], ["device", "to_read", "start_reading"]),
    });
    const user = setupUser();
    renderNav();

    const button = await pill();
    expect(button).toHaveAccessibleName("Getting Started, all set");
    await user.click(button);
    const dialog = await panel();
    expect(within(dialog).queryByText("0 of 0 done")).toBeNull();
    expect(
      within(dialog).getByText("Nothing left on your list"),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText("Every step is marked Not for me."),
    ).toBeInTheDocument();
    expect(
      within(dialog).queryByText("You've finished your first steps."),
    ).toBeNull();
  });

  it("clears the hidden-list toast once the list is restored", async () => {
    serve({});
    const success = vi.spyOn(toast, "success");
    const dismissToast = vi.spyOn(toast, "dismiss");
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    await user.click(
      within(await panel()).getByRole("button", { name: "Dismiss this list" }),
    );
    await waitFor(() =>
      expect(success).toHaveBeenCalledWith(
        "Getting Started is hidden",
        expect.objectContaining({ id: "getting-started-hidden" }),
      ),
    );

    await user.click(screen.getByRole("button", { name: "Account menu" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Getting Started" }),
    );
    expect(await panel()).toBeInTheDocument();
    expect(dismissToast).toHaveBeenCalledWith("getting-started-hidden");
  });

  it("returns focus to the account menu button after other menu entries", async () => {
    serve({});
    const user = setupUser();
    renderNav();

    const menuButton = screen.getByRole("button", { name: "Account menu" });
    await user.click(menuButton);
    await screen.findByRole("menuitem", { name: "Preferences" });
    await user.keyboard("{Escape}");

    await waitFor(() => expect(menuButton).toHaveFocus());
  });

  it("returns focus to the account menu button when restoring fails", async () => {
    const { request } = serve({ dismissed: true });
    const original = request.getMockImplementation()!;
    request.mockImplementation((method, path, ...rest) =>
      // It fails only after the menu has finished closing.
      path === "/getting-started/restore"
        ? new Promise((_, reject) =>
            setTimeout(() => reject(new Error("restore failed")), 1000),
          )
        : original(method, path, ...rest),
    );
    const user = setupUser();
    renderNav();

    const menuButton = screen.getByRole("button", { name: "Account menu" });
    await user.click(menuButton);
    await user.click(
      await screen.findByRole("menuitem", { name: "Getting Started" }),
    );
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    await act(() => vi.advanceTimersByTimeAsync(1000));

    await waitFor(() => expect(menuButton).toHaveFocus());
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers no user menu entry to a role that can do none of the steps", async () => {
    setAuth({ permissions: ["series:read"] });
    serve({ steps: steps([]) });
    const user = setupUser();
    renderNav();

    await user.click(screen.getByRole("button", { name: "Account menu" }));
    expect(
      await screen.findByRole("menuitem", { name: "Preferences" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("menuitem", { name: "Getting Started" }),
    ).toBeNull();
  });

  it("opens as a bottom sheet on a phone", async () => {
    setPhoneWidth(true);
    serve({ steps: steps(PERSONAL_STEPS) });
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    const dialog = await panel();
    expect(dialog).toHaveAttribute("data-layout", "sheet");
    expect(
      within(dialog).getByRole("link", { name: "Add a book to To Read" }),
    ).toBeInTheDocument();
  });

  it("opens as a popover on a wider screen", async () => {
    setPhoneWidth(false);
    serve({ steps: steps(PERSONAL_STEPS) });
    const user = setupUser();
    renderNav();

    await user.click(await pill());
    expect(await panel()).toHaveAttribute("data-layout", "popover");
  });

  it("shows and asks for nothing in Demo Mode", async () => {
    setAuth({ permissions: ["books:read"], demoMode: true });
    const { request } = serve({ enabled: false, steps: [] });
    const user = setupUser();
    renderNav();

    await user.click(screen.getByRole("button", { name: "Account menu" }));
    expect(
      await screen.findByRole("menuitem", { name: "Preferences" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("menuitem", { name: "Getting Started" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: /^Getting Started/ }),
    ).toBeNull();
    expect(request).not.toHaveBeenCalled();
  });
});
