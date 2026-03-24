import { Outlet, ScrollRestoration } from "react-router-dom";

import { BulkDownloadToast } from "@/components/library/BulkDownloadToast";
import MobileDrawer from "@/components/library/MobileDrawer";
import { Toaster } from "@/components/ui/sonner";
import { MobileNavProvider } from "@/contexts/MobileNav";

const Root = () => {
  return (
    <MobileNavProvider>
      <ScrollRestoration />
      <div className="flex bg-background font-sans min-h-screen">
        <div className="w-full">
          <Outlet />
        </div>
        <MobileDrawer />
        <Toaster richColors />
        <BulkDownloadToast />
      </div>
    </MobileNavProvider>
  );
};

export default Root;
