import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Rule } from "@/types";
import type {
  DeviceResponse,
  UpdateDevicePayload,
} from "@/types/generated/devices";

import DeviceRulesSection from "./DeviceRulesSection";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const preset: Rule[] = [
  { source_file_type: "epub", delivered_file_type: "kepub" },
  { source_file_type: "cbz", delivered_file_type: "kepub" },
];
// The user's default, without the rules a Kobo can't receive.
const userDefault: Rule[] = [
  { source_file_type: "epub", delivered_file_type: "epub" },
];

const kobo = (overrides: Partial<DeviceResponse> = {}): DeviceResponse => ({
  id: "kobo-1",
  user_id: 1,
  name: "Libra 2",
  device_type: "kobo",
  scope_type: "all",
  url_scope_pending: false,
  delivery_rules_mode: "preset",
  delivery_rules: preset,
  delivery_rule_options: [
    { source_file_type: "epub", delivered_file_types: ["epub", "kepub"] },
    { source_file_type: "cbz", delivered_file_types: ["kepub", "epub"] },
  ],
  has_preset: true,
  credential: "abcdefghjkmnpqrs",
  connections: ["kobo_sync", "ereader_browser"],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  ...overrides,
});

// Answers PATCH the way the server does: following a list resolves it, a
// custom list is saved as sent, and an empty one is refused.
const mockServer = () =>
  vi.spyOn(API, "request").mockImplementation(async (_method, _path, body) => {
    const rules = (body as UpdateDevicePayload).delivery_rules!;
    if (rules.mode === "custom" && !rules.rules?.length) {
      throw new ShishoAPIError(
        "Delivery Rules need at least one rule.",
        "validation_error",
        422,
      );
    }
    return kobo({
      delivery_rules_mode: rules.mode,
      delivery_rules:
        rules.mode === "custom"
          ? rules.rules!
          : rules.mode === "preset"
            ? preset
            : userDefault,
    });
  });

// Renders the section the way the Device page does, re-rendering with each
// saved Device as the page's query would.
const renderSection = (initial: DeviceResponse) => {
  const onHasChangesChange = vi.fn();
  const queryClient = new QueryClient();
  const view = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DeviceRulesSection
          device={initial}
          onHasChangesChange={onHasChangesChange}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  const rerender = (device: DeviceResponse) =>
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <DeviceRulesSection
            device={device}
            onHasChangesChange={onHasChangesChange}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  return { onHasChangesChange, rerender };
};

const patches = (request: ReturnType<typeof mockServer>) =>
  request.mock.calls
    .filter((call) => call[0] === "PATCH")
    .map((call) => call[2]);

const modeButton = (name: string) =>
  within(screen.getByRole("group", { name: "Delivery Rules" })).getByRole(
    "button",
    { name },
  );

const setup = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

