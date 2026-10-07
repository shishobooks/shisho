import { Settings } from "lucide-react";
import { useMemo } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";

import {
  AdvancedPluginsDialog,
  type AdvancedSection,
} from "@/components/plugins/AdvancedPluginsDialog";
import { DiscoverTab } from "@/components/plugins/DiscoverTab";
import { InstalledTab } from "@/components/plugins/InstalledTab";
import { TabUpdatePill } from "@/components/plugins/TabUpdatePill";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { usePluginsInstalled } from "@/hooks/queries/plugins";
import { useCan } from "@/hooks/useCan";
import { usePageTitle } from "@/hooks/usePageTitle";

// The first entry is the default section.
const ADVANCED_SECTIONS: readonly AdvancedSection[] = ["order", "repositories"];

const isAdvancedSection = (value: string): value is AdvancedSection =>
  (ADVANCED_SECTIONS as readonly string[]).includes(value);

const AdminPlugins = () => {
  usePageTitle("Plugins");

  const canWrite = useCan("config:write");

  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const activeTab: "installed" | "discover" = location.pathname.endsWith(
    "/discover",
  )
    ? "discover"
    : "installed";

  const { data: plugins = [] } = usePluginsInstalled();
  const updateCount = useMemo(
    () => plugins.filter((p) => !!p.update_available_version).length,
    [plugins],
  );

  // ?advanced=<section> is the dialog's state: present means open, and an
  // unknown section falls back to the first tab.
  const advancedParam = searchParams.get("advanced");
  const advancedOpen = advancedParam !== null;
  const advancedSection: AdvancedSection =
    advancedParam !== null && isAdvancedSection(advancedParam)
      ? advancedParam
      : ADVANCED_SECTIONS[0];

  const setAdvanced = (section: AdvancedSection | null) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (section === null) {
        next.delete("advanced");
      } else {
        next.set("advanced", section);
      }
      return next;
    });
  };

  const handleTabChange = (value: string) => {
    navigate(
      value === "discover" ? "/settings/plugins/discover" : "/settings/plugins",
    );
  };

  return (
    <div>
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between md:mb-8">
        <div>
          <h1 className="mb-1 text-xl font-semibold md:mb-2 md:text-2xl">
            Plugins
          </h1>
          <p className="text-sm text-muted-foreground md:text-base">
            Manage installed plugins, discover available plugins, configure
            execution order, and manage repositories.
          </p>
        </div>
        <div className="shrink-0">
          <Button
            aria-label="Advanced plugin settings"
            onClick={() => setAdvanced(ADVANCED_SECTIONS[0])}
            size="icon"
            variant="ghost"
          >
            <Settings aria-hidden="true" className="h-4 w-4" />
          </Button>
        </div>
      </div>

      <Tabs onValueChange={handleTabChange} value={activeTab}>
        <TabsList className="w-full justify-start overflow-x-auto">
          <TabsTrigger className="text-xs sm:text-sm" value="installed">
            Installed <TabUpdatePill count={updateCount} />
          </TabsTrigger>
          <TabsTrigger className="text-xs sm:text-sm" value="discover">
            Discover
          </TabsTrigger>
        </TabsList>

        <TabsContent value="installed">
          <InstalledTab />
        </TabsContent>

        <TabsContent value="discover">
          <DiscoverTab canWrite={canWrite} />
        </TabsContent>
      </Tabs>

      <AdvancedPluginsDialog
        onOpenChange={(open) => setAdvanced(open ? advancedSection : null)}
        onSectionChange={setAdvanced}
        open={advancedOpen}
        section={advancedSection}
      />
    </div>
  );
};

export default AdminPlugins;
