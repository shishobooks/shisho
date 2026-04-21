# Admin Sidebar & Top Nav Consistency

## Background

The library pages (`LibrarySidebar`, `TopNav`) and admin pages (`AdminLayout`'s inline sidebar and `AdminHeader`) implement the same UI elements — a left navigation sidebar and a top navigation bar — but in drifted, separately-maintained code. The library implementations are better: a collapsible sidebar with icon-only mode and tooltips, a top nav with a user menu and global search. The admin implementations have their own patterns (non-collapsible desktop sidebar, mobile horizontal strip, no user menu, a "← Back to Library" link) and have diverged in styling details (dark mode color shades, layout structure).

## Goal

Bring the admin sidebar and top nav in line with the library versions, and extract the shared pieces into components/constants so the two cannot drift again.

## Approach

Two principles guide which pieces become shared components and which do not:

1. **Same shape, high drift risk → shared component.** Pieces where the two sites implement the same visual/behavioral element deserve a single source of truth.
2. **Different shape, low shared surface → don't force a shell.** The library top nav is a rich component (library picker, resync, global search with mobile-expand behavior). The admin top nav after this change is minimal (label + user menu). Forcing them into a slotted shell moves code without preventing meaningful drift.

The pieces with same-shape / high-drift characteristics are:
- The whole sidebar chrome (container, collapse state, nav items, tooltips, footer)
- The user menu dropdown

The piece with limited shared surface is the top nav shell itself — handled with shared className constants rather than a component.

## Components and Files

### New: `app/components/layout/Sidebar.tsx`

A generic sidebar primitive. Owns everything that should not drift:
- Collapsed state with `localStorage` persistence under the key `shisho-sidebar-collapsed` (shared across admin and library — the same key that `LibrarySidebar` currently uses).
- Width animation: `w-14` collapsed, `w-48` expanded.
- Container styling: `shrink-0 border-r border-border bg-muted/30 dark:bg-neutral-900/50 transition-all duration-200 h-[calc(100vh-3.5rem)] md:h-[calc(100vh-4rem)] sticky top-14 md:top-16 flex flex-col`.
- `NavItem` rendering: link styling, active/inactive color states, tooltip wrapper when collapsed.
- Collapse toggle button (chevrons-left/chevrons-right) with tooltip when collapsed.
- Version link footer — identical to the current `LibrarySidebar` footer, shown only when expanded.

Props:

```ts
export type SidebarItem = {
  to: string;
  icon: ReactNode;
  label: string;
  isActive: boolean;
  show?: boolean; // defaults to true; false hides the item
};

type SidebarProps = {
  items: SidebarItem[];
};
```

The component filters by `show` internally. No footer slot — the version link is baked in because both current and planned consumers want the same footer. If that ever needs to vary, we add a prop then, not now.

### New: `app/components/pages/AdminSidebar.tsx`

Thin wrapper. Computes the admin nav items with active-state logic and permission gates, passes them to `<Sidebar>`. Items:

- **Server** — `/settings/server` (active on `/settings` as well) — `canViewConfig`
- **Libraries** — `/settings/libraries` — `canViewLibraries`
- **Users** — `/settings/users` — `canViewUsers`
- **Jobs** — `/settings/jobs` — `canViewJobs`
- **Plugins** — `/settings/plugins` — `canViewConfig`
- **Logs** — `/settings/logs` — `canViewConfig`

Uses the same lucide icons as the current admin sidebar (`Cog`, `Library`, `Users`, `Briefcase`, `Puzzle`, `ScrollText`).

### Refactored: `app/components/library/LibrarySidebar.tsx`

Becomes a thin wrapper, same shape as `AdminSidebar`. Retains the `if (!libraryId) return null;` guard. Computes the 6 library nav items (Books, Series, People, Genres, Tags, Settings) with their current active-state logic and the Settings permission check. Passes items to `<Sidebar>`.

### New: `app/components/layout/UserMenu.tsx`

Extracted verbatim from `TopNav.tsx` lines 144–183. The dropdown shows username + role, then links to Lists, Security, User Settings, and Sign out. Owns its own `handleLogout` logic (the `useAuth` and `useNavigate` hooks move into this component).

No props — the component gets everything it needs from `useAuth`.

### New: `app/components/layout/topNavClasses.ts`

Shared className constants for the outer top nav layout. Wrapped in `cn()` so the prettier Tailwind plugin sorts them.

```ts
import { cn } from "@/libraries/utils";

export const TOP_NAV_WRAPPER = cn(
  "sticky top-0 z-30 border-b border-border bg-background dark:bg-neutral-900",
);
export const TOP_NAV_INNER = cn("mx-auto max-w-7xl px-4 md:px-6");
export const TOP_NAV_ROW = cn("flex h-14 items-center justify-between md:h-16");
```

Both `TopNav` and `AdminHeader` import and use these.

### Refactored: `AdminLayout.tsx`

Changes:

1. **`AdminHeader`:**
   - Remove both "← Back to Library" variants (the desktop `<Link>` and the mobile home-icon button).
   - Replace the `<span className="hidden sm:inline text-sm text-muted-foreground">Settings</span>` with a styled element that matches the layout of `LibraryListPicker`'s trigger — same `h-9 gap-2 text-muted-foreground` row, gear icon (`Settings` from lucide) + "Settings" text — but rendered as a non-interactive `<div>`: no dropdown, no chevron, no hover state. Hidden on mobile (`hidden sm:flex`) like the library's picker is hidden on mobile.
   - Add `<UserMenu />` in the right section.
   - Container classes use the new constants.

2. **`AdminLayoutContent`:**
   - Remove the mobile horizontal nav strip (currently lines 194–207) and its supporting `mobileNavItems` array and `MobileNavItem` component.
   - Replace the inline desktop `<aside>` (lines 212–300) with `<AdminSidebar />`.
   - Remove the inline `NavItem` component.
   - Remove the user/logout block (now in `UserMenu`).

3. The existing `<MobileDrawer />` call at line 192 stays. See "Verification required" below.

### Refactored: `TopNav.tsx`

- Replace the inline user menu dropdown (lines 144–183) with `<UserMenu />`.
- Replace the outer container classes (line 66) with the new `TOP_NAV_*` constants.

No behavior changes.

## Layout Alignment Detail

The user called out that the admin nav's "Settings" text sits at a different horizontal position than the library nav's "Books" button. In the library nav, "Books" is inside `LibraryListPicker`'s `<Button variant="ghost" className="h-9 gap-2 ...">` with the library icon to its left. After this change, admin's "Settings" uses the same 9-unit row height, same `gap-2`, same muted color, and the same gear icon to the left — so it occupies the same visual slot.

No dropdown indicator, no hover state, no click handler.

## Verification Required During Implementation

**`MobileDrawer` contents.** The current `AdminLayoutContent` renders both `<MobileDrawer />` and its own mobile horizontal nav strip. This plan drops the strip and relies on `MobileDrawer` to show admin nav items on mobile. Before removing the strip, verify that `MobileDrawer` is route-aware and renders admin items on `/settings/*` routes. If it only shows library items today, it needs to be updated to render the admin nav set as well — probably by deriving items from the same `SidebarItem[]` arrays the sidebars use, so mobile drawer content can't drift from sidebar content either.

This is the one place in the plan that might require additional work beyond the files listed above; it will be handled in the implementation plan.

## Migration / Compatibility

- No backend changes.
- No route changes.
- No API changes.
- No user-facing doc changes (nothing user-configurable changes; the nav arrangement is implementation detail).

## Out of Scope

- Changing the admin nav item set, order, or permissions.
- Changing the library nav item set.
- Adding a scan/resync button to admin (explicitly requested not to).
- Making the new "Settings" label a dropdown (explicitly requested not to).
- Extracting a full top-nav shell component (covered above — intentionally declined).

## Testing

- Manual verification in the browser for both library and admin pages across desktop and mobile breakpoints:
  - Sidebar collapses/expands and persists across page reloads and across library ↔ admin navigation.
  - Tooltips appear on hover when collapsed.
  - Active state highlights correctly on every admin and library route.
  - User menu opens, all four links work, Sign out works.
  - Mobile: hamburger opens `MobileDrawer`, which shows the correct nav items for the current area.
  - Admin "Settings" label aligns visually with library "Books" button.
- No new unit tests required — the extracted components are pure structural refactors of existing JSX. If the `Sidebar` component's `show`-filtering logic grows any real logic, revisit.
