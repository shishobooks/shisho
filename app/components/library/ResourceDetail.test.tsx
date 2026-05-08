import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { ResourceDetail } from "./ResourceDetail";

vi.mock("@/hooks/queries/libraries", () => ({
  useLibrary: () => ({ data: { name: "My Library" } }),
}));

// LibraryLayout pulls in TopNav which requires AuthProvider — mock it to
// render children directly so ResourceDetail tests focus on header/dialog logic.
vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

beforeAll(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
});

function wrap(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/libraries/1/genres/5"]}>
        {ui}
      </MemoryRouter>
    </QueryClientProvider>
  );
}

const defaultProps = {
  libraryId: "1",
  entityId: 5,
  entityType: "genre" as const,
  name: "Science Fiction",
  aliases: [] as string[],
  bookCount: 3,
  breadcrumbItems: [
    { label: "Genres", to: "/libraries/1/genres" },
    { label: "Science Fiction" },
  ],
  editConfig: {
    isPending: false,
    onSave: vi.fn(),
  },
  mergeConfig: {
    entities: [],
    isLoadingEntities: false,
    isPending: false,
    onMerge: vi.fn(),
    onSearch: vi.fn(),
  },
  deleteConfig: {
    isPending: false,
    onDelete: vi.fn(),
    disabled: false,
  },
};

describe("ResourceDetail", () => {
  it("renders the entity name as page heading", () => {
    render(wrap(<ResourceDetail {...defaultProps} />));
    expect(
      screen.getByRole("heading", { level: 1, name: "Science Fiction" }),
    ).toBeInTheDocument();
  });

  it("renders Edit, Merge, and Delete action buttons", () => {
    render(wrap(<ResourceDetail {...defaultProps} />));
    expect(screen.getByRole("button", { name: /Edit/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Merge/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Delete/ })).toBeInTheDocument();
  });

  it("hides Delete button when disabled", () => {
    render(
      wrap(
        <ResourceDetail
          {...defaultProps}
          deleteConfig={{ ...defaultProps.deleteConfig, disabled: true }}
        />,
      ),
    );
    expect(
      screen.queryByRole("button", { name: /Delete/ }),
    ).not.toBeInTheDocument();
  });

  it("renders aliases comma-separated without prefix", () => {
    render(
      wrap(
        <ResourceDetail
          {...defaultProps}
          aliases={["Sci-Fi", "SF", "Science-Fiction"]}
        />,
      ),
    );
    expect(screen.getByText("Sci-Fi, SF, Science-Fiction")).toBeInTheDocument();
  });

  it("does not render aliases section when empty", () => {
    render(wrap(<ResourceDetail {...defaultProps} aliases={[]} />));
    expect(screen.queryByText(/Aliases/)).not.toBeInTheDocument();
  });

  it("renders book count badge", () => {
    render(wrap(<ResourceDetail {...defaultProps} bookCount={7} />));
    expect(screen.getByText("7 books")).toBeInTheDocument();
  });

  it("renders singular book count", () => {
    render(wrap(<ResourceDetail {...defaultProps} bookCount={1} />));
    expect(screen.getByText("1 book")).toBeInTheDocument();
  });

  it("renders custom count label when provided", () => {
    render(
      wrap(
        <ResourceDetail
          {...defaultProps}
          bookCount={5}
          countLabel={{ singular: "file", plural: "files" }}
        />,
      ),
    );
    expect(screen.getByText("5 files")).toBeInTheDocument();
  });

  it("renders singular custom count label", () => {
    render(
      wrap(
        <ResourceDetail
          {...defaultProps}
          bookCount={1}
          countLabel={{ singular: "file", plural: "files" }}
        />,
      ),
    );
    expect(screen.getByText("1 file")).toBeInTheDocument();
  });

  it("renders children", () => {
    render(
      wrap(
        <ResourceDetail {...defaultProps}>
          <div data-testid="child-section">Custom Content</div>
        </ResourceDetail>,
      ),
    );
    expect(screen.getByTestId("child-section")).toBeInTheDocument();
  });

  it("opens edit dialog when Edit is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(wrap(<ResourceDetail {...defaultProps} />));
    await user.click(screen.getByRole("button", { name: /Edit/ }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Edit Genre")).toBeInTheDocument();
  });

  it("closes delete dialog after onDelete resolves", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onDelete = vi.fn().mockResolvedValue(undefined);
    render(
      wrap(
        <ResourceDetail
          {...defaultProps}
          deleteConfig={{ isPending: false, onDelete, disabled: false }}
        />,
      ),
    );
    await user.click(screen.getByRole("button", { name: /Delete/ }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    // Click the destructive Delete button inside the dialog
    const dialogButtons = screen.getAllByRole("button", { name: /Delete/ });
    const confirmButton = dialogButtons.find(
      (b) => b.closest("[role='dialog']") !== null,
    )!;
    await user.click(confirmButton);

    expect(onDelete).toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes merge dialog after onMerge resolves", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onMerge = vi.fn().mockResolvedValue(undefined);
    const entities = [
      { id: 10, name: "Fantasy", count: 5 },
      { id: 20, name: "Horror", count: 3 },
    ];
    render(
      wrap(
        <ResourceDetail
          {...defaultProps}
          mergeConfig={{
            entities,
            isLoadingEntities: false,
            isPending: false,
            onMerge,
            onSearch: vi.fn(),
          }}
        />,
      ),
    );
    await user.click(screen.getByRole("button", { name: /Merge/ }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    // Open the combobox and select an entity
    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Fantasy"));

    // Click the Merge confirm button
    const mergeButton = screen
      .getAllByRole("button", { name: /Merge/ })
      .find((b) => b.closest("[role='dialog']") !== null)!;
    await user.click(mergeButton);

    expect(onMerge).toHaveBeenCalledWith(10);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
