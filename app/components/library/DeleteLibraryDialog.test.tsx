import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";

import { DeleteLibraryDialog } from "./DeleteLibraryDialog";

const mockDelete = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/queries/libraries", () => ({
  useDeleteLibrary: () => ({
    mutateAsync: mockDelete,
    isPending: false,
  }),
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

const renderDialog = (
  props: Partial<React.ComponentProps<typeof DeleteLibraryDialog>> = {},
) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DeleteLibraryDialog
          library={{ id: 1, name: "My Library" }}
          onOpenChange={onOpenChange}
          open={true}
          {...props}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { onOpenChange };
};

describe("DeleteLibraryDialog", () => {
  it("disables the Delete button until the user types the exact library name", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDialog();

    const deleteButton = screen.getByRole("button", { name: /^Delete$/ });
    expect(deleteButton).toBeDisabled();

    const input = screen.getByLabelText(/Type the library name to confirm/i);
    await user.type(input, "my library"); // wrong case
    expect(deleteButton).toBeDisabled();

    await user.clear(input);
    await user.type(input, "My Library");
    expect(deleteButton).toBeEnabled();
  });

  it("calls the delete mutation with the library id on confirm", async () => {
    mockDelete.mockResolvedValueOnce(undefined);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDialog();

    await user.type(
      screen.getByLabelText(/Type the library name to confirm/i),
      "My Library",
    );
    await user.click(screen.getByRole("button", { name: /^Delete$/ }));

    expect(mockDelete).toHaveBeenCalledWith({ id: 1 });
  });

  it("keeps the dialog open and shows an error toast when the mutation fails", async () => {
    mockDelete.mockRejectedValueOnce(new Error("server exploded"));
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onOpenChange } = renderDialog();

    await user.type(
      screen.getByLabelText(/Type the library name to confirm/i),
      "My Library",
    );
    await user.click(screen.getByRole("button", { name: /^Delete$/ }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("server exploded");
    });
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });

  it("surfaces the three caveats in the warning banner", () => {
    renderDialog();

    expect(screen.getByText(/irreversible/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Files on disk will not be deleted/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Sidecar and metadata files will not be cleaned up/i),
    ).toBeInTheDocument();
  });
});
