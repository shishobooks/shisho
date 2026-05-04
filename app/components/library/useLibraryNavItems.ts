import {
  Book,
  Bookmark,
  Building2,
  Layers,
  Settings,
  Stamp,
  Tags,
  Users,
  type LucideIcon,
} from "lucide-react";
import { useLocation, useParams } from "react-router-dom";

import { useAuth } from "@/hooks/useAuth";

export type LibraryNavItem = {
  to: string;
  Icon: LucideIcon;
  label: string;
  drawerLabel?: string;
  isActive: boolean;
  show: boolean;
};

export const useLibraryNavItems = (): LibraryNavItem[] | null => {
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
      !location.pathname.startsWith(`${basePath}/publishers`) &&
      !location.pathname.startsWith(`${basePath}/imprints`) &&
      !location.pathname.startsWith(`${basePath}/settings`));

  return [
    {
      to: basePath,
      Icon: Book,
      label: "Books",
      isActive: isBooksActive,
      show: true,
    },
    {
      to: `${basePath}/series`,
      Icon: Layers,
      label: "Series",
      isActive: location.pathname.startsWith(`${basePath}/series`),
      show: true,
    },
    {
      to: `${basePath}/people`,
      Icon: Users,
      label: "People",
      isActive: location.pathname.startsWith(`${basePath}/people`),
      show: true,
    },
    {
      to: `${basePath}/genres`,
      Icon: Bookmark,
      label: "Genres",
      isActive: location.pathname.startsWith(`${basePath}/genres`),
      show: true,
    },
    {
      to: `${basePath}/tags`,
      Icon: Tags,
      label: "Tags",
      isActive: location.pathname.startsWith(`${basePath}/tags`),
      show: true,
    },
    {
      to: `${basePath}/publishers`,
      Icon: Building2,
      label: "Publishers",
      isActive: location.pathname.startsWith(`${basePath}/publishers`),
      show: true,
    },
    {
      to: `${basePath}/imprints`,
      Icon: Stamp,
      label: "Imprints",
      isActive: location.pathname.startsWith(`${basePath}/imprints`),
      show: true,
    },
    {
      to: `${basePath}/settings`,
      Icon: Settings,
      label: "Settings",
      drawerLabel: "Library Settings",
      isActive: location.pathname.startsWith(`${basePath}/settings`),
      show: hasPermission("libraries", "write"),
    },
  ];
};
