import { usePluginsAvailable } from "@/hooks/queries/plugins";

export const useInstalledPluginImageUrl = () => {
  const { data: available = [] } = usePluginsAvailable();
  return (scope: string, id: string): string | undefined =>
    available.find((p) => p.scope === scope && p.id === id)?.imageUrl ||
    undefined;
};
