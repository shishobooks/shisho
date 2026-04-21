import { Book, Bookmark, Layers, Settings, Tags, Users } from "lucide-react";
import { useLocation, useParams } from "react-router-dom";

import Sidebar, { type SidebarItem } from "@/components/layout/Sidebar";
import { useAuth } from "@/hooks/useAuth";

const LibrarySidebar = () => {
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
      !location.pathname.startsWith(`${basePath}/settings`));

  const items: SidebarItem[] = [
    {
      to: basePath,
      icon: <Book className="h-4 w-4" />,
      label: "Books",
      isActive: isBooksActive,
    },
    {
      to: `${basePath}/series`,
      icon: <Layers className="h-4 w-4" />,
      label: "Series",
      isActive: location.pathname.startsWith(`${basePath}/series`),
    },
    {
      to: `${basePath}/people`,
      icon: <Users className="h-4 w-4" />,
      label: "People",
      isActive: location.pathname.startsWith(`${basePath}/people`),
    },
    {
      to: `${basePath}/genres`,
      icon: <Bookmark className="h-4 w-4" />,
      label: "Genres",
      isActive: location.pathname.startsWith(`${basePath}/genres`),
    },
    {
      to: `${basePath}/tags`,
      icon: <Tags className="h-4 w-4" />,
      label: "Tags",
      isActive: location.pathname.startsWith(`${basePath}/tags`),
    },
    {
      to: `${basePath}/settings`,
      icon: <Settings className="h-4 w-4" />,
      label: "Settings",
      isActive: location.pathname.startsWith(`${basePath}/settings`),
      show: hasPermission("libraries", "write"),
    },
  ];

  return <Sidebar items={items} />;
};

export default LibrarySidebar;
