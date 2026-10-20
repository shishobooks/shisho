import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryKey as DevicesQueryKey } from "@/hooks/queries/devices";
import { API } from "@/libraries/api";
import type { DefaultRulesResponse, Rule } from "@/types";

import DeliveryRulesEditor from "./DeliveryRulesEditor";

const builtIn: Rule[] = [
  { source_file_type: "epub", delivered_file_type: "epub" },
  { source_file_type: "cbz", delivered_file_type: "cbz" },
  { source_file_type: "m4b", delivered_file_type: "m4b" },
];

const options: DefaultRulesResponse["options"] = [
  { source_file_type: "epub", delivered_file_types: ["epub", "kepub"] },
  { source_file_type: "azw3", delivered_file_types: ["azw3"] },
  { source_file_type: "cbz", delivered_file_types: ["cbz", "kepub", "epub"] },
  { source_file_type: "m4b", delivered_file_types: ["m4b"] },
];

const response = (
  rules: Rule[],
  followsBuiltIn = false,
): DefaultRulesResponse => ({
  follows_built_in: followsBuiltIn,
  rules,
  options,
});

// Serves GET from `current` and answers PUT and DELETE the way the server
// does, recording what was sent.
const mockServer = (initial: DefaultRulesResponse) => {
  let current = initial;
  return vi
    .spyOn(API, "request")
    .mockImplementation(async (method, _path, body) => {
      if (method === "PUT") {
        current = response((body as { rules: Rule[] }).rules);
      } else if (method === "DELETE") {
        current = response(builtIn, true);
      }
      return current;
    });
};

const renderEditor = (
  onHasChangesChange = vi.fn(),
  queryClient = new QueryClient(),
) => {
  render(
    <QueryClientProvider client={queryClient}>
      <DeliveryRulesEditor onHasChangesChange={onHasChangesChange} />
    </QueryClientProvider>,
  );
  return { onHasChangesChange };
};

// The rule rows' source File Types, in order.
const ruleOrder = () =>
  screen
    .getAllByRole("button", { name: /^Remove the .* rule$/ })
    .map((button) =>
      button
        .getAttribute("aria-label")!
        .replace(/^Remove the (.*) rule$/, "$1"),
    );

const setup = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

describe("DeliveryRulesEditor", () => {
  afterEach(() => vi.restoreAllMocks());

  it("starts from the built-in rules and saves an edited copy as the user's own", async () => {
    const user = setup();
    const request = mockServer(response(builtIn, true));
    const { onHasChangesChange } = renderEditor();

    expect(
      await screen.findByText("Following Shisho's defaults"),
    ).toBeInTheDocument();
    expect(ruleOrder()).toEqual(["EPUB", "CBZ", "M4B"]);
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    await user.click(
      screen.getByRole("button", { name: "Move the CBZ rule up" }),
    );
    expect(ruleOrder()).toEqual(["CBZ", "EPUB", "M4B"]);
    expect(onHasChangesChange).toHaveBeenLastCalledWith(true);

    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(request).toHaveBeenCalledWith("PUT", "/user/delivery-rules", {
      rules: [builtIn[1], builtIn[0], builtIn[2]],
    });
    expect(await screen.findByText("Customized")).toBeInTheDocument();
    expect(onHasChangesChange).toHaveBeenLastCalledWith(false);
  });

  it("refetches Devices, which show the default they follow, after saving or resetting", async () => {
    const user = setup();
    mockServer(response(builtIn, true));
    const queryClient = new QueryClient();
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    renderEditor(vi.fn(), queryClient);

    await user.click(
      await screen.findByRole("button", { name: "Move the CBZ rule up" }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith({
        queryKey: [DevicesQueryKey.RetrieveDevice],
      }),
    );
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: [DevicesQueryKey.ListDevices],
    });

    invalidate.mockClear();
    await user.click(
      await screen.findByRole("button", { name: "Reset to Shisho's defaults" }),
    );
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith({
        queryKey: [DevicesQueryKey.RetrieveDevice],
      }),
    );
  });

  it("offers each source only the File Types it can become", async () => {
    const user = setup();
    const request = mockServer(response(builtIn, true));
    renderEditor();

    await user.click(
      await screen.findByRole("combobox", { name: "Deliver CBZ as" }),
    );
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["CBZ", "KePub", "EPUB"]);
    await user.click(screen.getByRole("option", { name: "EPUB" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(request).toHaveBeenCalledWith("PUT", "/user/delivery-rules", {
      rules: [
        builtIn[0],
        { source_file_type: "cbz", delivered_file_type: "epub" },
        builtIn[2],
      ],
    });
  });

  it("removes a rule and adds one back for an unlisted File Type", async () => {
    const user = setup();
    mockServer(response(builtIn, true));
    renderEditor();

    await user.click(
      await screen.findByRole("button", { name: "Remove the M4B rule" }),
    );
    expect(ruleOrder()).toEqual(["EPUB", "CBZ"]);

    await user.click(screen.getByRole("combobox", { name: "Add a rule" }));
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["AZW3", "M4B"]);
    await user.click(screen.getByRole("option", { name: "AZW3" }));
    expect(ruleOrder()).toEqual(["EPUB", "CBZ", "AZW3"]);
  });

  it("refuses to save an empty list", async () => {
    const user = setup();
    mockServer(response([builtIn[0]]));
    renderEditor();

    await user.click(
      await screen.findByRole("button", { name: "Remove the EPUB rule" }),
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Add at least one rule",
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("resets a customized list to follow the built-in rules", async () => {
    const user = setup();
    const request = mockServer(
      response([{ source_file_type: "cbz", delivered_file_type: "epub" }]),
    );
    renderEditor();

    expect(await screen.findByText("Customized")).toBeInTheDocument();
    expect(ruleOrder()).toEqual(["CBZ"]);

    await user.click(
      screen.getByRole("button", { name: "Reset to Shisho's defaults" }),
    );

    expect(request).toHaveBeenCalledWith("DELETE", "/user/delivery-rules");
    await waitFor(() => expect(ruleOrder()).toEqual(["EPUB", "CBZ", "M4B"]));
    expect(screen.getByText("Following Shisho's defaults")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Reset to Shisho's defaults" }),
    ).not.toBeInTheDocument();
  });

  it("keeps the edits when a save fails", async () => {
    const user = setup();
    vi.spyOn(API, "request").mockImplementation(async (method) => {
      if (method === "PUT") throw new Error("boom");
      return response(builtIn, true);
    });
    renderEditor();

    await user.click(
      await screen.findByRole("button", { name: "Move the M4B rule up" }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(ruleOrder()).toEqual(["EPUB", "M4B", "CBZ"]);
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });
});
