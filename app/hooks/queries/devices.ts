import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  Book,
  BookDevice,
  DeliveryPreviewQuery,
  DeliveryPreviewResponse,
  Rule,
} from "@/types";
import type {
  AdminDeviceResponse,
  CreateDevicePayload,
  DeliveryRulesMode,
  DeviceResponse,
  DeviceShortURL,
  UpdateDevicePayload,
} from "@/types/generated/devices";
import type {
  DismissAllPayload,
  LinkCandidate,
  LinkCandidatesQuery,
  LinkPayload,
  UnrecognizedBook,
} from "@/types/generated/koreaderplugin";

import { QueryKey as BooksQueryKey } from "./books";
import { useRequires } from "./permissions";
import {
  invalidateReadingRecords,
  QueryKey as ReadingQueryKey,
} from "./reading";
import { QueryKey as ServerStatsQueryKey } from "./serverStats";

export enum QueryKey {
  ListDevices = "ListDevices",
  RetrieveDevice = "RetrieveDevice",
  ListAllDevices = "ListAllDevices",
  DeliveryPreview = "DeliveryPreview",
  BookDevices = "BookDevices",
  KOReaderUnrecognizedBooks = "KOReaderUnrecognizedBooks",
  KOReaderLinkCandidates = "KOReaderLinkCandidates",
}

// The caller's own Devices. Any signed-in user manages their own, so the
// routes need no permission.
export const useDevices = () => {
  return useQuery<DeviceResponse[], ShishoAPIError>({
    queryKey: [QueryKey.ListDevices],
    queryFn: ({ signal }) =>
      API.request("GET", "/user/devices", null, null, signal),
  });
};

// Fetches the caller's Devices outside a component, such as when a save
// needs them, from the cache when they are loaded.
export const fetchDevices = (queryClient: QueryClient) =>
  queryClient.fetchQuery<DeviceResponse[], ShishoAPIError>({
    queryKey: [QueryKey.ListDevices],
    queryFn: ({ signal }) =>
      API.request("GET", "/user/devices", null, null, signal),
  });

export const useDevice = (id?: string) => {
  return useQuery<DeviceResponse, ShishoAPIError>({
    enabled: Boolean(id),
    queryKey: [QueryKey.RetrieveDevice, id],
    queryFn: ({ signal }) =>
      API.request("GET", `/user/devices/${id}`, null, null, signal),
  });
};

// Every user's Devices, for admins. Never carries a credential.
export const useAllDevices = () => {
  return useQuery<AdminDeviceResponse[], ShishoAPIError>({
    enabled: useRequires("users:read", true),
    queryKey: [QueryKey.ListAllDevices],
    queryFn: ({ signal }) => API.request("GET", "/devices", null, null, signal),
  });
};

const useInvalidateDevices = () => {
  const queryClient = useQueryClient();
  return () => {
    queryClient.invalidateQueries({ queryKey: [QueryKey.ListDevices] });
    queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveDevice] });
    queryClient.invalidateQueries({ queryKey: [QueryKey.ListAllDevices] });
    queryClient.invalidateQueries({ queryKey: [QueryKey.DeliveryPreview] });
    queryClient.invalidateQueries({ queryKey: [QueryKey.BookDevices] });
    // Reading Stats names each Device the user read on.
    queryClient.invalidateQueries({ queryKey: [ReadingQueryKey.ReadingStats] });
  };
};

export const useCreateDevice = () => {
  const invalidate = useInvalidateDevices();
  return useMutation<DeviceResponse, ShishoAPIError, CreateDevicePayload>({
    mutationFn: (payload) => API.request("POST", "/user/devices", payload),
    onSuccess: invalidate,
  });
};

export const useUpdateDevice = () => {
  const queryClient = useQueryClient();
  const invalidate = useInvalidateDevices();
  return useMutation<
    DeviceResponse,
    ShishoAPIError,
    { id: string; payload: UpdateDevicePayload }
  >({
    mutationFn: ({ id, payload }) =>
      API.request("PATCH", `/user/devices/${id}`, payload),
    // The saved Device shows at once, so an editor reset to it never reads
    // the stale one while the refetch runs.
    onSuccess: (device) => {
      queryClient.setQueryData([QueryKey.RetrieveDevice, device.id], device);
      invalidate();
    },
  });
};

