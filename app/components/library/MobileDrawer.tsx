import {
  Book,
  Bookmark,
  Check,
  ChevronRight,
  KeyRound,
  Layers,
  Library,
  List,
  LogOut,
  Settings,
  Tags,
  UserCog,
  Users,
  X,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import { useMobileNav } from "@/contexts/MobileNav";
import { useLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";
import { useAuth } from "@/hooks/useAuth";
import { cn } from "@/libraries/utils";

interface NavItemProps {
  to: string;
  icon: React.ReactNode;
  label: string;
  isActive: boolean;
  onClick: () => void;
}

const NavItem = ({ to, icon, label, isActive, onClick }: NavItemProps) => (
  <Link
    className={cn(
      "flex items-center gap-4 px-4 py-3.5 text-base font-medium transition-colors active:bg-muted/50",
      isActive
        ? "bg-primary/10 text-primary dark:bg-violet-500/20 dark:text-violet-300"
        : "text-foreground hover:bg-muted",
    )}
    onClick={onClick}
    to={to}
  >
    {icon}
    {label}
  </Link>
);

const MobileDrawer = () => {
  const { libraryId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout, hasPermission } = useAuth();
  const { isOpen, close } = useMobileNav();
  const [showLibraryPicker, setShowLibraryPicker] = useState(false);

  const librariesQuery = useLibraries({});
  const libraries = librariesQuery.data?.libraries || [];
  const currentLibrary = libraries.find((lib) => lib.id === Number(libraryId));

  const listsQuery = useListLists();
  const lists = listsQuery.data?.lists || [];

  // Check if we're currently viewing a list
  const listMatch = location.pathname.match(/^\/lists\/(\d+)/);
  const currentListId = listMatch ? Number(listMatch[1]) : null;
  const currentList = lists.find((list) => list.id === currentListId);
  const isViewingList = currentListId !== null;

  // Determine if we're in a library context
  const isLibraryContext = Boolean(libraryId);

  // Close drawer on route change
  useEffect(() => {
    close();
    setShowLibraryPicker(false);
  }, [location.pathname, close]);

  // Prevent body scroll when drawer is open
  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = "hidden";
    } else {
      document.body.style.overflow = "";
    }
    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen]);

  // Handle escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        if (showLibraryPicker) {
          setShowLibraryPicker(false);
        } else {
          close();
        }
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, close, showLibraryPicker]);

  const handleLogout = useCallback(async () => {
    close();
    try {
      await logout();
      toast.success("Signed out successfully");
      navigate("/login");
    } catch {
      toast.error("Failed to sign out");
    }
  }, [close, logout, navigate]);

  const handleLibrarySwitch = useCallback(
    (newLibraryId: number) => {
      close();
      navigate(`/libraries/${newLibraryId}`);
    },
    [close, navigate],
  );

  // Library navigation items (when in library context)
  const basePath = libraryId ? `/libraries/${libraryId}` : "";
  const libraryNavItems = libraryId
    ? [
        {
          to: basePath,
          icon: <Book className="h-5 w-5" />,
          label: "Books",
          isActive:
            location.pathname === basePath ||
            (location.pathname.startsWith(`${basePath}/books`) &&
              !location.pathname.includes("/series") &&
              !location.pathname.includes("/people") &&
              !location.pathname.includes("/genres") &&
              !location.pathname.includes("/tags") &&
              !location.pathname.includes("/settings")),
          show: true,
        },
        {
          to: `${basePath}/series`,
          icon: <Layers className="h-5 w-5" />,
          label: "Series",
          isActive: location.pathname.startsWith(`${basePath}/series`),
          show: true,
        },
        {
          to: `${basePath}/people`,
          icon: <Users className="h-5 w-5" />,
          label: "People",
          isActive: location.pathname.startsWith(`${basePath}/people`),
          show: true,
        },
        {
          to: `${basePath}/genres`,
          icon: <Bookmark className="h-5 w-5" />,
          label: "Genres",
          isActive: location.pathname.startsWith(`${basePath}/genres`),
          show: true,
        },
        {
          to: `${basePath}/tags`,
          icon: <Tags className="h-5 w-5" />,
          label: "Tags",
          isActive: location.pathname.startsWith(`${basePath}/tags`),
          show: true,
        },
        {
          to: `${basePath}/settings`,
          icon: <Settings className="h-5 w-5" />,
          label: "Library Settings",
          isActive: location.pathname.startsWith(`${basePath}/settings`),
          show: hasPermission("libraries", "write"),
        },
      ]
    : [];

  // Check if user has any admin permissions
  const canAccessAdmin =
    hasPermission("config", "read") ||
    hasPermission("users", "read") ||
    hasPermission("jobs", "read") ||
    hasPermission("libraries", "read");

  return (
    <>
      {/* Backdrop */}
      <div
        aria-hidden="true"
        className={cn(
          "fixed inset-0 z-40 bg-black/60 backdrop-blur-sm transition-opacity duration-300 md:hidden",
          isOpen ? "opacity-100" : "opacity-0 pointer-events-none",
        )}
        onClick={close}
      />

      {/* Main Drawer */}
      <aside
        aria-label="Mobile navigation"
        className={cn(
          "fixed inset-y-0 left-0 z-50 w-72 bg-background dark:bg-neutral-900 shadow-2xl transition-transform duration-300 ease-out md:hidden flex flex-col",
          isOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between h-14 px-4 border-b border-border shrink-0">
          <span className="text-lg font-semibold text-foreground">Menu</span>
          <button
            aria-label="Close navigation"
            className="p-2 -mr-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            onClick={close}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Scrollable content */}
        <div className="flex-1 overflow-y-auto">
          {/* Library/List Picker */}
          {(libraries.length > 0 || lists.length > 0) && (
            <div className="border-b border-border">
              <button
                className="flex items-center justify-between w-full px-4 py-3.5 text-left hover:bg-muted transition-colors"
                onClick={() => setShowLibraryPicker(!showLibraryPicker)}
              >
                <div className="flex items-center gap-3">
                  {isViewingList ? (
                    <List className="h-5 w-5 text-muted-foreground" />
                  ) : (
                    <Library className="h-5 w-5 text-muted-foreground" />
                  )}
                  <div>
                    <div className="text-sm text-muted-foreground">
                      {isViewingList ? "List" : "Library"}
                    </div>
                    <div className="font-medium">
                      {isViewingList
                        ? currentList?.name || "List"
                        : currentLibrary?.name || "Select Library"}
                    </div>
                  </div>
                </div>
                <ChevronRight
                  className={cn(
                    "h-5 w-5 text-muted-foreground transition-transform",
                    showLibraryPicker && "rotate-90",
                  )}
                />
              </button>

              {/* Library and List picker - expandable */}
              <div
                className={cn(
                  "overflow-hidden transition-all duration-200",
                  showLibraryPicker ? "max-h-96" : "max-h-0",
                )}
              >
                {/* Libraries section */}
                {libraries.length > 0 && (
                  <div className="bg-muted/30 py-1">
                    <div className="px-6 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                      Libraries
                    </div>
                    {libraries.map((library) => {
                      const isActive =
                        !isViewingList && library.id === Number(libraryId);
                      return (
                        <button
                          className={cn(
                            "flex items-center gap-3 w-full px-6 py-2.5 text-left transition-colors",
                            isActive
                              ? "bg-primary/10 text-primary dark:bg-violet-500/20 dark:text-violet-300"
                              : "hover:bg-muted",
                          )}
                          key={library.id}
                          onClick={() => handleLibrarySwitch(library.id)}
                        >
                          <span className="flex-1 truncate">
                            {library.name}
                          </span>
                          {isActive && <Check className="h-4 w-4 shrink-0" />}
                        </button>
                      );
                    })}
                  </div>
                )}

                {/* Lists section */}
                {lists.length > 0 && (
                  <div className="bg-muted/50 py-1 border-t border-border/50">
                    <div className="px-6 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                      Lists
                    </div>
                    {lists.slice(0, 5).map((list) => {
                      const isActive = currentListId === list.id;
                      return (
                        <Link
                          className={cn(
                            "flex items-center gap-3 w-full px-6 py-2.5 text-left transition-colors",
                            isActive
                              ? "bg-primary/10 text-primary dark:bg-violet-500/20 dark:text-violet-300"
                              : "hover:bg-muted",
                          )}
                          key={list.id}
                          onClick={close}
                          to={`/lists/${list.id}`}
                        >
                          <span className="flex-1 truncate">{list.name}</span>
                          <span
                            className={cn(
                              "shrink-0 tabular-nums text-xs",
                              isActive
                                ? "text-primary/70 dark:text-violet-300/70"
                                : "text-muted-foreground",
                            )}
                          >
                            {list.book_count}
                          </span>
                          {isActive && <Check className="h-4 w-4 shrink-0" />}
                        </Link>
                      );
                    })}
                    {lists.length > 5 && (
                      <Link
                        className="flex w-full items-center justify-center px-6 py-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                        onClick={close}
                        to="/lists"
                      >
                        View all {lists.length} lists →
                      </Link>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Library Navigation (when in library context) */}
          {isLibraryContext && libraryNavItems.length > 0 && (
            <nav className="py-2 border-b border-border">
              {libraryNavItems
                .filter((item) => item.show)
                .map((item) => (
                  <NavItem
                    icon={item.icon}
                    isActive={item.isActive}
                    key={item.to}
                    label={item.label}
                    onClick={close}
                    to={item.to}
                  />
                ))}
            </nav>
          )}

          {/* General Navigation */}
          <nav className="py-2 border-b border-border">
            <NavItem
              icon={<List className="h-5 w-5" />}
              isActive={location.pathname.startsWith("/lists")}
              label="Lists"
              onClick={close}
              to="/lists"
            />
            {canAccessAdmin && (
              <NavItem
                icon={<Settings className="h-5 w-5" />}
                isActive={location.pathname.startsWith("/settings")}
                label="Global Settings"
                onClick={close}
                to="/settings"
              />
            )}
          </nav>

          {/* User section */}
          <div className="py-2">
            <div className="px-4 py-2">
              <div className="text-sm font-medium text-foreground">
                {user?.username}
              </div>
              <div className="text-xs text-muted-foreground">
                {user?.role_name}
              </div>
            </div>
            <NavItem
              icon={<KeyRound className="h-5 w-5" />}
              isActive={location.pathname === "/user/security"}
              label="Security"
              onClick={close}
              to="/user/security"
            />
            <NavItem
              icon={<UserCog className="h-5 w-5" />}
              isActive={location.pathname === "/user/settings"}
              label="User Settings"
              onClick={close}
              to="/user/settings"
            />
            <button
              className="flex items-center gap-4 w-full px-4 py-3.5 text-base font-medium text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
              onClick={handleLogout}
            >
              <LogOut className="h-5 w-5" />
              Sign out
            </button>
            <div className="px-4 py-3">
              <a
                className="flex items-center justify-center gap-1.5 py-2 rounded-md text-muted-foreground/60 hover:text-muted-foreground transition-colors"
                href="https://github.com/shishobooks/shisho/releases"
                rel="noopener noreferrer"
                target="_blank"
              >
                <span className="text-xs">shisho</span>
                <span className="text-xs font-mono">{__APP_VERSION__}</span>
              </a>
            </div>
          </div>
        </div>
      </aside>
    </>
  );
};

export default MobileDrawer;
