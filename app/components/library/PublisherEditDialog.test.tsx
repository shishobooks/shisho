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

  describe("pending alias input on save (no Enter)", () => {
    it("should include pending alias text in save payload without pressing Enter", async () => {
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

      // Type alias without pressing Enter
      const aliasInput = screen.getByPlaceholderText(
        "Type alias and press Enter",
      );
      await user.type(aliasInput, "Foo Publishing");

      // Save should be enabled
      const saveButton = screen.getByRole("button", { name: /save/i });
      await waitFor(() => {
        expect(saveButton).not.toBeDisabled();
      });

      await user.click(saveButton);

      await waitFor(() => {
        expect(onSave).toHaveBeenCalledWith(
          expect.objectContaining({
            name: "Foobar",
            aliases: ["Foo Publishing"],
          }),
        );
      });
    });

    it("should include pending alias alongside existing aliases on save", async () => {
      const user = createUser();
      const onSave = vi.fn().mockResolvedValue(undefined);
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <PublisherEditDialog
            {...defaultProps}
            aliases={["Foo Press"]}
            onSave={onSave}
          />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText("Foo Press")).toBeInTheDocument();
      });

      // Type another alias without pressing Enter
      const aliasInput = screen.getByPlaceholderText("Add another...");
      await user.type(aliasInput, "Foo Publishing");

      const saveButton = screen.getByRole("button", { name: /save/i });
      await waitFor(() => {
        expect(saveButton).not.toBeDisabled();
      });
      await user.click(saveButton);

      await waitFor(() => {
        expect(onSave).toHaveBeenCalledWith(
          expect.objectContaining({
            aliases: ["Foo Press", "Foo Publishing"],
          }),
        );
      });
    });

    it("should not duplicate existing alias on save (case-insensitive)", async () => {
      const user = createUser();
      const onSave = vi.fn().mockResolvedValue(undefined);
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <PublisherEditDialog
            {...defaultProps}
            aliases={["Foo Press"]}
            onSave={onSave}
          />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText("Foo Press")).toBeInTheDocument();
      });

      // Type duplicate alias (different case) without Enter
      const aliasInput = screen.getByPlaceholderText("Add another...");
      await user.type(aliasInput, "foo press");

      // Change name to make save possible
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "FooBar Inc");

      const saveButton = screen.getByRole("button", { name: /save/i });
      await user.click(saveButton);

      await waitFor(() => {
        expect(onSave).toHaveBeenCalledWith(
          expect.objectContaining({
            aliases: expect.arrayContaining(["Foo Press"]),
          }),
        );
        // Should not have duplicated
        const callAliases = onSave.mock.calls[0][0].aliases;
        expect(
          callAliases.filter((a: string) => a.toLowerCase() === "foo press"),
        ).toHaveLength(1);
      });
    });

    it("should enable Save when only pending alias input is the change", async () => {
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

      // Save should be disabled initially
      const saveButton = screen.getByRole("button", { name: /save/i });
      expect(saveButton).toBeDisabled();

      // Type alias text without Enter
      const aliasInput = screen.getByPlaceholderText(
        "Type alias and press Enter",
      );
      await user.type(aliasInput, "Foo Publishing");

      // Save should now be enabled
      await waitFor(() => {
        expect(saveButton).not.toBeDisabled();
      });
    });
  });

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
