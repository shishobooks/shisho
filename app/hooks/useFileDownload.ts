import { useRef, useState } from "react";
import { toast } from "sonner";

import {
  API,
  requestErrorMessage,
  ShishoAPIError,
  toastRequestError,
} from "@/libraries/api";
import type { DeliveredFileType, File } from "@/types";
import {
  fileDownloadAsUrl,
  fileDownloadUrl,
  fileOriginalDownloadUrl,
} from "@/utils/downloadUrl";

export interface DownloadFailure {
  file: File;
  message: string;
}

/**
 * Replaces the authenticated download endpoints, as a Share Link page does.
 * A recipient has no unmodified download, so `downloadOriginal` falls back
 * to the file's own download.
 */
export interface DownloadEndpoints {
  downloadUrl: (file: File) => string;
  downloadAsUrl: (file: File, fileType: DeliveredFileType) => string;
}

/**
 * Downloads a file the way the book and file pages do: a HEAD request first,
 * so a file that fails to generate reports its error instead of saving an
 * error page, then a navigation that streams the file. A failure lands in
 * `failure` for DownloadErrorDialog, which offers the unmodified file.
 */
export const useFileDownload = (endpoints?: DownloadEndpoints) => {
  const [failure, setFailure] = useState<DownloadFailure | null>(null);
  const [downloadingFileId, setDownloadingFileId] = useState<number | null>(
    null,
  );
  const abortController = useRef<AbortController | null>(null);

  const downloadFrom = async (file: File, endpoint: string) => {
    setFailure(null);
    setDownloadingFileId(file.id);
    const controller = new AbortController();
    abortController.current = controller;

    try {
      // HEAD triggers generation and reports errors without loading the
      // whole file into the browser's memory.
      const headResponse = await fetch(endpoint, {
        method: "HEAD",
        signal: controller.signal,
      });

      if (!headResponse.ok) {
        // A HEAD response has no body, so GET the error message. The dialog
        // shows the API's message, or the fallback when a proxy answered or
        // the server failed without one (requestErrorMessage). A GET that
        // succeeds after all is not read, since its body is the whole file.
        const errorResponse = await fetch(endpoint, {
          signal: controller.signal,
        });
        let message = "Failed to download file";
        if (!errorResponse.ok) {
          try {
            await API.checkStatus(errorResponse);
          } catch (error) {
            // A cancel while reading the body is an AbortError, which the
            // outer catch ignores.
            if (!(error instanceof ShishoAPIError)) throw error;
            message = requestErrorMessage(error, message);
          }
        }
        setFailure({ file, message });
        return;
      }

      // The file is ready: stream it.
      window.location.assign(endpoint);
      toast.success("Download started");
    } catch (error) {
      // A cancel is the user's choice, not a failure.
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      console.error("Download error:", error);
      toastRequestError(error, "Failed to download file");
    } finally {
      abortController.current = null;
      setDownloadingFileId(null);
    }
  };

  /** The file as `deliveredFileType`, the type its download button names. */
  const download = (file: File, deliveredFileType: DeliveredFileType) =>
    downloadFrom(
      file,
      endpoints
        ? endpoints.downloadUrl(file)
        : deliveredFileType === file.file_type
          ? fileDownloadUrl(file.id)
          : fileDownloadAsUrl(file.id, deliveredFileType),
    );

  /** A download menu entry: the file as one File Type. */
  const downloadAs = (file: File, fileType: DeliveredFileType) =>
    downloadFrom(
      file,
      endpoints
        ? endpoints.downloadAsUrl(file, fileType)
        : fileDownloadAsUrl(file.id, fileType),
    );

  /** The file as it is on disk, which cannot fail to generate. */
  const downloadOriginal = (file: File) => {
    if (endpoints) {
      void download(file, file.file_type);
      return;
    }
    window.location.assign(fileOriginalDownloadUrl(file.id));
    setFailure(null);
  };

  const cancel = () => {
    abortController.current?.abort();
    abortController.current = null;
    setDownloadingFileId(null);
  };

  return {
    failure,
    clearFailure: () => setFailure(null),
    downloadingFileId,
    download,
    downloadAs,
    downloadOriginal,
    cancel,
  };
};
