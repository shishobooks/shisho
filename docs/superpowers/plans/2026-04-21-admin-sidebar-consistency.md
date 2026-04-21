# Admin Sidebar & Top Nav Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring admin-page sidebar and top nav in line with the library-page versions, extracting shared pieces into single-source components so they cannot drift again.

**Architecture:** Extract a shared `<Sidebar>` primitive that owns the collapse/localStorage/styling/footer chrome, with thin area-specific wrappers (`LibrarySidebar`, `AdminSidebar`) that only compute nav items. Extract a shared `<UserMenu>` used by both top navs. Share top-nav container classes via `cn()`-wrapped constants rather than a full shell component (library top nav has too much area-specific behavior to slot cleanly). Remove the admin page's "Back to Library" link and mobile horizontal nav strip; the mobile drawer takes over mobile nav (with a small change to show admin items when on `/settings/*` routes).

**Tech Stack:** React 19, TypeScript, TailwindCSS, Radix UI (via shadcn/ui), Vitest + React Testing Library, React Router v6.

---

## File Structure

**Create:**
- `app/components/layout/Sidebar.tsx` — shared sidebar primitive
- `app/components/layout/Sidebar.test.tsx` — component tests
- `app/components/layout/UserMenu.tsx` — shared user dropdown
- `app/components/layout/topNavClasses.ts` — shared container classes
- `app/components/pages/AdminSidebar.tsx` — admin thin wrapper

**Modify:**
- `app/components/library/LibrarySidebar.tsx` — becomes thin wrapper over `<Sidebar>`
- `app/components/library/TopNav.tsx` — use `<UserMenu>` and class constants
- `app/components/library/MobileDrawer.tsx` — show admin nav items when on `/settings/*`
- `app/components/pages/AdminLayout.tsx` — drop inline sidebar and mobile strip, use `<AdminSidebar>`; update `AdminHeader` to remove back link, add icon+label Settings, add `<UserMenu>`, use class constants

---

## Task 1: Establish baseline

**Files:** (none changed — verification only)

- [ ] **Step 1: Confirm all checks pass on current branch**

Run: `mise check:quiet`
Expected: All checks pass. If anything fails, stop and fix before proceeding.

---

## Task 2: Create shared top-nav class constants

**Files:**
- Create: `app/components/layout/topNavClasses.ts`

- [ ] **Step 1: Create the constants file**

```ts
// app/components/layout/topNavClasses.ts
import { cn } from "@/libraries/utils";

export const TOP_NAV_WRAPPER = cn(
  "sticky top-0 z-30 border-b border-border bg-background dark:bg-neutral-900",
);
export const TOP_NAV_INNER = cn("mx-auto max-w-7xl px-4 md:px-6");
export const TOP_NAV_ROW = cn("flex h-14 items-center justify-between md:h-16");
```

- [ ] **Step 2: Verify types compile**

Run: `pnpm lint:types`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add app/components/layout/topNavClasses.ts
git commit -m "[Frontend] Add shared top-nav class constants"
```

---

## Task 3: Extract `<UserMenu>` component

**Files:**
- Create: `app/components/layout/UserMenu.tsx`
- Modify: `app/components/library/TopNav.tsx`

- [ ] **Step 1: Create the UserMenu component**

```tsx
// app/components/layout/UserMenu.tsx
import { KeyRound, List, LogOut, User, UserCog } from "lucide-react";
import { useCallback } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useAuth } from "@/hooks/useAuth";

