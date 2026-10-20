import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  DeliveryPreviewQuery,
  DeliveryPreviewResponse,
  Rule,
} from "@/types";
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

const basePreview: DeliveryPreviewResponse = {
  received_books: 1200,
  not_received: [{ source_file_type: "pdf", books: 2 }],
  moved_books: 0,
  retyped_books: 0,
};
// What the delivery preview answers, by its query.
let previewFor: (query: DeliveryPreviewQuery) => DeliveryPreviewResponse;

// Answers the delivery preview with previewFor, and PATCH the way the
// server does: following a list resolves it, a custom list is saved as
// sent, and an empty one is refused.
const mockServer = () =>
  vi
    .spyOn(API, "request")
    .mockImplementation(async (method, path, body, query) => {
      if (method === "GET" && path.endsWith("/delivery-preview")) {
        return previewFor((query ?? {}) as DeliveryPreviewQuery);
      }
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

const previews = (request: ReturnType<typeof mockServer>) =>
  request.mock.calls.filter((call) => call[0] === "GET").map((call) => call[3]);

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
  beforeEach(() => {
    previewFor = () => basePreview;
    mockServer();
  });
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
    ).toHaveAttribute("href", "/user/preferences#delivery-rules");

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

  it("previews what the Device receives", async () => {
    renderSection(kobo());

    const preview = await screen.findByLabelText("What this Device receives");
    await waitFor(() =>
      expect(preview).toHaveTextContent(
        "Receives 1,200 books. Doesn't receive 2 books with PDF.",
      ),
    );
  });

  it("previews a custom list while it is edited", async () => {
    const user = setup();
    const request = mockServer();
    previewFor = (query) =>
      query.mode === "custom"
        ? {
            ...basePreview,
            received_books: 7,
            not_received: [{ source_file_type: "cbz", books: 30 }],
          }
        : basePreview;
    renderSection(kobo());

    await user.click(modeButton("Custom"));
    await user.click(
      screen.getByRole("button", { name: "Remove the CBZ rule" }),
    );

    await waitFor(() =>
      expect(
        screen.getByLabelText("What this Device receives"),
      ).toHaveTextContent(
        "Receives 7 books. Doesn't receive 30 books with CBZ.",
      ),
    );
    expect(previews(request)).toContainEqual({
      mode: "custom",
      rules: "epub:kepub",
    });
  });

  it("warns before saving a switch between KePub and EPUB", async () => {
    const user = setup();
    const request = mockServer();
    previewFor = (query) =>
      query.mode === "default"
        ? { ...basePreview, retyped_books: 2 }
        : basePreview;
    renderSection(kobo());

    await user.click(modeButton("My default"));

    const dialog = await screen.findByRole("dialog", {
      name: "Switch books between KePub and EPUB?",
    });
    expect(dialog).toHaveTextContent(
      "2 books on this Kobo will switch between KePub and EPUB",
    );
    expect(dialog).toHaveTextContent("reading position and highlights");
    expect(patches(request)).toEqual([]);

    await user.click(
      within(dialog).getByRole("button", { name: "Save rules" }),
    );
    await waitFor(() =>
      expect(patches(request)).toEqual([
        { delivery_rules: { mode: "default", rules: undefined } },
      ]),
    );
  });

  it("saves nothing when the switch is canceled", async () => {
    const user = setup();
    const request = mockServer();
    previewFor = () => ({ ...basePreview, retyped_books: 1 });
    renderSection(kobo());

    await user.click(modeButton("Custom"));
    await user.click(
      screen.getByRole("button", { name: "Remove the CBZ rule" }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(patches(request)).toEqual([]);
  });

  it("never asks a Device that isn't a Kobo", async () => {
    const user = setup();
    const request = mockServer();
    previewFor = () => ({ ...basePreview, retyped_books: 3 });
    renderSection(
      kobo({
        device_type: "opds",
        has_preset: false,
        delivery_rules_mode: "default",
        delivery_rules: userDefault,
      }),
    );

    await user.click(modeButton("Custom"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patches(request)).toHaveLength(1));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("warns when books on the Kobo move to a different edition", async () => {
    previewFor = () => ({ ...basePreview, moved_books: 1 });
    renderSection(kobo());

    expect(
      await screen.findByText(
        "1 book on this Kobo will move to a different edition at its next sync, and may reopen at the start.",
      ),
    ).toBeInTheDocument();
  });

  it("asks before following rules that move books to a different edition", async () => {
    const user = setup();
    const request = mockServer();
    previewFor = (query) =>
      query.mode === "default"
        ? { ...basePreview, moved_books: 3 }
        : basePreview;
    renderSection(kobo());

    await user.click(modeButton("My default"));

    const dialog = await screen.findByRole("dialog", {
      name: "Move books to a different edition?",
    });
    expect(dialog).toHaveTextContent(
      "3 books on this Kobo will move to a different edition",
    );
    expect(patches(request)).toEqual([]);
    await user.click(
      within(dialog).getByRole("button", { name: "Save rules" }),
    );
    await waitFor(() => expect(patches(request)).toHaveLength(1));
  });
});