export const useRegenerateDeviceCredential = () => {
  const invalidate = useInvalidateDevices();
  return useMutation<DeviceResponse, ShishoAPIError, string>({
    mutationFn: (id) => API.request("POST", `/user/devices/${id}/credential`),
    onSuccess: invalidate,
  });
};

export const useDeleteDevice = () => {
  const invalidate = useInvalidateDevices();
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.request("DELETE", `/user/devices/${id}`),
    onSuccess: invalidate,
  });
};

export const useRevokeDevice = () => {
  const invalidate = useInvalidateDevices();
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.request("DELETE", `/devices/${id}`),
    onSuccess: invalidate,
  });
};

export const useGenerateDeviceShortUrl = () => {
  return useMutation<DeviceShortURL, ShishoAPIError, string>({
    mutationFn: (id) => API.request("POST", `/user/devices/${id}/short-url`),
  });
};

/**
 * Downloads the KOReader plugin for a KOReader Device. The zip carries this
 * server's URL and a fresh pairing code, which replaces the Device's earlier
 * one, so the plugin signs itself in if it's installed within 30 minutes.
 */
export const useDownloadKOReaderPlugin = () => {
  return useMutation<Blob, ShishoAPIError, string>({
    mutationFn: async (id) => {
      const response = await fetch(`/api/user/devices/${id}/koreader-plugin`, {
        method: "POST",
      });
      if (!response.ok) {
        // Throws the server's error.
        await API.checkStatus(response);
      }
      return response.blob();
    },
  });
};

export const useClearKoboSync = () => {
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.request("DELETE", `/user/devices/${id}/kobo-sync`),
  });
};

/**
 * A change to a Device's Delivery Rules to preview before saving it: a mode
 * to follow, with the custom list when the mode is custom.
 */
export interface RulesProposal {
  mode: DeliveryRulesMode;
  rules?: Rule[];
}

const previewQuery = (
  proposal: RulesProposal | undefined,
): DeliveryPreviewQuery => {
  if (!proposal) return {};
  return {
    mode: proposal.mode,
    rules: proposal.rules
      ?.map((r) => `${r.source_file_type}:${r.delivered_file_type}`)
      .join(","),
  };
};

const deliveryPreviewOptions = (
  id: string,
  proposal: RulesProposal | undefined,
) => {
  const query = previewQuery(proposal);
  return {
    queryKey: [QueryKey.DeliveryPreview, id, query],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      API.request<DeliveryPreviewResponse>(
        "GET",
        `/user/devices/${id}/delivery-preview`,
        null,
        query,
        signal,
      ),
  };
};

// What a Device receives under the rules it follows, or under a proposed
// change. It depends on the library and on what a Kobo last synced, which
// change without any mutation here, so it is always refetched when shown.
export const useDeliveryPreview = (id: string, proposal?: RulesProposal) => {
  return useQuery<DeliveryPreviewResponse, ShishoAPIError>({
    ...deliveryPreviewOptions(id, proposal),
    staleTime: 0,
    // Editing a custom list changes the key on every move; keep showing the
    // last counts until the new ones arrive.
    placeholderData: keepPreviousData,
  });
};

// Fetches the preview of a proposed change, for a warning before saving it.
export const fetchDeliveryPreview = (
  queryClient: QueryClient,
  id: string,
  proposal: RulesProposal,
) =>
  queryClient.fetchQuery<DeliveryPreviewResponse, ShishoAPIError>({
    ...deliveryPreviewOptions(id, proposal),
    staleTime: 0,
  });

// What each of the caller's Devices gets of a book. Like the preview, it
// depends on what a Kobo last synced, so it is refetched when shown. What a
// Device gets follows the book's Files, so the key describes them: a File
// added, removed, or moved, from the book page or a scan, refetches it.
export const useBookDevices = (
  book: Pick<Book, "id" | "files">,
  options: { enabled?: boolean } = {},
) => {
  const filesKey = (book.files ?? [])
    .map((f) => `${f.id}:${f.file_type}:${f.file_role}`)
    .join(",");
  return useQuery<BookDevice[], ShishoAPIError>({
    enabled: useRequires("books:read", options.enabled ?? true),
    staleTime: 0,
    queryKey: [QueryKey.BookDevices, book.id, filesKey],
    queryFn: ({ signal }) =>
      API.request("GET", `/books/${book.id}/devices`, null, null, signal),
  });
};

