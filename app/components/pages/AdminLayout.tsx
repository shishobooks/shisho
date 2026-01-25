import { Briefcase, Cog, Library, LogOut, Puzzle, Users } from "lucide-react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import Logo from "@/components/library/Logo";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import { useAuth } from "@/hooks/useAuth";

interface NavItemProps {
  to: string;
  icon: React.ReactNode;
  label: string;
  isActive: boolean;
}

const NavItem = ({ to, icon, label, isActive }: NavItemProps) => (
  <Link
    className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
      isActive
        ? "bg-primary/10 text-primary dark:bg-violet-900/30 dark:text-violet-300"
        : "text-muted-foreground hover:bg-muted hover:text-foreground"
    }`}
    to={to}
  >
    {icon}
    {label}
  </Link>
);

const AdminLayout = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout, hasPermission } = useAuth();

  const handleLogout = async () => {
    try {
      await logout();
      toast.success("Logged out successfully");
      navigate("/login");
    } catch {
      toast.error("Failed to log out");
    }
  };

  const canViewLibraries = hasPermission("libraries", "read");
  const canViewUsers = hasPermission("users", "read");
  const canViewJobs = hasPermission("jobs", "read");
  const canViewConfig = hasPermission("config", "read");

  return (
    <div className="min-h-screen bg-background">
      {/* Top bar */}
      <div className="border-b border-border bg-background dark:bg-neutral-900">
        <div className="max-w-7xl mx-auto px-6">
          <div className="flex items-center justify-between h-16">
            <div className="flex items-center gap-8">
              <Logo asLink />
              <span className="text-sm text-muted-foreground">Settings</span>
            </div>
            <div className="flex items-center gap-4">
              <Button asChild size="sm" variant="ghost">
                <Link to="/">← Back to Library</Link>
              </Button>
            </div>
          </div>
        </div>
      </div>

      <div className="max-w-7xl mx-auto px-6 py-8">
        <div className="flex gap-8">
          {/* Sidebar */}
          <aside className="w-56 flex-shrink-0">
            <nav className="space-y-1">
              {canViewConfig && (
                <NavItem
                  icon={<Cog className="h-4 w-4" />}
                  isActive={
                    location.pathname === "/settings/server" ||
                    location.pathname === "/settings"
                  }
                  label="Server"
                  to="/settings/server"
                />
              )}
              {canViewLibraries && (
                <NavItem
                  icon={<Library className="h-4 w-4" />}
                  isActive={location.pathname.startsWith("/settings/libraries")}
                  label="Libraries"
                  to="/settings/libraries"
                />
              )}
              {canViewUsers && (
                <NavItem
                  icon={<Users className="h-4 w-4" />}
                  isActive={location.pathname.startsWith("/settings/users")}
                  label="Users"
                  to="/settings/users"
                />
              )}
              {canViewJobs && (
                <NavItem
                  icon={<Briefcase className="h-4 w-4" />}
                  isActive={location.pathname === "/settings/jobs"}
                  label="Jobs"
                  to="/settings/jobs"
                />
              )}
              {canViewConfig && (
                <NavItem
                  icon={<Puzzle className="h-4 w-4" />}
                  isActive={location.pathname === "/settings/plugins"}
                  label="Plugins"
                  to="/settings/plugins"
                />
              )}
            </nav>

            {/* User info and logout */}
            <div className="mt-8 pt-6 border-t border-border">
              <div className="px-3 mb-3">
                <p className="text-sm font-medium text-foreground">
                  {user?.username}
                </p>
                <p className="text-xs text-muted-foreground">
                  {user?.role_name}
                </p>
              </div>
              <Button
                className="w-full justify-start gap-3 text-muted-foreground hover:text-foreground"
                onClick={handleLogout}
                size="sm"
                variant="ghost"
              >
                <LogOut className="h-4 w-4" />
                Sign out
              </Button>
              <a
                className="group mt-4 mx-3 flex items-center justify-center gap-1.5 py-1.5 rounded border border-transparent hover:border-border/40 hover:bg-muted/30 transition-all duration-200"
                href="https://github.com/shishobooks/shisho/releases"
                rel="noopener noreferrer"
                target="_blank"
              >
                <span className="text-[10px] text-muted-foreground/40 group-hover:text-muted-foreground/70 transition-colors">
                  shisho
                </span>
                <span className="text-[10px] font-mono text-muted-foreground/50 group-hover:text-muted-foreground transition-colors">
                  {__APP_VERSION__}
                </span>
              </a>
            </div>
          </aside>

          {/* Main content */}
          <main className="flex-1 min-w-0">
            <Outlet />
          </main>
        </div>
      </div>
      <Toaster richColors />
    </div>
  );
};

export default AdminLayout;
