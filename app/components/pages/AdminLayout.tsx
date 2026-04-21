import { Menu, Settings } from "lucide-react";
import { Outlet } from "react-router-dom";

import {
  TOP_NAV_INNER,
  TOP_NAV_ROW,
  TOP_NAV_WRAPPER,
} from "@/components/layout/topNavClasses";
import UserMenu from "@/components/layout/UserMenu";
import Logo from "@/components/library/Logo";
import AdminSidebar from "@/components/pages/AdminSidebar";
import { Button } from "@/components/ui/button";
import { useMobileNav } from "@/contexts/MobileNav";

const AdminHeader = () => {
  const { toggle } = useMobileNav();

  return (
    <div className={TOP_NAV_WRAPPER}>
      <div className={TOP_NAV_INNER}>
        <div className={TOP_NAV_ROW}>
          <div className="flex items-center gap-2 md:gap-8">
            <Button
              aria-label="Open navigation menu"
              className="-ml-1 h-9 w-9 md:hidden"
              onClick={toggle}
              size="icon"
              variant="ghost"
            >
              <Menu className="h-5 w-5" />
            </Button>
            <Logo asLink />
            <div className="hidden items-center gap-2 text-sm font-medium text-muted-foreground sm:flex">
              <Settings className="h-4 w-4" />
              <span>Settings</span>
            </div>
          </div>
          <div className="flex items-center gap-1 md:gap-4">
            <UserMenu />
          </div>
        </div>
      </div>
    </div>
  );
};

const AdminLayout = () => {
  return (
    <div className="flex min-h-screen flex-col">
      <AdminHeader />
      <div className="flex flex-1">
        <div className="hidden md:block">
          <AdminSidebar />
        </div>
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-4 md:px-6 md:py-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
};

export default AdminLayout;
