import { Outlet } from "react-router-dom";

import MobileDrawer from "@/components/library/MobileDrawer";
import { Toaster } from "@/components/ui/sonner";
import { MobileNavProvider } from "@/contexts/MobileNav";

const Root = () => {
  return (
    <MobileNavProvider>
      <div className="flex bg-background font-sans min-h-screen">
        <div className="w-full">
          <Outlet />
        </div>
        <MobileDrawer />
        <Toaster richColors />
      </div>
    </MobileNavProvider>
  );
};

export default Root;
