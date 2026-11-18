import {
  QueryClient,
  QueryClientProvider,
  useMutation,
} from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  createMemoryRouter,
  Link,
  Outlet,
  RouterProvider,
} from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { BUILD_HEADER, noteServerBuild, reloadPage } from "@/libraries/build";

import { NewBuildBanner, ReloadOnNewBuild } from "./NewBuild";

vi.mock("@/libraries/build", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/libraries/build")>()),
  reloadPage: vi.fn(),
}));

const serverSends = (id: string) =>
  act(() =>
    noteServerBuild(new Response(null, { headers: { [BUILD_HEADER]: id } })),
  );

let finishSave: () => void = () => undefined;

const SavingPage = () => {
  const save = useMutation({
    mutationFn: () =>
      new Promise<void>((resolve) => {
        finishSave = resolve;
      }),
  });
  return (
    <>
      <button onClick={() => save.mutate()} type="button">
        Save
      </button>
      <Link to="/other">Other</Link>
    </>
  );
};

const renderApp = (path = "/") => {
  const router = createMemoryRouter(
    [
      {
        path: "/",
        element: (
          <>
            <ReloadOnNewBuild />
            <NewBuildBanner />
            <Outlet />
          </>
        ),
        children: [
          { index: true, element: <SavingPage /> },
          { path: "other", element: <Link to="/">Home</Link> },
          {
            path: "libraries/:libraryId/books/:bookId/files/:fileId/read",
            element: <p>reader</p>,
            handle: { reader: true },
          },
        ],
      },
    ],
    { initialEntries: [path] },
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
};

describe("new build", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    const meta = document.createElement("meta");
    meta.name = "shisho-build";
    meta.content = "old";
    document.head.append(meta);
    noteServerBuild(new Response(null, { headers: { [BUILD_HEADER]: "old" } }));
  });

  afterEach(() => {
    document.head
      .querySelectorAll('meta[name="shisho-build"]')
      .forEach((meta) => meta.remove());
    vi.mocked(reloadPage).mockClear();
    vi.restoreAllMocks();
  });

  it("shows no banner while the tab runs the server's build", () => {
    renderApp();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("offers a reload once the server has a newer build", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderApp();
    await serverSends("new");
    expect(screen.getByRole("status")).toHaveTextContent(
      "Shisho was updated. Reload to get the new version.",
    );
    await user.click(screen.getByRole("button", { name: "Reload" }));
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  it("keeps the banner out of the reader", async () => {
    renderApp("/libraries/1/books/2/files/3/read");
    await serverSends("new");
    expect(screen.getByText("reader")).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("does a full page load at the next navigation instead of the in-app one", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderApp();
    await serverSends("new");
    expect(reloadPage).not.toHaveBeenCalled();
    await user.click(screen.getByRole("link", { name: "Other" }));
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  it("navigates in-app while the tab runs the server's build", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderApp();
    await user.click(screen.getByRole("link", { name: "Other" }));
    expect(screen.getByRole("link", { name: "Home" })).toBeInTheDocument();
    expect(reloadPage).not.toHaveBeenCalled();
  });

  it("stops reloading on its own when a reload did not reach the new build", async () => {
    // A proxy caching index.html serves the old page again. Reloading at
    // every navigation, or forever on a page that redirects on load, helps
    // nobody; the banner stays.
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    window.sessionStorage.setItem("shisho-new-build-reload", "new");
    renderApp();
    await serverSends("new");
    await user.click(screen.getByRole("link", { name: "Other" }));
    expect(reloadPage).not.toHaveBeenCalled();
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("reloads again for a build newer than the one it last reloaded for", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    window.sessionStorage.setItem("shisho-new-build-reload", "new");
    renderApp();
    await serverSends("newer");
    await user.click(screen.getByRole("link", { name: "Other" }));
    expect(reloadPage).toHaveBeenCalledTimes(1);
    expect(window.sessionStorage.getItem("shisho-new-build-reload")).toBe(
      "newer",
    );
  });

  it("lets a save in flight finish before reloading", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderApp();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await serverSends("new");
    await user.click(screen.getByRole("link", { name: "Other" }));
    expect(reloadPage).not.toHaveBeenCalled();
    await act(async () => {
      finishSave();
      // React Query batches the settled mutation's notification on a timer.
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  it("checks the server's build when the tab comes back into view", async () => {
    const fetchSpy = vi
      .spyOn(window, "fetch")
      .mockResolvedValue(
        new Response(null, { headers: { [BUILD_HEADER]: "new" } }),
      );
    renderApp();
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
    });
    expect(fetchSpy).toHaveBeenCalledWith("/api/auth/status", {
      cache: "no-store",
    });
    expect(screen.getByRole("status")).toBeInTheDocument();
  });
});
