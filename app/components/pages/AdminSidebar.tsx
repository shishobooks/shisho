import Sidebar, { type SidebarItem } from "@/components/layout/Sidebar";

import { useAdminNavItems } from "./useAdminNavItems";

const AdminSidebar = () => {
  const navItems = useAdminNavItems();

  const items: SidebarItem[] = navItems.map((item) => ({
    to: item.to,
    icon: <item.Icon className="h-4 w-4" />,
    label: item.label,
    isActive: item.isActive,
    show: item.show,
  }));

  return <Sidebar items={items} />;
};

export default AdminSidebar;
