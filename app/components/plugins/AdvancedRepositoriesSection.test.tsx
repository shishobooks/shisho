import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";

import { AdvancedRepositoriesSection } from "./AdvancedRepositoriesSection";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const add = vi.hoisted(() => ({
  mutate: undefined as unknown as (...args: never[]) => void,
}));

vi.mock("@/hooks/queries/plugins", () => ({
  usePluginRepositories: () => ({ data: [], isLoading: false, error: null }),
  useAddRepository: () => ({ mutate: add.mutate, isPending: false }),
  useRemoveRepository: () => ({ mutate: vi.fn(), isPending: false }),
  useSyncRepository: () => ({ mutate: vi.fn(), isPending: false }),
}));

beforeEach(() => setAuth({ permissions: ["config:read", "config:write"] }));

describe("AdvancedRepositoriesSection", () => {
  it("enables Add only once both URL and scope are filled", async () => {
    add.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<AdvancedRepositoriesSection />);

    const addButton = screen.getByRole("button", { name: /add/i });
    expect(addButton).toBeDisabled();

    const scope = screen.getByPlaceholderText("my-scope");
    await user.type(scope, "mine");
    expect(addButton).toBeDisabled();

    await user.type(
      screen.getByPlaceholderText("https://example.com/plugins/index.json"),
      "https://example.com/index.json",
    );
    expect(addButton).toBeEnabled();

    await user.clear(scope);
    await user.type(scope, "   ");
    expect(addButton).toBeDisabled();
  });

  it("toasts when adding a repository fails", async () => {
    const error = vi.spyOn(toast, "error");
    add.mutate = rejectingMutate();
    const user = userEvent.setup();
    render(<AdvancedRepositoriesSection />);

    await user.type(
      screen.getByPlaceholderText("https://example.com/plugins/index.json"),
      "https://example.com/index.json",
    );
    await user.type(screen.getByPlaceholderText("my-scope"), "mine");
    await user.click(screen.getByRole("button", { name: /add/i }));

    expect(add.mutate).toHaveBeenCalled();
    expect(error).toHaveBeenCalledWith(REJECTION_MESSAGE, undefined);
  });
});
