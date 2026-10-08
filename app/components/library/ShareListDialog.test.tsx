import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import type { ListShare } from "@/types";

import { ShareListDialog } from "./ShareListDialog";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const share: ListShare = {
  id: 10,
  list_id: 7,
  user_id: 3,
  user: { id: 3, username: "carol" },
  permission: "viewer",
  created_at: "",
};

// Whether a share update is in flight, which locks every share action.
let updatePending = false;

const mutation = (isPending = false) => ({ mutateAsync: vi.fn(), isPending });

vi.mock("@/hooks/queries/lists", () => ({
  useList: () => ({ data: { id: 7, user_id: 1 }, isLoading: false }),
  useListShares: () => ({ data: [share], error: null, isLoading: false }),
  useCreateShare: () => mutation(),
  useUpdateShare: () => mutation(updatePending),
  useDeleteShare: () => mutation(),
}));

vi.mock("@/hooks/queries/users", () => ({
  useUserDirectory: () => ({
    data: [
      { id: 1, username: "owner" },
      { id: 2, username: "bob" },
      { id: 3, username: "carol" },
    ],
    error: null,
    isLoading: false,
  }),
}));

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const renderDialog = () => (
  <ShareListDialog
    listId={7}
    listName="Favorites"
    onOpenChange={vi.fn()}
    open
  />
);

const shareButton = () => screen.getByRole("button", { name: "Share" });

const pickUser = async (user: ReturnType<typeof createUser>) => {
  await user.click(
    screen.getByRole("combobox", { name: "User to share with" }),
  );
  await user.click(screen.getByRole("option", { name: "bob" }));
};

describe("ShareListDialog Share button", () => {
  beforeEach(() => {
    setAuth({ permissions: [], user: { id: 1 } });
    updatePending = false;
  });

  it("is disabled until a user is selected", async () => {
    const user = createUser();
    render(renderDialog());
    expect(shareButton()).toBeDisabled();

    await pickUser(user);

    expect(shareButton()).toBeEnabled();
  });

  it("is disabled while another share change is pending", async () => {
    const user = createUser();
    const { rerender } = render(renderDialog());
    await pickUser(user);
    expect(shareButton()).toBeEnabled();

    updatePending = true;
    rerender(renderDialog());

    expect(shareButton()).toBeDisabled();
  });
});

describe("ShareListDialog controls", () => {
  beforeEach(() => {
    setAuth({ permissions: [], user: { id: 1 } });
    updatePending = false;
  });

  it("names each picker, and each share's permission by its user", () => {
    render(renderDialog());

    expect(
      screen.getByRole("combobox", { name: "User to share with" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: "Permission for the new share" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: "Permission for carol" }),
    ).toHaveTextContent(/view/i);
  });
});
