import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import { LanguageCombobox } from "./LanguageCombobox";

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
  }),
}));

const onChange = vi.fn();

const Harness = () => {
  const [value, setValue] = useState("");
  return (
    <LanguageCombobox
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
    auth.permissions = new Set(["books:read", "books:write", "libraries:read"]);
  });

  it("suggests the library's own language tags for a role with Libraries Read", async () => {
    const request = vi.spyOn(API, "request").mockResolvedValue(["tlh"]);
    const user = userEvent.setup();
    renderCombobox();

    await waitFor(() => expect(languagesRequests(request)).toHaveLength(1));
    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText("Search languages..."), "tlh");

    expect(await screen.findByRole("option", { name: /tlh/ })).toBeVisible();
    expect(screen.queryByText(/Use custom tag/)).toBeNull();
  });

  it("requests no library languages for a role without Libraries Read and still accepts a typed tag", async () => {
    auth.permissions = new Set(["books:read", "books:write"]);
    const request = vi.spyOn(API, "request").mockResolvedValue(["tlh"]);
    const user = userEvent.setup();
    renderCombobox();

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText("Search languages..."), "tlh");
    await user.click(
      await screen.findByRole("option", { name: /Use custom tag: "tlh"/ }),
    );

    expect(onChange).toHaveBeenCalledWith("tlh");
    expect(languagesRequests(request)).toHaveLength(0);
  });
});
