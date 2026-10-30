import { Outlet, ScrollRestoration } from "react-router-dom";

import DemoBanner from "@/components/layout/DemoBanner";
import { BulkDownloadToast } from "@/components/library/BulkDownloadToast";
import MobileDrawer from "@/components/library/MobileDrawer";
import { Toaster } from "@/components/ui/sonner";
import { ContentWidthProvider } from "@/contexts/ContentWidth";
import { MobileNavProvider } from "@/contexts/MobileNav";
import { useFillTimeZone } from "@/hooks/useFillTimeZone";

const Root = () => {
  useFillTimeZone();
  return (
    <MobileNavProvider>
      <ContentWidthProvider>
        <ScrollRestoration />
        <div className="flex bg-background font-sans min-h-screen">
          <div className="w-full">
            <DemoBanner />
            <Outlet />
          </div>
          <MobileDrawer />
          <Toaster richColors />
          <BulkDownloadToast />
        </div>
      </ContentWidthProvider>
    </MobileNavProvider>
  );
};

export default Root;