// A KOReader Device's unrecognized books. The KOReader plugin adds to them on
// any sync, which changes nothing here, so they are refetched when shown.
// They name the library's Books, so the routes need permission to read them.
export const useKOReaderUnrecognizedBooks = (
  deviceId: string,
  options: { enabled?: boolean } = {},
) => {
  return useQuery<UnrecognizedBook[], ShishoAPIError>({
    enabled: useRequires("books:read", options.enabled ?? true),
    staleTime: 0,
    queryKey: [QueryKey.KOReaderUnrecognizedBooks, deviceId],
    queryFn: ({ signal }) =>
      API.request(
        "GET",
        `/user/devices/${deviceId}/koreader-unrecognized`,
        null,
        null,
        signal,
      ),
  });
};

// Files the Device reaches that an unrecognized book could be linked to,
// for when none of its suggestions fits.
export const useKOReaderLinkCandidates = (
  deviceId: string,
  q: string,
  options: { enabled?: boolean } = {},
) => {
  return useQuery<LinkCandidate[], ShishoAPIError>({
    enabled: useRequires(
      "books:read",
      (options.enabled ?? true) && q.trim() !== "",
    ),
    queryKey: [QueryKey.KOReaderLinkCandidates, deviceId, q],
    queryFn: ({ signal }) =>
      API.request<LinkCandidate[], null, LinkCandidatesQuery>(
        "GET",
        `/user/devices/${deviceId}/koreader-unrecognized/candidates`,
        null,
        { q },
        signal,
      ),
    placeholderData: keepPreviousData,
  });
};

interface LinkUnrecognizedVariables {
  deviceId: string;
  document: string;
  fileId: number;
}

// Links an unrecognized book to a File. The server applies the reading data it
// held for it at once: Reading Sessions, Read-throughs, status, Rating, and a
// position. Those show on the Book's page, book cards, Reading History and
// Stats, and Server Stats, and a finish can move every Book on To Read.
export const useLinkKOReaderUnrecognized = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, LinkUnrecognizedVariables>({
    mutationFn: ({ deviceId, document, fileId }) =>
      API.request<void, LinkPayload>(
        "POST",
        `/user/devices/${deviceId}/koreader-unrecognized/${document}/link`,
        { file_id: fileId },
      ),
    onSuccess: (_data, { deviceId }) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.KOReaderUnrecognizedBooks, deviceId],
      });
      queryClient.invalidateQueries({
        queryKey: [ReadingQueryKey.BookReading],
      });
      queryClient.invalidateQueries({
        queryKey: [ReadingQueryKey.FilePosition],
      });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      invalidateReadingRecords(queryClient);
      queryClient.invalidateQueries({
        queryKey: [ServerStatsQueryKey.ServerStats],
      });
    },
  });
};

interface DismissUnrecognizedVariables {
  deviceId: string;
  document: string;
}

// Dismisses an unrecognized book, deleting the reading data held for it.
export const useDismissKOReaderUnrecognized = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, DismissUnrecognizedVariables>({
    mutationFn: ({ deviceId, document }) =>
      API.request(
        "POST",
        `/user/devices/${deviceId}/koreader-unrecognized/${document}/dismiss`,
      ),
    onSuccess: (_data, { deviceId }) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.KOReaderUnrecognizedBooks, deviceId],
      });
    },
  });
};

interface UnlinkUnrecognizedVariables {
  deviceId: string;
  document: string;
}

// Undoes a link, so the copy is listed to link again. The link is the
// user's, so the copy goes back on every one of their Devices' lists. What
// the link added to reading stays.
export const useUnlinkKOReaderUnrecognized = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, UnlinkUnrecognizedVariables>({
    mutationFn: ({ deviceId, document }) =>
      API.request(
        "POST",
        `/user/devices/${deviceId}/koreader-unrecognized/${document}/unlink`,
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.KOReaderUnrecognizedBooks],
      });
    },
  });
};

// Dismisses the books on a Device still to link that its page listed,
// deleting the reading data held for them. Linked books stay.
export const useDismissAllKOReaderUnrecognized = () => {
  const queryClient = useQueryClient();
  return useMutation<
    void,
    ShishoAPIError,
    { deviceId: string; documents: string[] }
  >({
    mutationFn: ({ deviceId, documents }) =>
      API.request<void, DismissAllPayload>(
        "POST",
        `/user/devices/${deviceId}/koreader-unrecognized/dismiss-all`,
        { documents },
      ),
    onSuccess: (_data, { deviceId }) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.KOReaderUnrecognizedBooks, deviceId],
      });
    },
  });
};
