import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import { LanguageCombobox } from "./LanguageCombobox";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const onChange = vi.fn();

const Harness = () => {
  const [value, setValue] = useState("");
  return (
    <LanguageCombobox
      label="Language"
      libraryId={5}
      onChange={(next) => {
        onChange(next);
        setValue(next);
      }}
      value={value}
    />
  );
};

const renderCombobox = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <Harness />
    </QueryClientProvider>,
  );
};

const languagesRequests = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.filter(
    (call: unknown[]) => call[1] === "/libraries/5/languages",
  );

describe("LanguageCombobox", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    onChange.mockClear();
    setAuth({ permissions: ["books:read", "books:write"] });
  });

  it("names the trigger and its search box by the field, not the prompt shown", async () => {
    vi.spyOn(API, "request").mockResolvedValue([]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderCombobox();

    const trigger = screen.getByRole("combobox", { name: "Language" });
    expect(trigger).toHaveTextContent("Select language...");
    await user.click(trigger);
    expect(
      screen.getByRole("combobox", { name: "Search languages" }),
    ).toBeInTheDocument();
  });

  it("suggests the library's own language tags for a role with Books Read, without Libraries Read", async () => {
    const request = vi.spyOn(API, "request").mockResolvedValue(["tlh"]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderCombobox();

    await waitFor(() => expect(languagesRequests(request)).toHaveLength(1));
    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText("Search languages..."), "tlh");

    expect(await screen.findByRole("option", { name: /tlh/ })).toBeVisible();
    expect(screen.queryByText(/Use custom tag/)).toBeNull();
  });

  it("requests no library languages for a role without Books Read and still accepts a typed tag", async () => {
    setAuth({ permissions: ["books:write"] });
    const request = vi.spyOn(API, "request").mockResolvedValue(["tlh"]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderCombobox();

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText("Search languages..."), "tlh");
    await user.click(
      await screen.findByRole("option", { name: /Use custom tag: "tlh"/ }),
    );

    expect(onChange).toHaveBeenCalledWith("tlh");
    expect(languagesRequests(request)).toHaveLength(0);
  });

  it("reopens the picker from the keyboard through the chosen language", async () => {
    vi.spyOn(API, "request").mockResolvedValue([]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LanguageCombobox
          label="Language"
          libraryId={5}
          onChange={onChange}
          value="en"
        />
      </QueryClientProvider>,
    );

    const badge = screen.getByRole("button", {
      name: "Change language: English (en)",
    });
    await user.tab();
    expect(badge).toHaveFocus();
    await user.keyboard("{Enter}");

    const search = await screen.findByPlaceholderText("Search languages...");
    expect(search).toBeVisible();

    // Closing returns focus to the language, not to the page body.
    await user.keyboard("{Escape}");
    await waitFor(() => expect(search).not.toBeInTheDocument());
    expect(badge).toHaveFocus();
  });

  it("says which language is chosen in the open list, not only with a check icon", async () => {
    vi.spyOn(API, "request").mockResolvedValue([]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LanguageCombobox
          label="Language"
          libraryId={5}
          onChange={onChange}
          value="en"
        />
      </QueryClientProvider>,
    );

    await user.click(
      screen.getByRole("button", { name: "Change language: English (en)" }),
    );
    await screen.findAllByRole("option");
    const chosen = screen
      .getAllByRole("option")
      .filter((option) => option.textContent?.includes("(chosen)"));
    expect(chosen.map((option) => option.textContent)).toEqual([
      "English(chosen)en",
    ]);
  });
});
