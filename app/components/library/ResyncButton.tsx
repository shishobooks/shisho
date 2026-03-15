import { formatDistanceToNow } from "date-fns";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useCreateJob, useLatestScanJob } from "@/hooks/queries/jobs";

interface ResyncButtonProps {
  libraryId: number;
}

export function ResyncButton({ libraryId }: ResyncButtonProps) {
  const navigate = useNavigate();
  const { data, isLoading } = useLatestScanJob(libraryId);
  const createJob = useCreateJob();

  const latestJob = data?.jobs[0];
  const isActive =
    latestJob?.status === "pending" || latestJob?.status === "in_progress";
  const isFailed = latestJob?.status === "failed";
  const isCompleted = latestJob?.status === "completed";
  // Show spinning when mutation is pending or job is active
  const showSpinning = createJob.isPending || isActive;

  const handleClick = () => {
    if (isActive || isFailed) {
      navigate(`/settings/jobs/${latestJob?.id}`);
    } else {
      createJob.mutate({
        payload: { type: "scan", library_id: libraryId, data: {} },
      });
    }
  };

  const getTooltipContent = () => {
    if (createJob.isPending) {
      return "Starting scan...";
    }
    if (isActive && latestJob) {
      return `Scan started ${formatDistanceToNow(new Date(latestJob.created_at), { addSuffix: true })}`;
    }
    if (isFailed) {
      return "Last scan failed - view logs";
    }
    if (isCompleted && latestJob) {
      return `Last synced ${formatDistanceToNow(new Date(latestJob.updated_at), { addSuffix: true })}`;
    }
    return "Resync library";
  };

  if (isLoading) {
    return (
      <Button className="h-9 w-9" disabled size="icon" variant="ghost">
        <RefreshCw className="h-4 w-4" />
      </Button>
    );
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            className="h-9 w-9 relative cursor-pointer"
            disabled={createJob.isPending}
            onClick={handleClick}
            size="icon"
            variant="ghost"
          >
            <RefreshCw
              className={`h-4 w-4 ${showSpinning ? "animate-spin" : ""}`}
            />
            {isFailed && (
              <AlertTriangle className="h-3 w-3 text-yellow-500 absolute -top-0.5 -right-0.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>{getTooltipContent()}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
