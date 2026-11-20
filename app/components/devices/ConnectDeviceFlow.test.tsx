import {
  keepPreviousData,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import DeviceDetail from "@/components/pages/DeviceDetail";
import Devices from "@/components/pages/Devices";
import { API, ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import { eventStream, MockEventSource } from "@/testing/eventSource";
import { emulatePhoneWidth } from "@/testing/phoneWidth";
import { WithSSE } from "@/testing/WithSSE";
import type { DeviceResponse, DeviceType } from "@/types/generated/devices";
import { pageHostname } from "@/utils/location";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("@/libraries/build", () => ({ checkServerBuild: vi.fn() }));
vi.mock("@/components/library/TopNav", () => ({ default: () => null }));
vi.mock("@/utils/location", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/utils/location")>()),
  pageHostname: vi.fn(),
}));

const CONNECTIONS: Record<DeviceType, DeviceResponse["connections"]> = {
  kobo: ["kobo_sync", "ereader_browser"],
  koreader: ["koreader_plugin", "opds", "koreader_sync"],
  kindle: ["ereader_browser"],
  opds: ["opds"],
  browser: ["ereader_browser"],
  shisho_app: ["shisho_app"],
};

const newDevice = (
  name: string,
  deviceType: DeviceType,
  overrides: Partial<DeviceResponse> = {},
): DeviceResponse => ({
  id: "new-1",
  user_id: 1,
  name,
  device_type: deviceType,
  scope_type: "all",
  url_scope_pending: false,
  delivery_rules_mode: "default",
  delivery_rules: [],
  delivery_rule_options: [],
  has_preset: false,
  unlinked_books: 0,
  credential: "abcdefghjkmnpqrs",
  connections: CONNECTIONS[deviceType],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  ...overrides,
});

// stubAPI answers the Device routes: the user starts with existing Devices,
// and the one the flow creates stays unconnected until connect() is called,
// as the server would record its first request.
const stubAPI = (existing: DeviceResponse[] = [], initial?: DeviceResponse) => {
  let created: DeviceResponse | undefined = initial;
  let deleted = false;
  const request = vi
    .spyOn(API, "request")
    .mockImplementation(async (method, path, body) => {
      if (method === "GET" && path === "/user/devices")
        return created ? [...existing, created] : existing;
      if (method === "POST" && path === "/user/devices") {
        const payload = body as { name: string; device_type: DeviceType };
        created = newDevice(payload.name, payload.device_type);
        return created;
      }
      if (
        method === "GET" &&
        created &&
        path === `/user/devices/${created.id}`
      ) {
        if (deleted)
          throw new ShishoAPIError("Device not found", "not_found", 404);
        return created;
      }
      if (method === "POST" && path.endsWith("/short-url"))
        return {
          id: "short-1",
          device_id: "new-1",
          short_code: "abc123",
          expires_at: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
          created_at: new Date().toISOString(),
        };
      throw new Error(`unexpected request ${method} ${path}`);
    });
  const connect = (overrides: Partial<DeviceResponse> = {}) => {
    created = {
      ...created!,
      last_accessed_at: "2026-01-02T00:00:00Z",
      ...overrides,
    };
  };
  const remove = () => {
    deleted = true;
  };
  return { request, connect, remove };
};

const renderAt = (path: string) => {
  // The app's own query defaults, which keep a query's previous data while
  // a new key loads, without its retries.
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        placeholderData: keepPreviousData,
        refetchOnMount: "always",
        retry: false,
        staleTime: Infinity,
      },
    },
  });
  const router = createMemoryRouter(
    [
      { path: "/user/devices", element: <Devices /> },
      { path: "/user/devices/:id", element: <DeviceDetail /> },
    ],
    { initialEntries: [path] },
  );
  return render(
    <QueryClientProvider client={queryClient}>
      <WithSSE>
        <RouterProvider router={router} />
      </WithSSE>
    </QueryClientProvider>,
  );
};

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const flow = () => screen.findByRole("dialog");

// choose picks what the user reads on and goes on to the setup steps.
const choose = async (
  user: ReturnType<typeof setupUser>,
  label: string | RegExp,
) => {
  const dialog = await flow();
  await user.click(within(dialog).getByRole("radio", { name: label }));
  await user.click(within(dialog).getByRole("button", { name: "Continue" }));
  await within(dialog).findByText("Step 2 of 3");
  return dialog;
};

