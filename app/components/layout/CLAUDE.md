# Layout Primitives

Cross-area layout components shared between the library and admin/settings pages. Any UI element that appears in both contexts and needs to stay visually identical should live here (or be added here when it starts drifting).

## Current Primitives

| File | What it owns |
|------|--------------|
| `Sidebar.tsx` | Collapsible sidebar chrome: collapse state + `shisho-sidebar-collapsed` localStorage persistence, `NavItem` rendering, tooltip-when-collapsed, collapse toggle, version footer. Takes `items: SidebarItem[]`. |
| `UserMenu.tsx` | Avatar dropdown with username/role label and Lists / Security / User Settings / Sign out actions. Used in both library `TopNav` and admin header. |
| `topNavClasses.ts` | `cn()`-wrapped class constants (`TOP_NAV_WRAPPER`, `TOP_NAV_INNER`, `TOP_NAV_ROW`) for the outer top-nav geometry. Both `TopNav` and `AdminHeader` use these to guarantee identical container styling. |

## Usage Pattern

Area-specific sidebars (`app/components/library/LibrarySidebar.tsx`, `app/components/pages/AdminSidebar.tsx`) are thin wrappers: they compute a `SidebarItem[]` from route state and permissions and render `<Sidebar items={items} />`. All chrome (collapse, tooltips, footer) lives in `Sidebar` and cannot drift between the two.

## When to Add Here

If a UI element appears in both the library and admin contexts and needs to stay identical, put it in `layout/`. If it's specific to one context (e.g., `LibraryListPicker` for the library picker dropdown), keep it under that context's directory.
