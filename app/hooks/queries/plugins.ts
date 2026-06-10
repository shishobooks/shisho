import {
  keepPreviousData,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { QueryKey as GenresQueryKey } from "@/hooks/queries/genres";
import { QueryKey as PeopleQueryKey } from "@/hooks/queries/people";
import { QueryKey as PublishersQueryKey } from "@/hooks/queries/publishers";
import { QueryKey as SeriesQueryKey } from "@/hooks/queries/series";
import { QueryKey as TagsQueryKey } from "@/hooks/queries/tags";
import { API, type ShishoAPIError } from "@/libraries/api";
import type {
  AddPluginRepositoryPayload,
  AvailablePlugin,
  InstallPluginPayload,
  LibraryPluginOrderResponse,
  Plugin,
  PluginApplyPayload,
  PluginConfigResponse,
  PluginHookConfig,
  PluginIdentifierType,
  PluginRepository,
  PluginSearchPayload,
  PluginSearchResponse,
  SetLibraryPluginOrderPayload,
  SetPluginFieldSettingsPayload,
  SetPluginOrderPayload,
  SyncRepositoryResponse,
  UpdatePluginPayload,
} from "@/types";

// Re-export generated types so consumers can import from this module.
// PluginHookConfig is re-exported as PluginOrder for backward compatibility.
// The wire types (AvailablePlugin, PluginSearchResult, ...) are generated
// from Go via tygo and re-exported (aliased) from @/types — never hand-define
// them here (ADR 0004).
export type {
  AvailablePlugin,
  ConfigField,
  ConfigSchema,
  InstallPluginPayload,
  LibraryPluginOrderPlugin,
  LibraryPluginOrderResponse,
  Plugin,
  PluginApplyPayload,
  PluginCapabilities,
  PluginConfigResponse,
  PluginHookConfig as PluginOrder,
  PluginHookType,
  PluginMode,
  PluginRepository,
  PluginSearchError,
  PluginSearchResponse,
  PluginSearchResult,
  PluginSearchSkipped,
  PluginStatus,
  PluginVersion,
  SyncRepositoryResponse,
  UpdatePluginPayload,
} from "@/types";
export {
  PluginStatusActive,
  PluginStatusDisabled,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
} from "@/types";

// --- Query Keys ---

export enum QueryKey {
  PluginsInstalled = "PluginsInstalled",
  PluginsAvailable = "PluginsAvailable",
  PluginConfig = "PluginConfig",
  PluginOrder = "PluginOrder",
  PluginRepositories = "PluginRepositories",
  PluginIdentifierTypes = "PluginIdentifierTypes",
  PluginSearch = "PluginSearch",
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

const pluginOrderQuery = (hookType: string) => ({
  queryKey: [QueryKey.PluginOrder, hookType],
  queryFn: ({ signal }: { signal: AbortSignal }) => {
    return API.request<PluginHookConfig[]>(
      "GET",
      `/plugins/order/${hookType}`,
      null,
      null,
      signal,
    );
  },
});

export const usePluginOrder = (hookType: string) => {
  return useQuery<PluginHookConfig[], ShishoAPIError>(
    pluginOrderQuery(hookType),
  );
};

export const useAllPluginOrders = (hookTypes: string[]) => {
  return useQueries({
    queries: hookTypes.map((ht) => pluginOrderQuery(ht)),
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

export const usePluginManifest = (
  scope: string | undefined,
  id: string | undefined,
  options: { enabled?: boolean } = {},
) => {
  return useQuery<unknown, ShishoAPIError>({
    queryKey: ["plugins", "manifest", scope, id],
    enabled: !!scope && !!id && options.enabled !== false,
    queryFn: ({ signal }) => {
      return API.request<unknown>(
        "GET",
        `/plugins/installed/${scope}/${id}/manifest`,
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
      // The backend removes the plugin from every hook's global order, so the
      // AdvancedOrderSection list must refetch — otherwise it keeps showing
      // the now-uninstalled plugin until the user navigates away.
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginOrder],
      });
      // Library-scoped orders (LibraryPluginsTab) are keyed under
      // ["libraries", libraryId, "plugins", "order", hookType]. Invalidate
      // only those — the shared "libraries" prefix is used by other hooks
      // too (books, settings) and blanket-invalidating it would trigger
      // unrelated refetches on every uninstall.
      queryClient.invalidateQueries({
        predicate: (query) => {
          const key = query.queryKey;
          return (
            Array.isArray(key) &&
            key[0] === "libraries" &&
            key[2] === "plugins" &&
            key[3] === "order"
          );
        },
      });
    },
  });
};

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
    // A failed enable still mutates server state (Malfunctioned status +
    // load_error get persisted), so the detail page needs PluginsInstalled
    // refetched on error too — otherwise the error alert doesn't appear
    // until manual reload.
    onError: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsInstalled],
      });
    },
    // PluginConfig: the config endpoint returns an empty schema while the
    // runtime is unloaded, so toggling enabled (or saving config) must
    // reload the form's schema/values. The first time a plugin is enabled,
    // LoadPlugin also registers identifier types and appends the plugin to
    // each hook's order (global + per-library fallback), so those caches
    // need to refetch too. None of these change on a failed enable, so
    // they're scoped to onSuccess.
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginsInstalled],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginConfig],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginIdentifierTypes],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginOrder],
      });
      // Library-scoped orders (LibraryPluginsTab) are keyed under
      // ["libraries", libraryId, "plugins", "order", hookType]. Invalidate
      // only those — the shared "libraries" prefix is used by other hooks
      // too (books, settings) and blanket-invalidating it would trigger
      // unrelated refetches on every enable/disable.
      queryClient.invalidateQueries({
        predicate: (query) => {
          const key = query.queryKey;
          return (
            Array.isArray(key) &&
            key[0] === "libraries" &&
            key[2] === "plugins" &&
            key[3] === "order"
          );
        },
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
    { hookType: string; order: SetPluginOrderPayload["order"] }
  >({
    mutationFn: ({ hookType, order }) => {
      const payload: SetPluginOrderPayload = { order };
      return API.request("PUT", `/plugins/order/${hookType}`, payload);
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

  return useMutation<void, ShishoAPIError, AddPluginRepositoryPayload>({
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
    }: { scope: string; id: string } & Pick<
      UpdatePluginPayload,
      "config" | "confidence_threshold" | "clear_confidence_threshold"
    >) => {
      const payload: UpdatePluginPayload = {
        config,
        confidence_threshold,
        clear_confidence_threshold,
      };
      return API.request<Plugin>(
        "PATCH",
        `/plugins/installed/${scope}/${id}`,
        payload,
      );
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
      fields: SetPluginFieldSettingsPayload["fields"];
    }) => {
      const payload: SetPluginFieldSettingsPayload = { fields };
      return API.request(
        "PUT",
        `/plugins/installed/${scope}/${id}/fields`,
        payload,
      );
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

  return useMutation<SyncRepositoryResponse, ShishoAPIError, { scope: string }>(
    {
      mutationFn: ({ scope }) => {
        return API.request<SyncRepositoryResponse>(
          "POST",
          `/plugins/repositories/${scope}/sync`,
        );
      },
      onSuccess: () => {
        queryClient.invalidateQueries({
          queryKey: [QueryKey.PluginRepositories],
        });
        queryClient.invalidateQueries({
          queryKey: [QueryKey.PluginsAvailable],
        });
        queryClient.invalidateQueries({
          queryKey: [QueryKey.PluginsInstalled],
        });
      },
    },
  );
};

// --- Per-Library Plugin Order ---

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
      plugins: SetLibraryPluginOrderPayload["plugins"];
    }
  >({
    mutationFn: ({ libraryId, hookType, plugins }) => {
      const payload: SetLibraryPluginOrderPayload = { plugins };
      return API.request(
        "PUT",
        `/libraries/${libraryId}/plugins/order/${hookType}`,
        payload,
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

// PluginSearchParams is the immutable snapshot of a submitted Identify search.
// The dialog holds one of these (distinct from the live input fields) and the
// query is keyed on it, so submitting a new search supersedes any in-flight
// one: TanStack Query aborts the previous query's request (via its signal) and
// the displayed results are always those of the most recently submitted key.
export interface PluginSearchParams {
  query: string;
  bookId: number;
  fileId?: number;
  author?: string;
  identifiers?: Array<{ type: string; value: string }>;
}

// usePluginSearch models the Identify search as a query (not a mutation): the
// backend POST /plugins/search performs no writes to Shisho state (it only
// fans out to plugins), so it's semantically a read. As a query it gets request
// sequencing and an AbortSignal for free (a mutation's mutationFn gets neither
// in TanStack Query v5). The POST method and request payload shape are
// unchanged. `params` is null until a search has been submitted; the query is
// disabled until there's a non-empty query string.
//
// `placeholderData: keepPreviousData` keeps the previous key's results mounted
// while a new search loads, avoiding a dialog resize. Loading/dimming is driven
// off `isFetching` by the caller, not `isPlaceholderData`. `isFetching` is
// true across new-key loads, identical-resubmit refetches, and stale-key
// background refetches, whereas `isPlaceholderData` misses the latter two.
export const usePluginSearch = (params: PluginSearchParams | null) => {
  return useQuery<PluginSearchResponse, ShishoAPIError>({
    enabled: Boolean(params && params.query),
    queryKey: [QueryKey.PluginSearch, params],
    queryFn: ({ signal }) => {
      // params is non-null here because the query is disabled otherwise.
      const p = params as PluginSearchParams;
      const payload: PluginSearchPayload = {
        query: p.query,
        book_id: p.bookId,
        file_id: p.fileId,
        author: p.author || undefined,
        identifiers: p.identifiers?.length ? p.identifiers : undefined,
      };
      return API.request<PluginSearchResponse>(
        "POST",
        "/plugins/search",
        payload,
        null,
        signal,
      );
    },
    placeholderData: keepPreviousData,
  });
};

export const usePluginApply = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, PluginApplyPayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/plugins/apply", payload);
    },
    onSuccess: async () => {
      // Apply accepts entity name strings; the server creates new persons,
      // series, publishers, genres, or tags as needed. Invalidate
      // those caches so newly-created entities show up in admin pages and
      // combobox results.
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: [BooksQueryKey.ListBooks],
        }),
        queryClient.invalidateQueries({
          queryKey: [BooksQueryKey.RetrieveBook],
        }),
        queryClient.invalidateQueries({
          queryKey: [PeopleQueryKey.ListPeople],
        }),
        queryClient.invalidateQueries({
          queryKey: [SeriesQueryKey.ListSeries],
        }),
        queryClient.invalidateQueries({
          queryKey: [PublishersQueryKey.ListPublishers],
        }),
        queryClient.invalidateQueries({
          queryKey: [GenresQueryKey.ListGenres],
        }),
        queryClient.invalidateQueries({
          queryKey: [TagsQueryKey.ListTags],
        }),
      ]);
    },
  });
};
