import { ChevronsLeft, ChevronsRight } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/libraries/utils";

const SIDEBAR_COLLAPSED_KEY = "shisho-sidebar-collapsed";

export type SidebarItem = {
  to: string;
  icon: ReactNode;
  label: string;
  isActive: boolean;
  show?: boolean;
};

interface NavItemProps {
  item: SidebarItem;
  collapsed: boolean;
}

const NavItem = ({ item, collapsed }: NavItemProps) => {
  const linkContent = (
    <Link
      className={cn(
        "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
        collapsed && "justify-center px-2",
        item.isActive
          ? "bg-primary/10 text-primary dark:bg-violet-500/20 dark:text-violet-300"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
      to={item.to}
    >
      {item.icon}
      {!collapsed && item.label}
    </Link>
  );

  if (collapsed) {
    return (
      <Tooltip delayDuration={0}>
        <TooltipTrigger asChild>{linkContent}</TooltipTrigger>
        <TooltipContent side="right">{item.label}</TooltipContent>
      </Tooltip>
    );
  }

  return linkContent;
};

interface SidebarProps {
  items: SidebarItem[];
}

const Sidebar = ({ items }: SidebarProps) => {
  const [collapsed, setCollapsed] = useState(() => {
    const stored = localStorage.getItem(SIDEBAR_COLLAPSED_KEY);
    return stored === "true";
  });

  useEffect(() => {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(collapsed));
  }, [collapsed]);

  const visibleItems = items.filter((item) => item.show !== false);

  return (
    <aside
      className={cn(
        "sticky top-14 flex h-[calc(100vh-3.5rem)] shrink-0 flex-col border-r border-border bg-muted/30 transition-all duration-200 md:top-16 md:h-[calc(100vh-4rem)] dark:bg-neutral-900/50",
        collapsed ? "w-14" : "w-48",
      )}
    >
      <div className="flex-1">
        <nav className={cn("space-y-1 p-4", collapsed && "px-2")}>
          {visibleItems.map((item) => (
            <NavItem collapsed={collapsed} item={item} key={item.to} />
          ))}
        </nav>
        <div className={cn("px-4 pt-2", collapsed && "px-2")}>
          <div className="border-t border-border/50 pt-3">
            <Tooltip delayDuration={0}>
              <TooltipTrigger asChild>
                <button
                  aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                  className={cn(
                    "flex w-full cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                    "text-muted-foreground/60 hover:bg-muted/50 hover:text-muted-foreground",
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
      {!collapsed && (
        <div className="p-4 pt-0">
          <a
            className="group flex items-center justify-center gap-1.5 rounded border border-transparent py-1.5 transition-all duration-200 hover:border-border/40 hover:bg-muted/30"
            href="https://github.com/shishobooks/shisho/releases"
            rel="noopener noreferrer"
            target="_blank"
          >
            <span className="text-[10px] text-muted-foreground/40 transition-colors group-hover:text-muted-foreground/70">
              shisho
            </span>
            <span className="font-mono text-[10px] text-muted-foreground/50 transition-colors group-hover:text-muted-foreground">
              {__APP_VERSION__}
            </span>
          </a>
        </div>
      )}
    </aside>
  );
};

export default Sidebar;
