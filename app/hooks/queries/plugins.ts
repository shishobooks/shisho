import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { API, type ShishoAPIError } from "@/libraries/api";
import type {
  Plugin,
  PluginIdentifierType,
  PluginOrder,
  PluginRepository,
} from "@/types/generated/models";

// Re-export generated types so consumers can import from this module
export type {
  Plugin,
  PluginHookType,
  PluginOrder,
  PluginRepository,
  PluginStatus,
} from "@/types/generated/models";
export {
  PluginStatusActive,
  PluginStatusDisabled,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
} from "@/types/generated/models";

export interface PluginVersion {
  version: string;
  min_app_version: string;
  changelog: string;
  download_url: string;
  sha256: string;
  manifest_version: number;
}

export interface AvailablePlugin {
  scope: string;
  id: string;
  name: string;
  overview: string;
  description: string;
  author: string;
  homepage: string;
  imageUrl: string;
  versions: PluginVersion[];
}

// --- Query Keys ---

export interface ConfigField {
  type: "string" | "boolean" | "number" | "select" | "textarea";
  label: string;
  description: string;
  required: boolean;
  secret: boolean;
  default?: unknown;
  min?: number | null;
  max?: number | null;
  options?: { value: string; label: string }[] | null;
}

export type ConfigSchema = Record<string, ConfigField>;

export interface PluginConfigResponse {
  schema: ConfigSchema;
  values: Record<string, unknown>;
  declaredFields?: string[];
  fieldSettings?: Record<string, boolean>;
  confidence_threshold?: number | null;
}

export enum QueryKey {
  PluginsInstalled = "PluginsInstalled",
  PluginsAvailable = "PluginsAvailable",
  PluginConfig = "PluginConfig",
  PluginOrder = "PluginOrder",
  PluginRepositories = "PluginRepositories",
  PluginIdentifierTypes = "PluginIdentifierTypes",
}

// --- Queries ---

export const usePluginsInstalled = () => {
  return useQuery<Plugin[], ShishoAPIError>({
    queryKey: [QueryKey.PluginsInstalled],
    queryFn: ({ signal }) => {
      return API.request("GET", "/plugins/installed", null, null, signal);
    },
  });
};

export const usePluginsAvailable = () => {
  return useQuery<AvailablePlugin[], ShishoAPIError>({
    queryKey: [QueryKey.PluginsAvailable],
    queryFn: ({ signal }) => {
      return API.request("GET", "/plugins/available", null, null, signal);
    },
  });
};

export const usePluginOrder = (hookType: string) => {
  return useQuery<PluginOrder[], ShishoAPIError>({
    queryKey: [QueryKey.PluginOrder, hookType],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/plugins/order/${hookType}`,
        null,
        null,
        signal,
      );
    },
  });
};

export const usePluginConfig = (scope?: string, id?: string) => {
  return useQuery<PluginConfigResponse, ShishoAPIError>({
    enabled: Boolean(scope && id),
    queryKey: [QueryKey.PluginConfig, scope, id],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/plugins/installed/${scope}/${id}/config`,
        null,
        null,
        signal,
      );
    },
  });
};

export const usePluginRepositories = () => {
  return useQuery<PluginRepository[], ShishoAPIError>({
    queryKey: [QueryKey.PluginRepositories],
    queryFn: ({ signal }) => {
      return API.request("GET", "/plugins/repositories", null, null, signal);
    },
  });
};

export const usePluginIdentifierTypes = () => {
  return useQuery<PluginIdentifierType[], ShishoAPIError>({
    queryKey: [QueryKey.PluginIdentifierTypes],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        "/plugins/identifier-types",
        null,
        null,
        signal,
      );
    },
  });
};

// --- Mutations ---

export interface InstallPluginPayload {
  scope: string;
  id: string;
  name?: string;
  version?: string;
  download_url?: string;
  sha256?: string;
}

export const useInstallPlugin = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, InstallPluginPayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/plugins/installed", payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsInstalled],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsAvailable],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginIdentifierTypes],
      });
    },
  });
};

export const useUninstallPlugin = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, { scope: string; id: string }>({
    mutationFn: ({ scope, id }) => {
      return API.request("DELETE", `/plugins/installed/${scope}/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsInstalled],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsAvailable],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginIdentifierTypes],
      });
    },
  });
};

export interface UpdatePluginPayload {
  enabled?: boolean;
  config?: Record<string, string>;
}

export const useUpdatePlugin = () => {
  const queryClient = useQueryClient();

  return useMutation<
    void,
    ShishoAPIError,
    { scope: string; id: string; payload: UpdatePluginPayload }
  >({
    mutationFn: ({ scope, id, payload }) => {
      return API.request("PATCH", `/plugins/installed/${scope}/${id}`, payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsInstalled],
      });
    },
  });
};

export const useUpdatePluginVersion = () => {
  const queryClient = useQueryClient();

  return useMutation<Plugin, ShishoAPIError, { scope: string; id: string }>({
    mutationFn: ({ scope, id }) => {
      return API.request<Plugin>(
        "POST",
        `/plugins/installed/${scope}/${id}/update`,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsInstalled],
      });
    },
  });
};

export const useReloadPlugin = () => {
  const queryClient = useQueryClient();

  return useMutation<Plugin, ShishoAPIError, { scope: string; id: string }>({
    mutationFn: ({ scope, id }) => {
      return API.request<Plugin>(
        "POST",
        `/plugins/installed/${scope}/${id}/reload`,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsInstalled],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginIdentifierTypes],
      });
    },
  });
};

export const useSetPluginOrder = () => {
  const queryClient = useQueryClient();

  return useMutation<
    void,
    ShishoAPIError,
    { hookType: string; order: { scope: string; id: string }[] }
  >({
    mutationFn: ({ hookType, order }) => {
      return API.request("PUT", `/plugins/order/${hookType}`, { order });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginOrder, variables.hookType],
      });
    },
  });
};

export const useAddRepository = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, { url: string; scope: string }>({
    mutationFn: (payload) => {
      return API.request("POST", "/plugins/repositories", payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginRepositories],
      });
    },
  });
};

export const useRemoveRepository = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, { scope: string }>({
    mutationFn: ({ scope }) => {
      return API.request("DELETE", `/plugins/repositories/${scope}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginRepositories],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsAvailable],
      });
    },
  });
};

