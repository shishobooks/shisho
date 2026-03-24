import {
  BulkDownloadContext,
  type BulkDownloadContextValue,
  type BulkDownloadProgress,
} from "./context";
import { useCallback, useMemo, useState, type ReactNode } from "react";

export const BulkDownloadProvider = ({ children }: { children: ReactNode }) => {
  const [activeDownload, setActiveDownload] =
    useState<BulkDownloadProgress | null>(null);

  const startDownload = useCallback(
    (jobId: number, total: number, estimatedSizeBytes: number) => {
      setActiveDownload({
        jobId,
        status: "generating",
        current: 0,
        total,
        estimatedSizeBytes,
        dismissed: false,
      });
    },
    [],
  );

  const updateProgress = useCallback(
    (
      jobId: number,
      status: string,
      current: number,
      total: number,
      estimatedSizeBytes: number,
    ) => {
      setActiveDownload((prev) => {
        if (!prev || prev.jobId !== jobId) return prev;
        return {
          ...prev,
          status: status as BulkDownloadProgress["status"],
          current,
          total,
          estimatedSizeBytes,
        };
      });
    },
    [],
  );

  const completeDownload = useCallback((jobId: number) => {
    setActiveDownload((prev) => {
      if (!prev || prev.jobId !== jobId) return prev;
      return { ...prev, status: "completed", dismissed: false };
    });
  }, []);

  const failDownload = useCallback((jobId: number) => {
    setActiveDownload((prev) => {
      if (!prev || prev.jobId !== jobId) return prev;
      return { ...prev, status: "failed", dismissed: false };
    });
  }, []);

  const dismissDownload = useCallback(() => {
    setActiveDownload((prev) => {
      if (!prev) return prev;
      if (prev.status === "completed" || prev.status === "failed") {
        return null;
      }
      return { ...prev, dismissed: true };
    });
  }, []);

  const value: BulkDownloadContextValue = useMemo(
    () => ({
      activeDownload,
      startDownload,
      updateProgress,
      completeDownload,
      failDownload,
      dismissDownload,
    }),
    [
      activeDownload,
      startDownload,
      updateProgress,
      completeDownload,
      failDownload,
      dismissDownload,
    ],
  );

  return (
    <BulkDownloadContext.Provider value={value}>
      {children}
    </BulkDownloadContext.Provider>
  );
};
