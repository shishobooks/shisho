import { Outlet, ScrollRestoration } from "react-router-dom";

import { ReloadOnNewBuild } from "@/components/layout/NewBuild";
import TopBanners from "@/components/layout/TopBanners";
import { BulkDownloadToast } from "@/components/library/BulkDownloadToast";
import MobileDrawer from "@/components/library/MobileDrawer";
import { Toaster } from "@/components/ui/sonner";
import { ContentWidthProvider } from "@/contexts/ContentWidth";
import { GettingStartedPanelProvider } from "@/contexts/GettingStarted";
import { MobileNavProvider } from "@/contexts/MobileNav";
import { NavigationHistoryProvider } from "@/contexts/NavigationHistory";
import { useFillTimeZone } from "@/hooks/useFillTimeZone";

const Root = () => {
  useFillTimeZone();
  return (
    <NavigationHistoryProvider>
      <MobileNavProvider>
        <GettingStartedPanelProvider>
          <ContentWidthProvider>
            <ScrollRestoration />
            <ReloadOnNewBuild />
            <div className="flex bg-background font-sans min-h-screen">
              <div className="w-full">
                <TopBanners />
                <Outlet />
              </div>
              <MobileDrawer />
              <Toaster richColors />
              <BulkDownloadToast />
            </div>
          </ContentWidthProvider>
        </GettingStartedPanelProvider>
      </MobileNavProvider>
    </NavigationHistoryProvider>
  );
};

export default Root;
