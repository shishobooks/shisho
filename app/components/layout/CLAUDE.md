# Layout Primitives

Cross-area layout components shared between the library and admin/settings pages. Any UI element that appears in both contexts and needs to stay visually identical should live here (or be added here when it starts drifting).

## Current Primitives

| File | What it owns |
|------|--------------|
| `DemoBanner.tsx` | Persistent Demo Mode notice and links, rendered once by the protected-route root layout. Sits at `z-40`: above the top nav (`z-30`), below the modal layer (`z-50`). |
| `Sidebar.tsx` | Collapsible sidebar chrome: collapse state + `shisho-sidebar-collapsed` localStorage persistence, `NavItem` rendering, tooltip-when-collapsed, collapse toggle, version footer. Takes `items: SidebarItem[]`. |
| `UserMenu.tsx` | Avatar dropdown with username/role label and Lists / Security / User Settings / Sign out actions (Security is hidden in Demo Mode). Used in both library `TopNav` and admin header. |
| `topNavClasses.ts` | `cn()`-wrapped class constants (`TOP_NAV_WRAPPER`, `TOP_NAV_INNER`, `TOP_NAV_ROW`) for the outer top-nav geometry. Both `TopNav` and `AdminHeader` use these to guarantee identical container styling. |

## Usage Pattern

Area-specific sidebars (`app/components/library/LibrarySidebar.tsx`, `app/components/pages/AdminSidebar.tsx`) are thin wrappers: they compute a `SidebarItem[]` from route state and permissions and render `<Sidebar items={items} />`. All chrome (collapse, tooltips, footer) lives in `Sidebar` and cannot drift between the two.

## Top Nav and Sidebar Geometry

`TOP_NAV_ROW` sets a 3.5rem mobile or 4rem desktop content height, while `TOP_NAV_WRAPPER` adds a 1px bottom border. `DemoBanner` measures its rendered height into `--demo-banner-height`; the top nav, sidebar, mobile drawer, full-screen readers, and shared layouts use that variable for their offsets and viewport heights. The sidebar calculations must include both the banner height and top-nav border. If any of this geometry changes, update these consumers together.

Modal surfaces (`Sheet`, `Dialog`, `Drawer`) are not offset for the banner. They open from the viewport edge at `z-50` and cover it, overlay included. Do not raise the banner above `z-50`: it then clips sheet headers and close buttons. Non-modal full-height surfaces (mobile drawer, readers) are offset with `--demo-banner-height` instead.

## When to Add Here

If a UI element appears in both the library and admin contexts and needs to stay identical, put it in `layout/`. If it's specific to one context (e.g., `LibraryListPicker` for the library picker dropdown), keep it under that context's directory.
