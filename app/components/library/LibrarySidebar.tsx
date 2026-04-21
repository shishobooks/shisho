import Sidebar, { type SidebarItem } from "@/components/layout/Sidebar";

import { useLibraryNavItems } from "./useLibraryNavItems";

const LibrarySidebar = () => {
  const navItems = useLibraryNavItems();

  if (!navItems) return null;

  const items: SidebarItem[] = navItems.map((item) => ({
    to: item.to,
    icon: <item.Icon className="h-4 w-4" />,
    label: item.label,
    isActive: item.isActive,
    show: item.show,
  }));

  return <Sidebar items={items} />;
};

export default LibrarySidebar;
