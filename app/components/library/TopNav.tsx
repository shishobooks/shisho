import { BookPlus, RefreshCw, Settings } from "lucide-react";
import { useCallback } from "react";
import { Link, useLocation } from "react-router-dom";
import { toast } from "sonner";

import ThemeToggle from "@/components/library/ThemeToggle";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useCreateJob } from "@/hooks/queries/jobs";
import { useCreateLibrary } from "@/hooks/queries/libraries";
import { JobTypeScan } from "@/types";

const TopNav = () => {
  const location = useLocation();
  const createJobMutation = useCreateJob();
  const createLibraryMutation = useCreateLibrary();

  const handleCreateSync = useCallback(async () => {
    try {
      await createJobMutation.mutateAsync({
        payload: { type: JobTypeScan, data: {} },
      });
      toast.success("Sync created!");
    } catch (e) {
      let msg = "Something went wrong.";
      if (e instanceof Error) {
        msg = e.message;
      }
      toast.error(msg);
    }
  }, [createJobMutation]);

  const handleCreateLibrary = useCallback(async () => {
    try {
      await createLibraryMutation.mutateAsync({
        payload: {
          name: "Books",
          library_paths: [
            "/Users/robinjoseph/code/personal/shisho/tmp/library",
          ],
        },
      });
      toast.success("Library created!");
    } catch (e) {
      let msg = "Something went wrong.";
      if (e instanceof Error) {
        msg = e.message;
      }
      toast.error(msg);
    }
  }, [createLibraryMutation]);

  return (
    <div className="bg-violet-300 px-6 py-4 flex items-center justify-between dark:bg-neutral-700 text-neutral-900 dark:text-neutral-50">
      <div className="flex items-center gap-8">
        <Link
          className="font-sans text-2xl font-black uppercase tracking-wider"
          to="/"
        >
          Shisho
          <span className="align-super text-xs font-normal dark:text-violet-300">
            司書
          </span>
        </Link>
        <nav className="flex gap-6">
          <Link
            className={`font-medium hover:text-violet-600 dark:hover:text-violet-300 transition-colors ${
              location.pathname === "/"
                ? "text-violet-600 dark:text-violet-300"
                : ""
            }`}
            to="/"
          >
            Books
          </Link>
          <Link
            className={`font-medium hover:text-violet-600 dark:hover:text-violet-300 transition-colors ${
              location.pathname.startsWith("/series")
                ? "text-violet-600 dark:text-violet-300"
                : ""
            }`}
            to="/series"
          >
            Series
          </Link>
        </nav>
      </div>
      <div className="flex gap-6">
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger>
              <Link to="/config">
                <Settings
                  className={`cursor-pointer ${
                    location.pathname === "/config"
                      ? "text-violet-600 dark:text-violet-300"
                      : ""
                  }`}
                />
              </Link>
            </TooltipTrigger>
            <TooltipContent>
              <p>Configuration</p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
        <ThemeToggle />
        {/*<TooltipProvider>*/}
        {/*  <Tooltip>*/}
        {/*    <TooltipTrigger>*/}
        {/*      {mode === "light" ? (*/}
        {/*        <Sun*/}
        {/*          className="cursor-pointer"*/}
        {/*          onClick={handleToggleDarkMode}*/}
        {/*        />*/}
        {/*      ) : (*/}
        {/*        <Moon*/}
        {/*          className="cursor-pointer"*/}
        {/*          onClick={handleToggleDarkMode}*/}
        {/*        />*/}
        {/*      )}*/}
        {/*    </TooltipTrigger>*/}
        {/*    <TooltipContent>*/}
        {/*      <p>Toggle to {mode === "light" ? "dark" : "light"} mode</p>*/}
        {/*    </TooltipContent>*/}
        {/*  </Tooltip>*/}
        {/*</TooltipProvider>*/}
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger>
              <BookPlus
                className="cursor-pointer"
                onClick={handleCreateLibrary}
              />
            </TooltipTrigger>
            <TooltipContent>
              <p>Create default library</p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger>
              <RefreshCw
                className="cursor-pointer"
                onClick={handleCreateSync}
              />
            </TooltipTrigger>
            <TooltipContent>
              <p>Sync libraries</p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>
    </div>
  );
};

export default TopNav;
