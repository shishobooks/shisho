import { Settings } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import { AdvancedPluginsDialog } from "@/components/plugins/AdvancedPluginsDialog";
import { DiscoverTab } from "@/components/plugins/DiscoverTab";
import { InstalledTab } from "@/components/plugins/InstalledTab";
import { TabUpdatePill } from "@/components/plugins/TabUpdatePill";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { usePluginsInstalled } from "@/hooks/queries/plugins";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";

// "browse" is kept for backward-compat URL slugs; both resolve to "discover".
const validTabs = ["installed", "discover", "browse"] as const;
type TabValue = (typeof validTabs)[number];

const normalizeTab = (tab: string | undefined): TabValue => {
  if (tab === "browse") return "discover";
  return validTabs.includes(tab as TabValue) ? (tab as TabValue) : "installed";
};

const AdminPlugins = () => {
  usePageTitle("Plugins");

  const { hasPermission } = useAuth();
  const canWrite = hasPermission("config", "write");

  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const activeTab: TabValue = normalizeTab(tab);

  const { data: plugins = [] } = usePluginsInstalled();
  const updateCount = useMemo(
    () => plugins.filter((p) => !!p.update_available_version).length,
    [plugins],
  );

  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [advancedDefault, setAdvancedDefault] = useState<
    "order" | "repositories"
  >("order");

  // On mount: open dialog at the right section if ?advanced= param is present
  useEffect(() => {
    const adv = searchParams.get("advanced");
    if (adv === "order" || adv === "repositories") {
      setAdvancedDefault(adv);
      setAdvancedOpen(true);
      const next = new URLSearchParams(searchParams);
      next.delete("advanced");
      setSearchParams(next, { replace: true });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // intentionally empty — only on mount

  const handleTabChange = (value: string) => {
    if (value === "installed") {
      navigate("/settings/plugins");
    } else {
      navigate(`/settings/plugins/${value}`);
    }
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
            onClick={() => setAdvancedOpen(true)}
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
        defaultSection={advancedDefault}
        onOpenChange={setAdvancedOpen}
        open={advancedOpen}
      />
    </div>
  );
};

export default AdminPlugins;
