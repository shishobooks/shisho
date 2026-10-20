import {
  BookCheck,
  Briefcase,
  Cog,
  HardDrive,
  Library,
  Puzzle,
  ScrollText,
  Share2,
  TabletSmartphone,
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
  const canViewConfig = useCan(ROUTE_PERMISSIONS.adminConfig);
  const canViewLibraries = useCan(ROUTE_PERMISSIONS.adminLibraries);
  const canViewUsers = useCan(ROUTE_PERMISSIONS.adminUsers);
  const canViewJobs = useCan(ROUTE_PERMISSIONS.adminJobs);

  return [
    {
      to: "/admin/server",
      Icon: Cog,
      label: "Server",
      isActive: location.pathname === "/admin/server",
      show: canViewConfig,
    },
    {
      to: "/admin/libraries",
      Icon: Library,
      label: "Libraries",
      isActive: location.pathname.startsWith("/admin/libraries"),
      show: canViewLibraries,
    },
    {
      to: "/admin/review-criteria",
      Icon: BookCheck,
      label: "Review Criteria",
      isActive: location.pathname.startsWith("/admin/review-criteria"),
      show: canViewConfig,
    },
    {
      to: "/admin/sharing",
      Icon: Share2,
      label: "Sharing",
      isActive: location.pathname.startsWith("/admin/sharing"),
      show: canViewConfig,
    },
    {
      to: "/admin/users",
      Icon: Users,
      label: "Users",
      isActive: location.pathname.startsWith("/admin/users"),
      show: canViewUsers,
    },
    {
      to: "/admin/devices",
      Icon: TabletSmartphone,
      label: "Devices",
      isActive: location.pathname.startsWith("/admin/devices"),
      show: canViewUsers,
    },
    {
      to: "/admin/jobs",
      Icon: Briefcase,
      label: "Jobs",
      isActive: location.pathname.startsWith("/admin/jobs"),
      show: canViewJobs,
    },
    {
      to: "/admin/plugins",
      Icon: Puzzle,
      label: "Plugins",
      isActive: location.pathname.startsWith("/admin/plugins"),
      show: canViewConfig,
    },
    {
      to: "/admin/cache",
      Icon: HardDrive,
      label: "Cache",
      isActive: location.pathname.startsWith("/admin/cache"),
      show: canViewConfig,
    },
    {
      to: "/admin/logs",
      Icon: ScrollText,
      label: "Logs",
      isActive: location.pathname.startsWith("/admin/logs"),
      show: canViewConfig,
    },
  ];
};
