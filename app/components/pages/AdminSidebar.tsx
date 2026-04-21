import {
  Briefcase,
  Cog,
  Library,
  Puzzle,
  ScrollText,
  Users,
} from "lucide-react";
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
