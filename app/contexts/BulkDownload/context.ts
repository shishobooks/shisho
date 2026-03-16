import { createContext } from "react";

export interface BulkDownloadProgress {
  jobId: number;
  status: "generating" | "zipping" | "completed" | "failed";
  current: number;
  total: number;
  estimatedSizeBytes: number;
  dismissed: boolean;
}

export interface BulkDownloadContextValue {
  activeDownload: BulkDownloadProgress | null;
  startDownload: (
    jobId: number,
    total: number,
    estimatedSizeBytes: number,
  ) => void;
  updateProgress: (
    jobId: number,
    status: string,
    current: number,
    total: number,
    estimatedSizeBytes: number,
  ) => void;
  completeDownload: (jobId: number) => void;
  failDownload: (jobId: number) => void;
  dismissDownload: () => void;
}

export const BulkDownloadContext =
  createContext<BulkDownloadContextValue | null>(null);
