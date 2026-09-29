import {
  BookCheck,
  Briefcase,
  Cog,
  HardDrive,
  Library,
  Puzzle,
  ScrollText,
  Share2,
  Users,
  type LucideIcon,
} from "lucide-react";
import { useLocation } from "react-router-dom";

import { useCan } from "@/hooks/useCan";
import { ROUTE_PERMISSIONS } from "@/utils/permissions";

export type AdminNavItem = {
  to: string;
  Icon: LucideIcon;
  label: string;
  isActive: boolean;
  show: boolean;
};

export const useAdminNavItems = (): AdminNavItem[] => {
  const location = useLocation();

  // Each entry shows when the role passes its page's route guard.
  const canViewConfig = useCan(ROUTE_PERMISSIONS.settingsConfig);
  const canViewLibraries = useCan(ROUTE_PERMISSIONS.settingsLibraries);
  const canViewUsers = useCan(ROUTE_PERMISSIONS.settingsUsers);
  const canViewJobs = useCan(ROUTE_PERMISSIONS.settingsJobs);

  return [
    {
      to: "/settings/server",
      Icon: Cog,
      label: "Server",
      isActive: location.pathname === "/settings/server",
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
      to: "/settings/review-criteria",
      Icon: BookCheck,
      label: "Review Criteria",
      isActive: location.pathname.startsWith("/settings/review-criteria"),
      show: canViewConfig,
    },
    {
      to: "/settings/sharing",
      Icon: Share2,
      label: "Sharing",
      isActive: location.pathname.startsWith("/settings/sharing"),
      show: canViewConfig,
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
