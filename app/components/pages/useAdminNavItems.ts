import {
  Briefcase,
  Cog,
  HardDrive,
  Library,
  Puzzle,
  ScrollText,
  Users,
  type LucideIcon,
} from "lucide-react";
import { useLocation } from "react-router-dom";

import { useAuth } from "@/hooks/useAuth";

export type AdminNavItem = {
  to: string;
  Icon: LucideIcon;
  label: string;
  isActive: boolean;
  show: boolean;
};

export const useAdminNavItems = (): AdminNavItem[] => {
  const location = useLocation();
  const { hasPermission } = useAuth();

  const canViewConfig = hasPermission("config", "read");
  const canViewLibraries = hasPermission("libraries", "read");
  const canViewUsers = hasPermission("users", "read");
  const canViewJobs = hasPermission("jobs", "read");

  return [
    {
      to: "/settings/server",
      Icon: Cog,
      label: "Server",
      isActive:
        location.pathname === "/settings/server" ||
        location.pathname === "/settings",
      show: canViewConfig,
    },
    {
      to: "/settings/libraries",
      Icon: Library,
      label: "Libraries",
      isActive: location.pathname.startsWith("/settings/libraries"),
      show: canViewLibraries,
    },
    {
      to: "/settings/users",
      Icon: Users,
      label: "Users",
      isActive: location.pathname.startsWith("/settings/users"),
      show: canViewUsers,
    },
    {
      to: "/settings/jobs",
      Icon: Briefcase,
      label: "Jobs",
      isActive: location.pathname.startsWith("/settings/jobs"),
      show: canViewJobs,
    },
    {
      to: "/settings/plugins",
      Icon: Puzzle,
      label: "Plugins",
      isActive: location.pathname.startsWith("/settings/plugins"),
      show: canViewConfig,
    },
    {
      to: "/settings/cache",
      Icon: HardDrive,
      label: "Cache",
      isActive: location.pathname.startsWith("/settings/cache"),
      show: canViewConfig,
    },
    {
      to: "/settings/logs",
      Icon: ScrollText,
      label: "Logs",
      isActive: location.pathname.startsWith("/settings/logs"),
      show: canViewConfig,
    },
  ];
};
