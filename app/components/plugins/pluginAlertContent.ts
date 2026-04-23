import {
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
  type Plugin,
} from "@/types/generated/models";

export const pluginAlertContent = (
  installed: Plugin | undefined,
): { body?: string; title: string } | null => {
  if (!installed) return null;
  if (installed.status === PluginStatusNotSupported) {
    return {
      body: installed.load_error,
      title: "Plugin is not compatible with this Shisho version",
    };
  }
  if (installed.status === PluginStatusMalfunctioned || installed.load_error) {
    return { body: installed.load_error, title: "Plugin failed to load" };
  }
  return null;
};
