import { Download, Loader2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { useBulkDownload } from "@/hooks/useBulkDownload";
import { formatFileSize } from "@/utils/format";

export const BulkDownloadToast = () => {
  const { activeDownload, dismissDownload } = useBulkDownload();

  if (!activeDownload || activeDownload.dismissed) {
    return null;
  }

  const { jobId, status, current, total, estimatedSizeBytes } = activeDownload;
  const progressPercent = total > 0 ? Math.round((current / total) * 100) : 0;

  return (
    <div className="fixed bottom-4 right-4 z-50 bg-background border rounded-lg shadow-lg p-4 w-80">
      <div className="flex items-start justify-between gap-2 mb-2">
        <div className="flex items-center gap-2 text-sm font-medium">
          {status === "completed" ? (
            <Download className="h-4 w-4 text-green-600" />
          ) : status === "failed" ? (
            <X className="h-4 w-4 text-destructive" />
          ) : (
            <Loader2 className="h-4 w-4 animate-spin" />
          )}
          <span>
            {status === "completed"
              ? "Download ready"
              : status === "failed"
                ? "Download failed"
                : status === "zipping"
                  ? "Creating zip file..."
                  : `Preparing ${current} of ${total} files...`}
          </span>
        </div>
        <Button
          className="h-6 w-6 shrink-0"
          onClick={dismissDownload}
          size="icon"
          variant="ghost"
        >
          <X className="h-3 w-3" />
        </Button>
      </div>

      {(status === "generating" || status === "zipping") && (
        <Progress className="h-2 mb-2" value={progressPercent} />
      )}

      <div className="text-xs text-muted-foreground">
        {formatFileSize(estimatedSizeBytes)}
      </div>

      {status === "completed" && (
        <Button asChild className="w-full mt-2" size="sm">
          <a href={`/api/jobs/${jobId}/download`}>
            <Download className="h-4 w-4" />
            Download Zip
          </a>
        </Button>
      )}
    </div>
  );
};
