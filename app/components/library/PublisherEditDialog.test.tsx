import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { PublisherEditDialog } from "./PublisherEditDialog";

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

// Mock useFormDialogClose hook
vi.mock("@/hooks/useFormDialogClose", () => ({
  useFormDialogClose: (
    _open: boolean,
    onOpenChange: (open: boolean) => void,
  ) => ({
    requestClose: () => onOpenChange(false),
  }),
}));

// Mock EntityCombobox to avoid complex dependency chain
vi.mock("@/components/common/EntityCombobox", () => ({
  EntityCombobox: () => <div data-testid="entity-combobox" />,
}));

describe("PublisherEditDialog", () => {
  const createQueryClient = () =>
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    entityName: "Foobar",
    aliases: [] as string[],
    onSave: vi.fn(),
    isPending: false,
    useParentSearch: vi.fn(),
  };

  describe("auto-alias on rename", () => {
    it("should auto-add initial name as alias when name changes", async () => {
      const user = createUser();
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <PublisherEditDialog {...defaultProps} />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Foobar")).toBeInTheDocument();
      });

      // Change name to "Foo"
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "Foo");

      // "Foobar" should auto-appear as an alias chip
      expect(screen.getByText("Foobar")).toBeInTheDocument();
    });

    it("should remove auto-added alias when name changes back to initial", async () => {
      const user = createUser();
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <PublisherEditDialog {...defaultProps} />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Foobar")).toBeInTheDocument();
      });

      // Change name to "Foo" — auto-alias appears
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "Foo");
      expect(screen.getByText("Foobar")).toBeInTheDocument();

      // Change name back to "Foobar" — auto-alias should disappear
      await user.clear(nameInput);
      await user.type(nameInput, "Foobar");

      const chips = screen.queryAllByRole("button", {
        name: /remove alias/i,
      });
      expect(chips).toHaveLength(0);
    });

    it("should not duplicate alias when initial name was already an existing alias", async () => {
      const user = createUser();
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <PublisherEditDialog {...defaultProps} aliases={["Foobar"]} />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText("Foobar")).toBeInTheDocument();
      });

      // Change name to "Foo" — should not add a duplicate
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "Foo");

      const chips = screen.getAllByRole("button", { name: /remove alias/i });
      expect(chips).toHaveLength(1);
    });

    it("should not remove pre-existing alias when name changes back to initial", async () => {
      const user = createUser();
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <PublisherEditDialog {...defaultProps} aliases={["Foobar"]} />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText("Foobar")).toBeInTheDocument();
      });

      // Change name to "Foo" then back to "Foobar"
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "Foo");
      await user.clear(nameInput);
      await user.type(nameInput, "Foobar");

      // Pre-existing alias should still be there
      expect(screen.getByText("Foobar")).toBeInTheDocument();
    });

    it("should include auto-added alias in save payload", async () => {
      const user = createUser();
      const onSave = vi.fn().mockResolvedValue(undefined);
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <PublisherEditDialog {...defaultProps} onSave={onSave} />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Foobar")).toBeInTheDocument();
      });

      // Change name
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "Foo");

      const saveButton = screen.getByRole("button", { name: /save/i });
      await user.click(saveButton);

      await waitFor(() => {
        expect(onSave).toHaveBeenCalledWith(
          expect.objectContaining({
            name: "Foo",
            aliases: ["Foobar"],
          }),
        );
      });
    });
  });
});
