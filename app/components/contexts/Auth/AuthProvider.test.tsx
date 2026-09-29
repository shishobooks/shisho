import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { useAuth } from "@/hooks/useAuth";
import { queryClient } from "@/libraries/query-client";

import AuthProvider from "./AuthProvider";

beforeAll(() => {
  vi.stubGlobal("__APP_VERSION__", "test");
});

afterEach(() => {
  vi.restoreAllMocks();
});

function stubAuth(me: Record<string, unknown> | null, demoMode = false) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = String(input).split("?")[0];
    if (path === "/api/auth/status")
      return Response.json({ needs_setup: false, demo_mode: demoMode });
    if (path === "/api/auth/me") {
      if (!me)
        return Response.json(
          { error: { code: "unauthorized", message: "no" } },
          { status: 401 },
        );
      return Response.json(me);
    }
    return Response.json({}, { status: 404 });
  });
}

function Probe() {
  const { isLoading, canWrite, hasPermission } = useAuth();
  if (isLoading) return <div>loading</div>;
  return (
    <ul>
      <li>books:{String(canWrite("books"))}</li>
      <li>series:{String(canWrite("series"))}</li>
      <li>people:{String(canWrite("people"))}</li>
      <li>books-read:{String(hasPermission("books", "read"))}</li>
    </ul>
  );
}

describe("AuthProvider canWrite", () => {
  it("is true only for resources whose write permission the user holds", async () => {
    stubAuth({
      id: 1,
      username: "ed",
      role_name: "editor",
      permissions: ["books:read", "books:write", "series:read"],
    });
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );
    expect(await screen.findByText("books:true")).toBeInTheDocument();
    expect(screen.getByText("series:false")).toBeInTheDocument();
    expect(screen.getByText("people:false")).toBeInTheDocument();
    expect(screen.getByText("books-read:true")).toBeInTheDocument();
  });

  it("is false for a read-only user", async () => {
    stubAuth({
      id: 2,
      username: "viewer",
      role_name: "viewer",
      permissions: ["books:read", "series:read", "people:read"],
    });
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );
    expect(await screen.findByText("books:false")).toBeInTheDocument();
    expect(screen.getByText("books-read:true")).toBeInTheDocument();
  });

  it("reflects role permissions only, even in Demo Mode", async () => {
    stubAuth(
      {
        id: 3,
        username: "demo",
        role_name: "editor",
        permissions: ["books:read", "books:write"],
      },
      true,
    );
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );
    expect(await screen.findByText("books:true")).toBeInTheDocument();
  });

  it("is false when signed out", async () => {
    stubAuth(null);
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );
    expect(await screen.findByText("books:false")).toBeInTheDocument();
    expect(screen.getByText("books-read:false")).toBeInTheDocument();
  });
});

describe("AuthProvider session changes", () => {
  function SessionProbe() {
    const { isLoading, login, logout } = useAuth();
    if (isLoading) return <div>loading</div>;
    return (
      <>
        <button onClick={() => void login("bea", "secret")}>Sign in</button>
        <button onClick={() => void logout()}>Sign out</button>
      </>
    );
  }

  // Cached queries belong to the previous user, such as their accessible
  // libraries, so the next user in the same tab must not see them.
  it.each(["Sign in", "Sign out"])(
    "clears cached queries on %s",
    async (button) => {
      stubAuth(null);
      vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
        const path = String(input).split("?")[0];
        if (path === "/api/auth/status")
          return Response.json({ needs_setup: false, demo_mode: false });
        if (path === "/api/auth/login")
          return Response.json({ id: 2, username: "bea", permissions: [] });
        if (path === "/api/auth/logout") return new Response(null);
        return Response.json(
          { error: { code: "unauthorized", message: "no" } },
          { status: 401 },
        );
      });
      queryClient.setQueryData(["UserLibraries"], [{ id: 1, name: "Old" }]);
      render(
        <AuthProvider>
          <SessionProbe />
        </AuthProvider>,
      );

      fireEvent.click(await screen.findByRole("button", { name: button }));

      await waitFor(() =>
        expect(queryClient.getQueryData(["UserLibraries"])).toBeUndefined(),
      );
    },
  );
});
