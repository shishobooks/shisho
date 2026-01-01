import { BookPlus, RefreshCw, Settings } from "lucide-react";
import { useCallback } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import ThemeToggle from "@/components/library/ThemeToggle";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useCreateJob } from "@/hooks/queries/jobs";
import { useCreateLibrary, useLibrary } from "@/hooks/queries/libraries";
import { JobTypeScan } from "@/types";

const TopNav = () => {
  const { libraryId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const createJobMutation = useCreateJob();
  const createLibraryMutation = useCreateLibrary();
  const isDevelopment = import.meta.env.DEV;

  // Load current library if we have a libraryId
  const libraryQuery = useLibrary(libraryId, {
    enabled: Boolean(libraryId),
  });

  const handleCreateSync = useCallback(async () => {
    try {
      await createJobMutation.mutateAsync({
        payload: { type: JobTypeScan, data: {} },
      });
      toast.success("Sync started");
    } catch (e) {
      let msg = "Something went wrong.";
      if (e instanceof Error) {
        msg = e.message;
      }
      toast.error(msg);
    }
  }, [createJobMutation]);

  const handleCreateDefaultLibrary = useCallback(async () => {
    try {
      await createLibraryMutation.mutateAsync({
        payload: {
          name: "Main",
          library_paths: [
            "/Users/robinjoseph/code/personal/shisho/tmp/library",
          ],
        },
      });
      toast.success("Default library created!");
    } catch (e) {
      let msg = "Something went wrong.";
      if (e instanceof Error) {
        msg = e.message;
      }
      toast.error(msg);
    }
  }, [createLibraryMutation]);

  const isBooksActive =
    location.pathname === `/libraries/${libraryId}` ||
    (location.pathname.startsWith(`/libraries/${libraryId}/books`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/series`));
  const isSeriesActive = location.pathname.startsWith(
    `/libraries/${libraryId}/series`,
  );

  return (
    <div className="border-b border-border bg-background dark:bg-neutral-900">
      <div className="max-w-7xl mx-auto px-6">
        <div className="flex items-center justify-between h-16">
          <div className="flex items-center gap-8">
            <Link
              className="text-xl font-bold uppercase tracking-wider text-foreground hover:opacity-80 transition-opacity"
              to="/"
            >
              Shisho
              <span className="align-super text-xs font-normal text-muted-foreground dark:text-violet-300 ml-0.5">
                司書
              </span>
            </Link>
            {libraryQuery.data && (
              <div className="text-sm text-muted-foreground">
                {libraryQuery.data.name}
              </div>
            )}
            {libraryId && (
              <nav className="flex gap-1">
                <Button
                  className={`h-9 ${
                    isBooksActive
                      ? "bg-violet-300 text-neutral-900 hover:bg-violet-400 dark:bg-violet-300 dark:text-neutral-900 dark:hover:bg-violet-400"
                      : "hover:text-violet-300 dark:hover:text-violet-300"
                  }`}
                  onClick={() => {
                    // Clear series filter when clicking Books
                    navigate(`/libraries/${libraryId}`, { replace: true });
                  }}
                  variant={isBooksActive ? "default" : "ghost"}
                >
                  Books
                </Button>
                <Button
                  asChild
                  className={`h-9 ${
                    isSeriesActive
                      ? "bg-violet-300 text-neutral-900 hover:bg-violet-400 dark:bg-violet-300 dark:text-neutral-900 dark:hover:bg-violet-400"
                      : "hover:text-violet-300 dark:hover:text-violet-300"
                  }`}
                  variant={isSeriesActive ? "default" : "ghost"}
                >
                  <Link to={`/libraries/${libraryId}/series`}>Series</Link>
                </Button>
              </nav>
            )}
          </div>
          <div className="flex items-center gap-2">
            {isDevelopment && (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      className="h-9 w-9"
                      disabled={createLibraryMutation.isPending}
                      onClick={handleCreateDefaultLibrary}
                      size="icon"
                      variant="ghost"
                    >
                      <BookPlus className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Create default library</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            )}
            {libraryId && (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      asChild
                      className="h-9 w-9"
                      size="icon"
                      variant="ghost"
                    >
                      <Link to={`/libraries/${libraryId}/settings`}>
                        <Settings className="h-4 w-4" />
                      </Link>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Settings</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            )}
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    className="h-9 w-9"
                    disabled={createJobMutation.isPending}
                    onClick={handleCreateSync}
                    size="icon"
                    variant="ghost"
                  >
                    <RefreshCw
                      className={`h-4 w-4 ${
                        createJobMutation.isPending ? "animate-spin" : ""
                      }`}
                    />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>Sync libraries</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <ThemeToggle />
          </div>
        </div>
      </div>
    </div>
  );
};

export default TopNav;
