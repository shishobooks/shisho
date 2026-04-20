import { PluginDangerZone } from "./PluginDangerZone";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";

import { PluginStatusActive, type Plugin } from "@/types/generated/models";

const mockUninstallMutate = vi.fn();
const mockNavigate = vi.fn();

vi.mock("@/hooks/queries/plugins", () => ({
  useUninstallPlugin: () => ({
    mutate: mockUninstallMutate,
    isPending: false,
  }),
}));

vi.mock("react-router-dom", () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

const makePlugin = (overrides: Partial<Plugin> = {}): Plugin => ({
  auto_update: false,
  id: "my-plugin",
  installed_at: "2024-01-01T00:00:00Z",
  name: "My Plugin",
  scope: "shisho",
  status: PluginStatusActive,
  version: "1.0.0",
  ...overrides,
});

describe("PluginDangerZone", () => {
  it("renders nothing when canWrite is false", () => {
    const { container } = render(
      <PluginDangerZone canWrite={false} plugin={makePlugin()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders the uninstall button when canWrite is true", () => {
    render(<PluginDangerZone canWrite={true} plugin={makePlugin()} />);
    expect(
      screen.getByRole("button", { name: /uninstall/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/danger zone/i)).toBeInTheDocument();
  });

  it("opens the confirm dialog when the uninstall button is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<PluginDangerZone canWrite={true} plugin={makePlugin()} />);

    await user.click(screen.getByRole("button", { name: /uninstall/i }));

    await waitFor(() => {
      expect(
        screen.getByRole("dialog", { name: /uninstall plugin/i }),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText(/are you sure you want to uninstall "my plugin"/i),
    ).toBeInTheDocument();
  });

  it("fires the uninstall mutation with the plugin's scope and id on confirm", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockUninstallMutate.mockReset();
    render(
      <PluginDangerZone
        canWrite={true}
        plugin={makePlugin({ id: "my-plugin", scope: "shisho" })}
      />,
    );

    await user.click(screen.getByRole("button", { name: /uninstall/i }));
    // The confirm dialog also has an "Uninstall" button; pick the one inside
    // the dialog by scoping to the dialog.
    const dialog = await screen.findByRole("dialog", {
      name: /uninstall plugin/i,
    });
    await user.click(
      within(dialog).getByRole("button", { name: /^uninstall$/i }),
    );

    await waitFor(() => {
      expect(mockUninstallMutate).toHaveBeenCalledWith(
        { id: "my-plugin", scope: "shisho" },
        expect.any(Object),
      );
    });
  });

  it("navigates to /settings/plugins on a successful uninstall", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockUninstallMutate.mockReset();
    mockNavigate.mockReset();
    // Simulate the mutation calling its onSuccess callback.
    mockUninstallMutate.mockImplementation((_args, opts) => {
      opts?.onSuccess?.();
    });

    render(<PluginDangerZone canWrite={true} plugin={makePlugin()} />);

    await user.click(screen.getByRole("button", { name: /uninstall/i }));
    const dialog = await screen.findByRole("dialog", {
      name: /uninstall plugin/i,
    });
    await user.click(
      within(dialog).getByRole("button", { name: /^uninstall$/i }),
    );

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/settings/plugins");
    });
    expect(toast.success).toHaveBeenCalledWith("My Plugin uninstalled");
  });

  it("shows an error toast when the uninstall mutation fails", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockUninstallMutate.mockReset();
    mockUninstallMutate.mockImplementation((_args, opts) => {
      opts?.onError?.(new Error("boom"));
    });

    render(<PluginDangerZone canWrite={true} plugin={makePlugin()} />);

    await user.click(screen.getByRole("button", { name: /uninstall/i }));
    const dialog = await screen.findByRole("dialog", {
      name: /uninstall plugin/i,
    });
    await user.click(
      within(dialog).getByRole("button", { name: /^uninstall$/i }),
    );

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("boom");
    });
  });
});
