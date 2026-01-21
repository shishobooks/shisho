import {
  Book,
  Bookmark,
  ChevronsLeft,
  ChevronsRight,
  Layers,
  Settings,
  Tags,
  Users,
} from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useLocation, useParams } from "react-router-dom";

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuth } from "@/hooks/useAuth";
import { cn } from "@/libraries/utils";

const SIDEBAR_COLLAPSED_KEY = "shisho-sidebar-collapsed";

interface NavItemProps {
  to: string;
  icon: React.ReactNode;
  label: string;
  isActive: boolean;
  collapsed: boolean;
}

const NavItem = ({ to, icon, label, isActive, collapsed }: NavItemProps) => {
  const linkContent = (
    <Link
      className={cn(
        "flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors",
        collapsed && "justify-center px-2",
        isActive
          ? "bg-primary/10 text-primary dark:bg-violet-500/20 dark:text-violet-300"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
      to={to}
    >
      {icon}
      {!collapsed && label}
    </Link>
  );

  if (collapsed) {
    return (
      <Tooltip delayDuration={0}>
        <TooltipTrigger asChild>{linkContent}</TooltipTrigger>
        <TooltipContent side="right">{label}</TooltipContent>
      </Tooltip>
    );
  }

  return linkContent;
};

const LibrarySidebar = () => {
  const { libraryId } = useParams();
  const location = useLocation();
  const { hasPermission } = useAuth();
  const [collapsed, setCollapsed] = useState(() => {
    const stored = localStorage.getItem(SIDEBAR_COLLAPSED_KEY);
    return stored === "true";
  });

  useEffect(() => {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(collapsed));
  }, [collapsed]);

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

  const isSeriesActive = location.pathname.startsWith(`${basePath}/series`);
  const isPeopleActive = location.pathname.startsWith(`${basePath}/people`);
  const isGenresActive = location.pathname.startsWith(`${basePath}/genres`);
  const isTagsActive = location.pathname.startsWith(`${basePath}/tags`);
  const isSettingsActive = location.pathname.startsWith(`${basePath}/settings`);

  const navItems = [
    {
      to: basePath,
      icon: <Book className="h-4 w-4" />,
      label: "Books",
      isActive: isBooksActive,
      show: true,
    },
    {
      to: `${basePath}/series`,
      icon: <Layers className="h-4 w-4" />,
      label: "Series",
      isActive: isSeriesActive,
      show: true,
    },
    {
      to: `${basePath}/people`,
      icon: <Users className="h-4 w-4" />,
      label: "People",
      isActive: isPeopleActive,
      show: true,
    },
    {
      to: `${basePath}/genres`,
      icon: <Bookmark className="h-4 w-4" />,
      label: "Genres",
      isActive: isGenresActive,
      show: true,
    },
    {
      to: `${basePath}/tags`,
      icon: <Tags className="h-4 w-4" />,
      label: "Tags",
      isActive: isTagsActive,
      show: true,
    },
    {
      to: `${basePath}/settings`,
      icon: <Settings className="h-4 w-4" />,
      label: "Settings",
      isActive: isSettingsActive,
      show: hasPermission("libraries", "write"),
    },
  ];

  return (
    <aside
      className={cn(
        "shrink-0 border-r border-border bg-muted/30 dark:bg-neutral-900/50 transition-all duration-200",
        collapsed ? "w-14" : "w-48",
      )}
    >
      <div className="sticky top-0 flex flex-col">
        <nav className={cn("p-4 space-y-1", collapsed && "px-2")}>
          {navItems
            .filter((item) => item.show)
            .map((item) => (
              <NavItem
                collapsed={collapsed}
                icon={item.icon}
                isActive={item.isActive}
                key={item.to}
                label={item.label}
                to={item.to}
              />
            ))}
        </nav>
        <div className={cn("px-4 pt-2", collapsed && "px-2")}>
          <div className="border-t border-border/50 pt-3">
            <Tooltip delayDuration={0}>
              <TooltipTrigger asChild>
                <button
                  aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                  className={cn(
                    "flex items-center gap-3 w-full px-3 py-2 rounded-md text-sm font-medium transition-colors",
                    "text-muted-foreground/60 hover:text-muted-foreground hover:bg-muted/50",
                    collapsed && "justify-center px-2",
                  )}
                  onClick={() => setCollapsed(!collapsed)}
                >
                  {collapsed ? (
                    <ChevronsRight className="h-4 w-4" />
                  ) : (
                    <>
                      <ChevronsLeft className="h-4 w-4" />
                      <span className="text-xs">Collapse</span>
                    </>
                  )}
                </button>
              </TooltipTrigger>
              {collapsed && (
                <TooltipContent side="right">Expand sidebar</TooltipContent>
              )}
            </Tooltip>
          </div>
        </div>
      </div>
    </aside>
  );
};

export default LibrarySidebar;
