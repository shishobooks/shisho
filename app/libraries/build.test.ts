import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "./api";
import {
  BUILD_HEADER,
  checkServerBuild,
  isNewBuildOut,
  noteServerBuild,
  subscribeToServerBuild,
} from "./build";

const withBuild = (id: string | null) =>
  new Response(null, { headers: id ? { [BUILD_HEADER]: id } : {} });

const setRunningBuild = (id: string | null) => {
  document.head
    .querySelectorAll('meta[name="shisho-build"]')
    .forEach((meta) => meta.remove());
  if (id) {
    const meta = document.createElement("meta");
    meta.name = "shisho-build";
    meta.content = id;
    document.head.append(meta);
  }
};

describe("build", () => {
  beforeEach(() => {
    setRunningBuild("old");
    // The store is module state; seeing the running build resets it.
    noteServerBuild(withBuild("old"));
  });

  afterEach(() => {
    setRunningBuild(null);
    vi.restoreAllMocks();
  });

  it("is not out while the server sends the build this page loaded", () => {
    expect(isNewBuildOut()).toBe(false);
  });

  it("is out once the server sends a different build", () => {
    const listener = vi.fn();
    const unsubscribe = subscribeToServerBuild(listener);
    noteServerBuild(withBuild("new"));
    expect(isNewBuildOut()).toBe(true);
    expect(listener).toHaveBeenCalledTimes(1);
    unsubscribe();
  });

  it("ignores responses without the header", () => {
    noteServerBuild(withBuild("new"));
    noteServerBuild(withBuild(null));
    expect(isNewBuildOut()).toBe(true);
  });

  it("is never out without a build of its own, as under the Vite dev server", () => {
    setRunningBuild(null);
    noteServerBuild(withBuild("new"));
    expect(isNewBuildOut()).toBe(false);
  });

  it("learns the server's build from every API response", async () => {
    await API.checkStatus(
      new Response("{}", {
        headers: { [BUILD_HEADER]: "new", "content-type": "application/json" },
      }),
    );
    expect(isNewBuildOut()).toBe(true);
  });

  it("learns the server's build from a failed API response too", async () => {
    await API.checkStatus(
      new Response("{}", {
        status: 500,
        headers: { [BUILD_HEADER]: "new", "content-type": "application/json" },
      }),
    ).catch(() => undefined);
    expect(isNewBuildOut()).toBe(true);
  });

  it("asks the server for its build on demand", async () => {
    const fetchSpy = vi
      .spyOn(window, "fetch")
      .mockResolvedValue(withBuild("new"));
    await checkServerBuild();
    expect(fetchSpy).toHaveBeenCalledWith("/api/auth/status", {
      cache: "no-store",
    });
    expect(isNewBuildOut()).toBe(true);
  });

  it("does not ask without a build of its own", async () => {
    setRunningBuild(null);
    const fetchSpy = vi.spyOn(window, "fetch");
    await checkServerBuild();
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("stays quiet when the check cannot reach the server", async () => {
    vi.spyOn(window, "fetch").mockRejectedValue(new TypeError("offline"));
    await expect(checkServerBuild()).resolves.toBeUndefined();
    expect(isNewBuildOut()).toBe(false);
  });
});