const UserMenu = () => {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const handleLogout = useCallback(async () => {
    try {
      await logout();
      toast.success("Signed out successfully");
      navigate("/login");
    } catch {
      toast.error("Failed to sign out");
    }
  }, [logout, navigate]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button className="h-9 w-9" size="icon" variant="ghost">
          <User className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>
          <div className="flex flex-col">
            <span>{user?.username}</span>
            <span className="text-xs font-normal text-muted-foreground">
              {user?.role_name}
            </span>
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link to="/lists">
            <List className="h-4 w-4" />
            Lists
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/user/security">
            <KeyRound className="h-4 w-4" />
            Security
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/user/settings">
            <UserCog className="h-4 w-4" />
            User Settings
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={handleLogout}>
          <LogOut className="h-4 w-4" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default UserMenu;
```

- [ ] **Step 2: Replace the inline user menu in `TopNav.tsx`**

In `app/components/library/TopNav.tsx`:

- Delete lines 144–183 (the entire `<DropdownMenu>...</DropdownMenu>` block for the user menu).
- Replace with: `<UserMenu />`.
- Add import at the top: `import UserMenu from "@/components/layout/UserMenu";`
- Remove now-unused imports from the top of the file: `KeyRound`, `List`, `LogOut`, `User`, `UserCog` from `lucide-react`; `DropdownMenu`, `DropdownMenuContent`, `DropdownMenuItem`, `DropdownMenuLabel`, `DropdownMenuSeparator`, `DropdownMenuTrigger` from the dropdown import; `toast` from sonner; `useCallback` from react; `useNavigate` from react-router-dom (if no longer used — keep if still referenced).
- Remove the now-unused `handleLogout` function and `logout`/`navigate` destructurings from `useAuth()` and `useNavigate()` (keep `user`, `hasPermission` from `useAuth`).

- [ ] **Step 3: Verify lint, types, and tests**

Run: `mise check:quiet`
Expected: All checks pass.

- [ ] **Step 4: Manually verify the library user menu still works**

Start dev server if not already running: `mise start` (in another terminal).
In the browser on any `/libraries/:id` page, click the user icon in the top nav. Confirm:
- Dropdown opens showing username and role.
- Lists, Security, User Settings links work.
- Sign out signs you out and redirects to `/login`.

- [ ] **Step 5: Commit**

```bash
git add app/components/layout/UserMenu.tsx app/components/library/TopNav.tsx
git commit -m "[Frontend] Extract UserMenu component from library TopNav"
```

---

## Task 4: Create `<Sidebar>` primitive with tests

**Files:**
- Create: `app/components/layout/Sidebar.tsx`
- Create: `app/components/layout/Sidebar.test.tsx`

- [ ] **Step 1: Write failing tests for the Sidebar primitive**

```tsx
// app/components/layout/Sidebar.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Book, Settings } from "lucide-react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Sidebar, { type SidebarItem } from "./Sidebar";

const renderSidebar = (items: SidebarItem[]) =>
  render(
    <MemoryRouter>
      <Sidebar items={items} />
    </MemoryRouter>,
  );

const buildItem = (overrides: Partial<SidebarItem> = {}): SidebarItem => ({
  to: "/books",
  icon: <Book className="h-4 w-4" />,
  label: "Books",
  isActive: false,
  ...overrides,
});

describe("Sidebar", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("renders each visible item's label", () => {
    renderSidebar([
      buildItem({ to: "/a", label: "Alpha" }),
      buildItem({ to: "/b", label: "Bravo" }),
    ]);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
    expect(screen.getByText("Bravo")).toBeInTheDocument();
  });

  it("hides items with show: false", () => {
    renderSidebar([
      buildItem({ to: "/a", label: "Alpha" }),
      buildItem({ to: "/b", label: "Bravo", show: false }),
    ]);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
    expect(screen.queryByText("Bravo")).not.toBeInTheDocument();
  });

  it("treats missing show as visible", () => {
    renderSidebar([buildItem({ to: "/a", label: "Alpha" })]);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
  });

  it("starts expanded by default and shows version footer", () => {
    renderSidebar([buildItem()]);
    expect(screen.getByText("shisho")).toBeInTheDocument();
  });

  it("starts collapsed when localStorage says so, and hides version footer", () => {
    localStorage.setItem("shisho-sidebar-collapsed", "true");
    renderSidebar([buildItem()]);
    expect(screen.queryByText("shisho")).not.toBeInTheDocument();
  });

  it("toggles collapsed state and persists to localStorage", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderSidebar([buildItem()]);
    expect(localStorage.getItem("shisho-sidebar-collapsed")).toBe("false");

    const collapseButton = screen.getByRole("button", {
      name: /collapse sidebar/i,
    });
    await user.click(collapseButton);

    expect(localStorage.getItem("shisho-sidebar-collapsed")).toBe("true");
    expect(
      screen.getByRole("button", { name: /expand sidebar/i }),
    ).toBeInTheDocument();
  });

  it("applies active styling to active items", () => {
    renderSidebar([
      buildItem({ to: "/a", label: "Alpha", isActive: true }),
      buildItem({ to: "/b", label: "Bravo", isActive: false }),
    ]);
    const alpha = screen.getByText("Alpha").closest("a");
    const bravo = screen.getByText("Bravo").closest("a");
    expect(alpha?.className).toContain("bg-primary/10");
    expect(bravo?.className).not.toContain("bg-primary/10");
  });

  it("supports a Settings icon item type", () => {
    renderSidebar([
      buildItem({
        to: "/settings",
        icon: <Settings className="h-4 w-4" data-testid="settings-icon" />,
        label: "Settings",
      }),
    ]);
    expect(screen.getByTestId("settings-icon")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run tests and confirm they fail**

Run: `pnpm test:unit -- app/components/layout/Sidebar.test.tsx`
Expected: FAIL — module not found (`./Sidebar`).

- [ ] **Step 3: Implement the Sidebar primitive**

```tsx
// app/components/layout/Sidebar.tsx
import { ChevronsLeft, ChevronsRight } from "lucide-react";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/libraries/utils";

const SIDEBAR_COLLAPSED_KEY = "shisho-sidebar-collapsed";

export type SidebarItem = {
  to: string;
  icon: ReactNode;
  label: string;
  isActive: boolean;
  show?: boolean;
};

interface NavItemProps {
  item: SidebarItem;
  collapsed: boolean;
}

const NavItem = ({ item, collapsed }: NavItemProps) => {
  const linkContent = (
    <Link
      className={cn(
        "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
        collapsed && "justify-center px-2",
        item.isActive
          ? "bg-primary/10 text-primary dark:bg-violet-500/20 dark:text-violet-300"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
      to={item.to}
    >
      {item.icon}
      {!collapsed && item.label}
    </Link>
  );

  if (collapsed) {
    return (
      <Tooltip delayDuration={0}>
        <TooltipTrigger asChild>{linkContent}</TooltipTrigger>
        <TooltipContent side="right">{item.label}</TooltipContent>
      </Tooltip>
    );
  }

  return linkContent;
};

interface SidebarProps {
  items: SidebarItem[];
}

const Sidebar = ({ items }: SidebarProps) => {
  const [collapsed, setCollapsed] = useState(() => {
    const stored = localStorage.getItem(SIDEBAR_COLLAPSED_KEY);
    return stored === "true";
  });

  useEffect(() => {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(collapsed));
  }, [collapsed]);

  const visibleItems = items.filter((item) => item.show !== false);

  return (
    <aside
      className={cn(
        "sticky top-14 flex h-[calc(100vh-3.5rem)] shrink-0 flex-col border-r border-border bg-muted/30 transition-all duration-200 md:top-16 md:h-[calc(100vh-4rem)] dark:bg-neutral-900/50",
        collapsed ? "w-14" : "w-48",
      )}
    >
      <div className="flex-1">
        <nav className={cn("space-y-1 p-4", collapsed && "px-2")}>
          {visibleItems.map((item) => (
            <NavItem collapsed={collapsed} item={item} key={item.to} />
          ))}
        </nav>
        <div className={cn("px-4 pt-2", collapsed && "px-2")}>
          <div className="border-t border-border/50 pt-3">
            <Tooltip delayDuration={0}>
              <TooltipTrigger asChild>
                <button
                  aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                  className={cn(
                    "flex w-full cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                    "text-muted-foreground/60 hover:bg-muted/50 hover:text-muted-foreground",
                    collapsed && "justify-center px-2",
                  )}
                  onClick={() => setCollapsed(!collapsed)}
                >
                  {collapsed ? (
                    <ChevronsRight className="h-4 w-4" />
                  ) : (
                    <>
                      <ChevronsLeft className="h-4 w-4" />
                      <span className="text-xs">Collapse</span>
                    </>
                  )}
                </button>
              </TooltipTrigger>
              {collapsed && (
                <TooltipContent side="right">Expand sidebar</TooltipContent>
              )}
            </Tooltip>
          </div>
        </div>
      </div>
      {!collapsed && (
        <div className="p-4 pt-0">
          <a
            className="group flex items-center justify-center gap-1.5 rounded border border-transparent py-1.5 transition-all duration-200 hover:border-border/40 hover:bg-muted/30"
            href="https://github.com/shishobooks/shisho/releases"
            rel="noopener noreferrer"
            target="_blank"
          >
            <span className="text-[10px] text-muted-foreground/40 transition-colors group-hover:text-muted-foreground/70">
              shisho
            </span>
            <span className="font-mono text-[10px] text-muted-foreground/50 transition-colors group-hover:text-muted-foreground">
              {__APP_VERSION__}
            </span>
          </a>
        </div>
      )}
    </aside>
  );
};

export default Sidebar;
```

- [ ] **Step 4: Run tests and confirm they pass**

Run: `pnpm test:unit -- app/components/layout/Sidebar.test.tsx`
Expected: All tests pass.

- [ ] **Step 5: Verify full check still passes**

Run: `mise check:quiet`
Expected: All checks pass.

- [ ] **Step 6: Commit**

```bash
git add app/components/layout/Sidebar.tsx app/components/layout/Sidebar.test.tsx
git commit -m "[Frontend] Add shared Sidebar primitive"
```

---

## Task 5: Refactor `LibrarySidebar` to use `<Sidebar>`

**Files:**
- Modify: `app/components/library/LibrarySidebar.tsx`

- [ ] **Step 1: Replace LibrarySidebar with thin wrapper**

Replace the entire contents of `app/components/library/LibrarySidebar.tsx` with:

```tsx
// app/components/library/LibrarySidebar.tsx
import { Book, Bookmark, Layers, Settings, Tags, Users } from "lucide-react";
import { useLocation, useParams } from "react-router-dom";

import Sidebar, { type SidebarItem } from "@/components/layout/Sidebar";
import { useAuth } from "@/hooks/useAuth";

const LibrarySidebar = () => {
  const { libraryId } = useParams();
  const location = useLocation();
  const { hasPermission } = useAuth();

  if (!libraryId) return null;

  const basePath = `/libraries/${libraryId}`;

  const isBooksActive =
    location.pathname === basePath ||
    (location.pathname.startsWith(`${basePath}/books`) &&
      !location.pathname.startsWith(`${basePath}/series`) &&
      !location.pathname.startsWith(`${basePath}/people`) &&
      !location.pathname.startsWith(`${basePath}/genres`) &&
      !location.pathname.startsWith(`${basePath}/tags`) &&
      !location.pathname.startsWith(`${basePath}/settings`));

  const items: SidebarItem[] = [
    {
      to: basePath,
      icon: <Book className="h-4 w-4" />,
      label: "Books",
      isActive: isBooksActive,
    },
    {
      to: `${basePath}/series`,
      icon: <Layers className="h-4 w-4" />,
      label: "Series",
      isActive: location.pathname.startsWith(`${basePath}/series`),
    },
    {
      to: `${basePath}/people`,
      icon: <Users className="h-4 w-4" />,
      label: "People",
      isActive: location.pathname.startsWith(`${basePath}/people`),
    },
    {
      to: `${basePath}/genres`,
      icon: <Bookmark className="h-4 w-4" />,
      label: "Genres",
      isActive: location.pathname.startsWith(`${basePath}/genres`),
    },
    {
      to: `${basePath}/tags`,
      icon: <Tags className="h-4 w-4" />,
      label: "Tags",
      isActive: location.pathname.startsWith(`${basePath}/tags`),
    },
    {
      to: `${basePath}/settings`,
      icon: <Settings className="h-4 w-4" />,
      label: "Settings",
      isActive: location.pathname.startsWith(`${basePath}/settings`),
      show: hasPermission("libraries", "write"),
    },
  ];

  return <Sidebar items={items} />;
};

export default LibrarySidebar;
```

- [ ] **Step 2: Verify checks**

Run: `mise check:quiet`
Expected: All checks pass.

- [ ] **Step 3: Manually verify library sidebar**

Visit any `/libraries/:id` page. Confirm:
- Sidebar renders with all items.
- Clicking each item navigates and highlights.
- Collapse toggle works and persists after reload.
- Settings item only shows if user has `libraries:write`.
- Tooltips appear when collapsed.

- [ ] **Step 4: Commit**

```bash
git add app/components/library/LibrarySidebar.tsx
git commit -m "[Frontend] Refactor LibrarySidebar to use shared Sidebar primitive"
```

---

## Task 6: Create `AdminSidebar`

**Files:**
- Create: `app/components/pages/AdminSidebar.tsx`

- [ ] **Step 1: Create AdminSidebar**

```tsx
// app/components/pages/AdminSidebar.tsx
import { Briefcase, Cog, Library, Puzzle, ScrollText, Users } from "lucide-react";
import { useLocation } from "react-router-dom";

import Sidebar, { type SidebarItem } from "@/components/layout/Sidebar";
import { useAuth } from "@/hooks/useAuth";

const AdminSidebar = () => {
  const location = useLocation();
  const { hasPermission } = useAuth();

  const canViewConfig = hasPermission("config", "read");
  const canViewLibraries = hasPermission("libraries", "read");
  const canViewUsers = hasPermission("users", "read");
  const canViewJobs = hasPermission("jobs", "read");

  const items: SidebarItem[] = [
    {
      to: "/settings/server",
      icon: <Cog className="h-4 w-4" />,
      label: "Server",
      isActive:
        location.pathname === "/settings/server" ||
        location.pathname === "/settings",
      show: canViewConfig,
    },
    {
      to: "/settings/libraries",
      icon: <Library className="h-4 w-4" />,
      label: "Libraries",
      isActive: location.pathname.startsWith("/settings/libraries"),
      show: canViewLibraries,
    },
    {
      to: "/settings/users",
      icon: <Users className="h-4 w-4" />,
      label: "Users",
      isActive: location.pathname.startsWith("/settings/users"),
      show: canViewUsers,
    },
    {
      to: "/settings/jobs",
      icon: <Briefcase className="h-4 w-4" />,
      label: "Jobs",
      isActive: location.pathname === "/settings/jobs",
      show: canViewJobs,
    },
    {
      to: "/settings/plugins",
      icon: <Puzzle className="h-4 w-4" />,
      label: "Plugins",
      isActive: location.pathname === "/settings/plugins",
      show: canViewConfig,
    },
    {
      to: "/settings/logs",
      icon: <ScrollText className="h-4 w-4" />,
      label: "Logs",
      isActive: location.pathname === "/settings/logs",
      show: canViewConfig,
    },
  ];

  return <Sidebar items={items} />;
};

export default AdminSidebar;
```

- [ ] **Step 2: Verify types and lint**

Run: `pnpm lint:types && pnpm lint:eslint`
Expected: No errors. (Note: the component is not yet used; that's OK for now — lint should still pass since it's exported.)

- [ ] **Step 3: Commit**

```bash
git add app/components/pages/AdminSidebar.tsx
git commit -m "[Frontend] Add AdminSidebar using shared Sidebar primitive"
```

---

## Task 7: Update `MobileDrawer` to show admin nav when on `/settings/*`

**Files:**
- Modify: `app/components/library/MobileDrawer.tsx`

Background: `MobileDrawer` currently only renders library nav items when in a library context (`libraryId` is present). It does not list admin sub-pages. Because Task 8 removes the admin page's mobile horizontal strip, the drawer must cover admin nav on `/settings/*` routes.

- [ ] **Step 1: Add admin nav items computation in MobileDrawer**

In `app/components/library/MobileDrawer.tsx`, add the following logic inside the `MobileDrawer` component, after the `libraryNavItems` declaration (after line 181) and before the `canAccessAdmin` declaration:

```tsx
  // Admin nav items (when on /settings/* routes)
  const isAdminContext = location.pathname.startsWith("/settings");
  const adminNavItems = isAdminContext
    ? [
        {
          to: "/settings/server",
          icon: <Cog className="h-5 w-5" />,
          label: "Server",
          isActive:
            location.pathname === "/settings/server" ||
            location.pathname === "/settings",
          show: hasPermission("config", "read"),
        },
        {
          to: "/settings/libraries",
          icon: <Library className="h-5 w-5" />,
          label: "Libraries",
          isActive: location.pathname.startsWith("/settings/libraries"),
          show: hasPermission("libraries", "read"),
        },
        {
          to: "/settings/users",
          icon: <Users className="h-5 w-5" />,
          label: "Users",
          isActive: location.pathname.startsWith("/settings/users"),
          show: hasPermission("users", "read"),
        },
        {
          to: "/settings/jobs",
          icon: <Briefcase className="h-5 w-5" />,
          label: "Jobs",
          isActive: location.pathname === "/settings/jobs",
          show: hasPermission("jobs", "read"),
        },
        {
          to: "/settings/plugins",
          icon: <Puzzle className="h-5 w-5" />,
          label: "Plugins",
          isActive: location.pathname === "/settings/plugins",
          show: hasPermission("config", "read"),
        },
        {
          to: "/settings/logs",
          icon: <ScrollText className="h-5 w-5" />,
          label: "Logs",
          isActive: location.pathname === "/settings/logs",
          show: hasPermission("config", "read"),
        },
      ]
    : [];
```

Add these icon imports to the existing `from "lucide-react"` import block at the top: `Briefcase`, `Cog`, `Puzzle`, `ScrollText`. (`Library`, `Users`, `Settings` are already imported.)

- [ ] **Step 2: Render the admin nav section**

Find the "Library Navigation" block (currently around lines 343–359) that looks like:

```tsx
{isLibraryContext && libraryNavItems.length > 0 && (
  <nav className="py-2 border-b border-border">
    {libraryNavItems
      .filter((item) => item.show)
      .map((item) => (
        <NavItem ... />
      ))}
  </nav>
)}
```

Add a matching block immediately after it:

```tsx
{isAdminContext && adminNavItems.length > 0 && (
  <nav className="py-2 border-b border-border">
    {adminNavItems
      .filter((item) => item.show)
      .map((item) => (
        <NavItem
          icon={item.icon}
          isActive={item.isActive}
          key={item.to}
          label={item.label}
          onClick={close}
          to={item.to}
        />
      ))}
  </nav>
)}
```

- [ ] **Step 3: Verify checks**

Run: `mise check:quiet`
Expected: All checks pass.

- [ ] **Step 4: Manually verify mobile drawer on admin routes**

Resize browser to mobile width (or use devtools mobile emulator). Visit `/settings/users`. Open the hamburger menu. Confirm:
- Drawer opens and lists all admin sub-nav items the user has permission to see.
- Active highlighting matches the current route.
- Clicking an item navigates and closes the drawer.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/MobileDrawer.tsx
git commit -m "[Frontend] Show admin nav in mobile drawer on /settings routes"
```

---

## Task 8: Refactor `AdminLayout` — new sidebar, new header, drop mobile strip

**Files:**
- Modify: `app/components/pages/AdminLayout.tsx`

Context: `app/components/pages/Root.tsx` already wraps all routes in `<MobileNavProvider>` and renders `<MobileDrawer />`, `<Toaster />`, and `<ScrollRestoration />`. `LibraryLayout` does not redundantly wrap these, but the current `AdminLayout` does. The new `AdminLayout` follows `LibraryLayout`'s pattern (no redundant wrapping).

- [ ] **Step 1: Replace the entire file contents**

Replace everything in `app/components/pages/AdminLayout.tsx` with:

```tsx
// app/components/pages/AdminLayout.tsx
import { Menu, Settings } from "lucide-react";
import { Outlet } from "react-router-dom";

import {
  TOP_NAV_INNER,
  TOP_NAV_ROW,
  TOP_NAV_WRAPPER,
} from "@/components/layout/topNavClasses";
import UserMenu from "@/components/layout/UserMenu";
import Logo from "@/components/library/Logo";
import AdminSidebar from "@/components/pages/AdminSidebar";
import { Button } from "@/components/ui/button";
import { useMobileNav } from "@/contexts/MobileNav";

const AdminHeader = () => {
  const { toggle } = useMobileNav();

  return (
    <div className={TOP_NAV_WRAPPER}>
      <div className={TOP_NAV_INNER}>
        <div className={TOP_NAV_ROW}>
          <div className="flex items-center gap-2 md:gap-8">
            <Button
              aria-label="Open navigation menu"
              className="-ml-1 h-9 w-9 md:hidden"
              onClick={toggle}
              size="icon"
              variant="ghost"
            >
              <Menu className="h-5 w-5" />
            </Button>
            <Logo asLink />
            <div className="hidden items-center gap-2 text-sm font-medium text-muted-foreground sm:flex">
              <Settings className="h-4 w-4" />
              <span>Settings</span>
            </div>
          </div>
          <div className="flex items-center gap-1 md:gap-4">
            <UserMenu />
          </div>
        </div>
      </div>
    </div>
  );
};

const AdminLayout = () => {
  return (
    <div className="flex min-h-screen flex-col">
      <AdminHeader />
      <div className="flex flex-1">
        <div className="hidden md:block">
          <AdminSidebar />
        </div>
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-4 md:px-6 md:py-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
};

export default AdminLayout;
```

This mirrors `LibraryLayout`'s structure exactly (outer `flex flex-col min-h-screen`, sidebar in `hidden md:block`, main as `flex-1 w-full mx-auto max-w-7xl` with padding).

- [ ] **Step 2: Verify checks**

Run: `mise check:quiet`
Expected: All checks pass.

- [ ] **Step 3: Manually verify admin pages**

Visit `/settings/server`. Confirm:
- Top nav shows: logo, "Settings" with gear icon (non-interactive), user menu icon on the right. No "Back to Library" link.
- Left sidebar shows: Server, Libraries, Users, Jobs, Plugins, Logs (gated by permissions). Current page is highlighted.
- Collapse toggle works and persists. Persists across navigating to library page and back.
- Navigating each sidebar item works, highlight moves correctly.
- User menu opens, Sign out works.

Resize to mobile width. Confirm:
- Sidebar is hidden.
- Horizontal strip of admin nav items is GONE.
- Hamburger opens the mobile drawer with admin nav.
- "Settings" label and text in top nav is hidden on mobile.

- [ ] **Step 4: Commit**

```bash
git add app/components/pages/AdminLayout.tsx
git commit -m "[Frontend] Rebuild AdminLayout using shared Sidebar and UserMenu"
```

---

## Task 9: Update library `TopNav` to use shared class constants

**Files:**
- Modify: `app/components/library/TopNav.tsx`

- [ ] **Step 1: Replace the outer container classes with constants**

In `app/components/library/TopNav.tsx`, find the return statement and the outer `<div>`:

```tsx
<div className="border-b border-border bg-background dark:bg-neutral-900 sticky top-0 z-30">
  <div className="max-w-7xl mx-auto px-4 md:px-6">
    <div className="flex items-center justify-between h-14 md:h-16">
```

Replace with:

```tsx
<div className={TOP_NAV_WRAPPER}>
  <div className={TOP_NAV_INNER}>
    <div className={TOP_NAV_ROW}>
```

Add this import near the top of the file:

```ts
import {
  TOP_NAV_INNER,
  TOP_NAV_ROW,
  TOP_NAV_WRAPPER,
} from "@/components/layout/topNavClasses";
```

- [ ] **Step 2: Verify checks**

Run: `mise check:quiet`
Expected: All checks pass.

- [ ] **Step 3: Manually compare library and admin top navs**

Visit a library page, then an admin page. Confirm:
- Outer container looks identical (border, background, sticky, height).
- "Settings" label on admin pages sits at the same horizontal position (left of where "Books" sits on library pages).

- [ ] **Step 4: Commit**

```bash
git add app/components/library/TopNav.tsx
git commit -m "[Frontend] Use shared class constants in library TopNav"
```

---

## Task 10: Final verification

**Files:** (none changed)

- [ ] **Step 1: Run full check**

Run: `mise check:quiet`
Expected: All checks pass.

- [ ] **Step 2: Manual regression pass**

In the browser, test:

**Library pages** (`/libraries/:id`):
- Top nav: logo, library picker, resync button (if permitted), search, gear icon (if admin), user menu. All work.
- Sidebar: all items, collapse/expand, tooltips, persistence. Settings item gated.
- Mobile hamburger opens drawer with library nav items.

**Admin pages** (`/settings/*`):
- Top nav: logo, "Settings" with gear (non-interactive, no dropdown), user menu. No "Back to Library".
- Sidebar: all items (permissions-gated), collapse/expand works.
- Mobile hamburger opens drawer with admin nav items.

**Cross-flow:**
- Collapse the sidebar on a library page, navigate to `/settings/server` — sidebar stays collapsed.
- Expand it on admin, navigate to library — expanded.

- [ ] **Step 3: Confirm no stray files**

Run: `git status`
Expected: clean working tree.

---

## Out of Scope (not addressed by this plan)

- Changing the admin nav item set, ordering, or permissions.
- Changing the library nav item set.
- Adding scan/resync to the admin top nav.
- Making the "Settings" label in admin a dropdown.
- Extracting a full top-nav shell component with slots.