describe("DeviceRulesSection", () => {
  afterEach(() => vi.restoreAllMocks());

  it("shows the preset a new Kobo follows, read-only", () => {
    renderSection(kobo());

    expect(modeButton("Kobo preset")).toHaveAttribute("aria-pressed", "true");
    const list = screen.getByRole("list", {
      name: "Rules this Device follows",
    });
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((li) => li.getAttribute("aria-label")),
    ).toEqual(["EPUB as KePub", "CBZ as KePub"]);
    expect(
      screen.queryByRole("link", { name: "Edit your default" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("follows the user's default in one click", async () => {
    const user = setup();
    const request = mockServer();
    renderSection(kobo());

    await user.click(modeButton("My default"));

    await waitFor(() =>
      expect(patches(request)).toEqual([
        { delivery_rules: { mode: "default", rules: undefined } },
      ]),
    );
  });

  it("follows the preset again from the default", async () => {
    const user = setup();
    const request = mockServer();
    renderSection(
      kobo({ delivery_rules_mode: "default", delivery_rules: userDefault }),
    );

    // Following the default links to where it's edited.
    expect(
      screen.getByRole("link", { name: "Edit your default" }),
    ).toHaveAttribute("href", "/user/settings#delivery-rules");

    expect(modeButton("My default")).toHaveAttribute("aria-pressed", "true");
    await user.click(modeButton("Kobo preset"));

    await waitFor(() =>
      expect(patches(request)).toEqual([
        { delivery_rules: { mode: "preset", rules: undefined } },
      ]),
    );
  });

  it("offers no preset for a type without one", () => {
    renderSection(
      kobo({
        device_type: "opds",
        has_preset: false,
        delivery_rules_mode: "default",
        delivery_rules: userDefault,
      }),
    );

    expect(
      within(screen.getByRole("group", { name: "Delivery Rules" }))
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["My default", "Custom"]);
  });

  it("customizes from a copy of what it follows and saves it only on Save", async () => {
    const user = setup();
    const request = mockServer();
    const { onHasChangesChange, rerender } = renderSection(kobo());

    await user.click(modeButton("Custom"));
    expect(patches(request)).toEqual([]);
    expect(onHasChangesChange).toHaveBeenLastCalledWith(true);
    expect(
      screen.getByRole("button", { name: "Remove the EPUB rule" }),
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "Remove the CBZ rule" }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(patches(request)).toEqual([
        {
          delivery_rules: {
            mode: "custom",
            rules: [{ source_file_type: "epub", delivered_file_type: "kepub" }],
          },
        },
      ]),
    );
    rerender(
      kobo({
        delivery_rules_mode: "custom",
        delivery_rules: [
          { source_file_type: "epub", delivered_file_type: "kepub" },
        ],
      }),
    );
    expect(modeButton("Custom")).toHaveAttribute("aria-pressed", "true");
    await waitFor(() =>
      expect(onHasChangesChange).toHaveBeenLastCalledWith(false),
    );
  });

  it("offers only the File Types the Device's type can receive", async () => {
    const user = setup();
    renderSection(kobo());

    await user.click(modeButton("Custom"));
    await user.click(screen.getByRole("combobox", { name: "Deliver CBZ as" }));

    expect(
      screen.getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["KePub", "EPUB"]);
  });

  it("won't save an empty custom list", async () => {
    const user = setup();
    const request = mockServer();
    renderSection(kobo());

    await user.click(modeButton("Custom"));
    await user.click(
      screen.getByRole("button", { name: "Remove the EPUB rule" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Remove the CBZ rule" }),
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Add at least one rule.",
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(patches(request)).toEqual([]);
  });

  it("cancels customizing without saving", async () => {
    const user = setup();
    const request = mockServer();
    renderSection(kobo());

    await user.click(modeButton("Custom"));
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(modeButton("Kobo preset")).toHaveAttribute("aria-pressed", "true");
    expect(patches(request)).toEqual([]);
  });

  it("resets a custom list by following the preset again", async () => {
    const user = setup();
    const request = mockServer();
    const custom: Rule[] = [
      { source_file_type: "epub", delivered_file_type: "epub" },
    ];
    const { rerender } = renderSection(
      kobo({ delivery_rules_mode: "custom", delivery_rules: custom }),
    );

    expect(modeButton("Custom")).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.getByRole("button", { name: "Remove the EPUB rule" }),
    ).toBeInTheDocument();

    await user.click(modeButton("Kobo preset"));
    await waitFor(() =>
      expect(patches(request)).toEqual([
        { delivery_rules: { mode: "preset", rules: undefined } },
      ]),
    );
    rerender(kobo());

    expect(modeButton("Kobo preset")).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.queryByRole("button", { name: "Remove the EPUB rule" }),
    ).toBeNull();
  });

  it("warns when following the default delivers nothing", () => {
    renderSection(
      kobo({
        device_type: "kindle",
        delivery_rules_mode: "default",
        delivery_rules: [],
      }),
    );

    expect(screen.getByText(/so it receives nothing/)).toBeInTheDocument();
  });
});