export const useSavePluginConfig = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      scope,
      id,
      config,
      confidence_threshold,
      clear_confidence_threshold,
    }: {
      scope: string;
      id: string;
      config: Record<string, string>;
      confidence_threshold?: number | null;
      clear_confidence_threshold?: boolean;
    }) => {
      return API.request<Plugin>("PATCH", `/plugins/installed/${scope}/${id}`, {
        config,
        confidence_threshold,
        clear_confidence_threshold,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginConfig, variables.scope, variables.id],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsInstalled],
      });
    },
  });
};

export const useSavePluginFieldSettings = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      scope,
      id,
      fields,
    }: {
      scope: string;
      id: string;
      fields: Record<string, boolean>;
    }) => {
      return API.request("PUT", `/plugins/installed/${scope}/${id}/fields`, {
        fields,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginConfig, variables.scope, variables.id],
      });
    },
  });
};

export const useScanPlugins = () => {
  const queryClient = useQueryClient();
  return useMutation<Plugin[], ShishoAPIError>({
    mutationFn: () => {
      return API.request<Plugin[]>("POST", "/plugins/scan");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.PluginsInstalled] });
    },
  });
};

export const useSyncRepository = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, { scope: string }>({
    mutationFn: ({ scope }) => {
      return API.request("POST", `/plugins/repositories/${scope}/sync`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginRepositories],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsAvailable],
      });
    },
  });
};

// --- Per-Library Plugin Order ---

export interface LibraryPluginOrderPlugin {
  scope: string;
  id: string;
  name: string;
  enabled: boolean;
}

export interface LibraryPluginOrderResponse {
  customized: boolean;
  plugins: LibraryPluginOrderPlugin[];
}

export const useLibraryPluginOrder = (
  libraryId: string | undefined,
  hookType: string,
) => {
  return useQuery<LibraryPluginOrderResponse, ShishoAPIError>({
    queryKey: ["libraries", libraryId, "plugins", "order", hookType],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/libraries/${libraryId}/plugins/order/${hookType}`,
        null,
        null,
        signal,
      );
    },
    enabled: !!libraryId,
  });
};

export const useSetLibraryPluginOrder = () => {
  const queryClient = useQueryClient();
  return useMutation<
    void,
    ShishoAPIError,
    {
      libraryId: string;
      hookType: string;
      plugins: { scope: string; id: string; enabled: boolean }[];
    }
  >({
    mutationFn: ({ libraryId, hookType, plugins }) => {
      return API.request(
        "PUT",
        `/libraries/${libraryId}/plugins/order/${hookType}`,
        { plugins },
      );
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["libraries", variables.libraryId, "plugins", "order"],
      });
    },
  });
};

export const useResetLibraryPluginOrder = () => {
  const queryClient = useQueryClient();
  return useMutation<
    void,
    ShishoAPIError,
    {
      libraryId: string;
      hookType: string;
    }
  >({
    mutationFn: ({ libraryId, hookType }) => {
      return API.request(
        "DELETE",
        `/libraries/${libraryId}/plugins/order/${hookType}`,
      );
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["libraries", variables.libraryId, "plugins", "order"],
      });
    },
  });
};

export const useResetAllLibraryPluginOrders = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, { libraryId: string }>({
    mutationFn: ({ libraryId }) => {
      return API.request("DELETE", `/libraries/${libraryId}/plugins/order`);
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["libraries", variables.libraryId, "plugins", "order"],
      });
    },
  });
};

// --- Metadata Search & Apply ---

export interface PluginSearchResult {
  title: string;
  authors?: Array<{ name: string; role?: string }>;
  description?: string;
  release_date?: string;
  publisher?: string;
  subtitle?: string;
  series?: string;
  series_number?: number;
  genres?: string[];
  tags?: string[];
  narrators?: string[];
  identifiers?: Array<{ type: string; value: string }>;
  imprint?: string;
  url?: string;
  cover_url?: string;
  plugin_scope: string;
  plugin_id: string;
  disabled_fields?: string[];
  confidence?: number;
}

export interface PluginSearchResponse {
  results: PluginSearchResult[];
}

export const usePluginSearch = () => {
  return useMutation<
    PluginSearchResponse,
    ShishoAPIError,
    {
      query: string;
      bookId: number;
      author?: string;
      identifiers?: Array<{ type: string; value: string }>;
    }
  >({
    mutationFn: ({ query, bookId, author, identifiers }) => {
      return API.request<PluginSearchResponse>("POST", "/plugins/search", {
        query,
        book_id: bookId,
        author: author || undefined,
        identifiers: identifiers?.length ? identifiers : undefined,
      });
    },
  });
};

interface PluginApplyPayload {
  book_id: number;
  file_id?: number;
  fields: Record<string, unknown>;
  plugin_scope: string;
  plugin_id: string;
}

export const usePluginApply = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, PluginApplyPayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/plugins/apply", payload);
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: [BooksQueryKey.ListBooks],
        }),
        queryClient.invalidateQueries({
          queryKey: [BooksQueryKey.RetrieveBook],
        }),
      ]);
    },
  });
};