describe("Connect a device", () => {
  beforeEach(() => {
    setAuth({ user: { username: "robin" } });
    vi.mocked(pageHostname).mockReturnValue("shisho.example.com");
    MockEventSource.instances = [];
    vi.stubGlobal("EventSource", MockEventSource);
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("opens from the link Getting Started follows, asking what the user reads on", async () => {
    stubAPI();
    renderAt("/user/devices?connect");

    const dialog = await flow();
    expect(within(dialog).getByText("What do you read on?")).toBeVisible();
    expect(within(dialog).getByText("Step 1 of 3")).toBeInTheDocument();
  });

  it("opens from the Devices page's button", async () => {
    stubAPI([newDevice("Libra 2", "kobo", { id: "kobo-1" })]);
    const user = setupUser();
    renderAt("/user/devices");

    await user.click(
      await screen.findByRole("button", { name: "Connect Device" }),
    );
    expect(
      within(await flow()).getByText("What do you read on?"),
    ).toBeInTheDocument();
  });

  it.each([
    [/A Kobo/, "Kobo", "kobo"],
    [/^KOReader/, "KOReader", "koreader"],
    [/A Kindle's browser/, "Kindle", "kindle"],
    [/A reading app on a phone or tablet/, "Reading app", "opds"],
    [/Another e-reader's browser/, "E-reader", "browser"],
  ])(
    "maps %s to a Device type, with its name filled in",
    async (label, name, deviceType) => {
      const { request } = stubAPI();
      const user = setupUser();
      renderAt("/user/devices?connect");

      const dialog = await flow();
      await user.click(within(dialog).getByRole("radio", { name: label }));
      expect(within(dialog).getByLabelText("Name")).toHaveValue(name);
      await user.click(
        within(dialog).getByRole("button", { name: "Continue" }),
      );

      await waitFor(() =>
        expect(request).toHaveBeenCalledWith("POST", "/user/devices", {
          name,
          device_type: deviceType,
        }),
      );
      expect(
        await within(dialog).findByRole("heading", { name: `Set up ${name}` }),
      ).toBeInTheDocument();
    },
  );

  it("tells a Kobo user that KOReader on a Kobo is the KOReader choice", async () => {
    stubAPI();
    renderAt("/user/devices?connect");

    const dialog = await flow();
    expect(
      within(dialog).getByRole("radio", { name: /A Kobo/ }),
    ).toHaveAccessibleName(/Using KOReader on your Kobo\? Choose KOReader/);
    expect(
      within(dialog).getByRole("radio", { name: /^KOReader/ }),
    ).toHaveAccessibleName(/Kobo, Kindle, Android, PocketBook, and others/);
  });

  it("names a second Device of a kind apart from the first, and keeps a name typed by hand", async () => {
    stubAPI([newDevice("Kobo", "kobo", { id: "kobo-1" })]);
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await flow();
    await user.click(within(dialog).getByRole("radio", { name: /A Kobo/ }));
    const name = within(dialog).getByLabelText("Name");
    expect(name).toHaveValue("Kobo 2");

    await user.clear(name);
    await user.type(name, "Bedroom");
    await user.click(within(dialog).getByRole("radio", { name: /^KOReader/ }));
    expect(name).toHaveValue("Bedroom");
  });

  it.each([
    [/A Kindle's browser/, "Open the Kindle's web browser"],
    [/Another e-reader's browser/, "Open the e-reader's web browser"],
  ])("names the device whose browser %s opens", async (label, step) => {
    stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, label);
    expect(
      within(dialog).getByRole("list", { name: "eReader browser setup" }),
    ).toHaveTextContent(step);
  });

  it("says to eject only a device that was plugged in over USB", async () => {
    stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /^KOReader/);
    const steps = () =>
      within(dialog).getByRole("list", { name: "KOReader plugin setup" });
    // Before the hardware is known, Android stays possible.
    expect(steps()).not.toHaveTextContent("Eject the e-reader and restart");
    expect(steps()).toHaveTextContent(/if it's connected with USB/);

    await user.click(within(dialog).getByRole("radio", { name: "Android" }));
    expect(steps()).not.toHaveTextContent(/Eject/);
    expect(steps()).toHaveTextContent(/Restart KOReader with Wi-Fi on/);

    await user.click(within(dialog).getByRole("radio", { name: "Kindle" }));
    expect(steps()).toHaveTextContent(
      /Eject the e-reader and restart KOReader with Wi-Fi on/,
    );
  });

  it("says where a Kobo's hidden .kobo folder is and how to show it", async () => {
    stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /A Kobo/);
    const steps = within(dialog).getByRole("list", { name: "Kobo sync setup" });
    expect(steps).toHaveTextContent(/\.kobo folder\. It's hidden/);
    expect(steps).toHaveTextContent(
      /Cmd\+Shift\+\..*View > Show > Hidden items/,
    );
    expect(within(dialog).getByLabelText("Kobo sync URL")).toHaveTextContent(
      `${window.location.origin}/kobo/abcdefghjkmnpqrs`,
    );
  });

  it("asks which device runs KOReader and shows only that one's plugins folder", async () => {
    stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /^KOReader/);
    expect(
      within(dialog).getByRole("button", { name: "Download Plugin" }),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByRole("button", {
        name: "KOReader asking for a pairing code?",
      }),
    ).toBeInTheDocument();
    expect(
      within(dialog).queryByLabelText("KOReader plugins folder"),
    ).not.toBeInTheDocument();

    await user.click(within(dialog).getByRole("radio", { name: "PocketBook" }));
    expect(
      within(dialog).getAllByLabelText("KOReader plugins folder"),
    ).toHaveLength(1);
    expect(
      within(dialog).getByLabelText("KOReader plugins folder"),
    ).toHaveTextContent("applications/koreader/plugins");
    expect(within(dialog).queryByText(/\.adds/)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/start with a dot/)).toBeNull();

    // A Kobo's folder is hidden, so it says how to show it.
    await user.click(within(dialog).getByRole("radio", { name: "Kobo" }));
    expect(
      within(dialog).getByLabelText("KOReader plugins folder"),
    ).toHaveTextContent(".adds/koreader/plugins");
    expect(
      within(dialog).getByText(/start with a dot are hidden/),
    ).toBeInTheDocument();
  });

  it("warns that a device can't reach Shisho at localhost, and marks the URLs", async () => {
    vi.mocked(pageHostname).mockReturnValue("localhost");
    stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /A Kobo/);
    expect(within(dialog).getByRole("note")).toHaveTextContent(
      "Your device can't reach localhost. Open Shisho at the address your devices use and set up the device from there.",
    );
    expect(
      within(dialog).getByLabelText("Kobo sync URL"),
    ).toHaveAccessibleDescription("Other devices can't reach this address.");
  });

  it.each(["127.0.0.1", "[::1]", "0.0.0.0"])(
    "warns at the loopback address %s too",
    async (host) => {
      vi.mocked(pageHostname).mockReturnValue(host);
      stubAPI();
      const user = setupUser();
      renderAt("/user/devices?connect");

      const dialog = await choose(user, /A reading app/);
      expect(within(dialog).getByRole("note")).toHaveTextContent(
        "Your device can't reach",
      );
    },
  );

  it("gives no warning at an address other devices can reach", async () => {
    stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /A Kobo/);
    expect(within(dialog).queryByRole("note")).not.toBeInTheDocument();
    expect(
      within(dialog).getByLabelText("Kobo sync URL"),
    ).not.toHaveAccessibleDescription();
  });

  it("waits for the device and switches to Connected! when it first connects", async () => {
    const { connect } = stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /Another e-reader's browser/);
    await user.click(
      within(dialog).getByRole("button", { name: "Get Setup URL" }),
    );
    expect(await within(dialog).findByLabelText("Setup URL")).toHaveTextContent(
      `${window.location.origin}/e/abc123`,
    );
    await user.click(within(dialog).getByRole("button", { name: "Next" }));

    expect(
      within(dialog).getByRole("heading", { name: "Waiting for your device" }),
    ).toBeInTheDocument();
    const help = within(dialog).getByRole("link", { name: "Not working?" });
    expect(help).toHaveAttribute(
      "href",
      "https://www.shishobooks.com/docs/ereader-browser#troubleshooting",
    );
    expect(help).toHaveAttribute("target", "_blank");

    connect();
    await eventStream().emit("device.connected", { device_id: "new-1" });

    expect(
      await within(dialog).findByRole("heading", { name: "Connected!" }),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole("status")).toHaveTextContent(
      "E-reader is connected to Shisho.",
    );
    expect(
      within(dialog).getByRole("link", { name: "Open Device" }),
    ).toHaveAttribute("href", "/user/devices/new-1");
  });

  it("keeps waiting while no event says the device connected", async () => {
    stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /A reading app/);
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    expect(
      within(dialog).getByRole("link", { name: "Not working?" }),
    ).toHaveAttribute(
      "href",
      "https://www.shishobooks.com/docs/opds#troubleshooting",
    );

    await eventStream().emit("job.created", { job_id: 1 });
    expect(
      within(dialog).getByRole("heading", { name: "Waiting for your device" }),
    ).toBeInTheDocument();
  });

  it("tells a connected KOReader what's left: progress sync, linking books, and where the books are", async () => {
    const { connect } = stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /^KOReader/);
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    expect(
      within(dialog).getByRole("link", { name: "Not working?" }),
    ).toHaveAttribute(
      "href",
      "https://www.shishobooks.com/docs/koreader#troubleshooting",
    );

    connect({
      last_accessed_at: undefined,
      plugin_last_connected_at: "2026-01-02T00:00:00Z",
      unlinked_books: 2,
    });
    await eventStream().emit("device.connected", { device_id: "new-1" });

    await within(dialog).findByRole("heading", { name: "Connected!" });
    const next = within(
      within(dialog).getByRole("list", { name: "What's next" }),
    ).getAllByRole("listitem");
    expect(next).toHaveLength(3);
    expect(next[0]).toHaveTextContent(
      /turn it off under Tools > Progress sync/,
    );
    expect(next[1]).toHaveTextContent(/link them to their books/);
    expect(next[1]).toHaveTextContent("2 books waiting now.");
    expect(
      within(next[1]).getByRole("link", { name: "this Device's page" }),
    ).toHaveAttribute("href", "/user/devices/new-1");
    expect(next[2]).toHaveTextContent(
      "Find your books in KOReader under Search > OPDS catalog > Shisho.",
    );
  });

  it("starts over at What do you read on? when opened again after connecting one", async () => {
    const { connect } = stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /A reading app/);
    connect();
    await eventStream().emit("device.connected", { device_id: "new-1" });
    await user.click(
      await within(dialog).findByRole("button", { name: "Done" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getByRole("button", { name: "Connect Device" }));
    const again = await flow();
    expect(within(again).getByText("What do you read on?")).toBeInTheDocument();
    expect(within(again).queryByText("Connected!")).not.toBeInTheDocument();
  });

  it("goes straight to the setup steps once the Device is made, without waiting on a request", async () => {
    const { request } = stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /A Kobo/);
    expect(within(dialog).getByLabelText("Kobo sync URL")).toBeInTheDocument();
    expect(
      request.mock.calls.filter(([method]) => method === "POST"),
    ).toHaveLength(1);
  });

  it("resumes the Device it made after a reload, instead of making another", async () => {
    const { request } = stubAPI([], newDevice("Kindle", "kindle"));
    renderAt("/user/devices?connect=new-1");

    const dialog = await flow();
    expect(
      await within(dialog).findByRole("heading", { name: "Set up Kindle" }),
    ).toBeInTheDocument();
    expect(
      request.mock.calls.filter(([method]) => method === "POST"),
    ).toHaveLength(0);
  });

  it("says so when the Device it's setting up was deleted, and starts over", async () => {
    const { remove } = stubAPI([], newDevice("Kindle", "kindle"));
    remove();
    const user = setupUser();
    renderAt("/user/devices?connect=new-1");

    const dialog = await flow();
    expect(
      await within(dialog).findByText(/This Device was deleted/),
    ).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole("button", { name: "Start Over" }),
    );
    expect(
      await within(await flow()).findByText("What do you read on?"),
    ).toBeInTheDocument();
  });

  it("notices the device connected even when the event never arrived", async () => {
    const { connect } = stubAPI();
    const user = setupUser();
    renderAt("/user/devices?connect");

    const dialog = await choose(user, /A reading app/);
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    connect();
    await act(async () => {
      vi.advanceTimersByTime(5000);
    });

    expect(
      await within(dialog).findByRole("heading", { name: "Connected!" }),
    ).toBeInTheDocument();
  });

  describe("at phone width", () => {
    emulatePhoneWidth();

    it("names the Connect Device button", async () => {
      stubAPI([newDevice("Libra 2", "kobo", { id: "kobo-1" })]);
      renderAt("/user/devices");

      expect(
        await screen.findByRole("button", { name: "Connect Device" }),
      ).toBeInTheDocument();
    });
  });
});
