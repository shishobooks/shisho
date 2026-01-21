import { Check, ChevronDown, Library, List } from "lucide-react";
import { useCallback } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";

const LibraryListPicker = () => {
  const { libraryId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();

  // Load all libraries for the switcher
  const librariesQuery = useLibraries({});
  const libraries = librariesQuery.data?.libraries || [];

  // Load lists for sidebar navigation
  const listsQuery = useListLists();
  const lists = listsQuery.data?.lists || [];
  const currentLibrary = libraries.find((lib) => lib.id === Number(libraryId));

  // Check if we're currently viewing a list
  const listMatch = location.pathname.match(/^\/lists\/(\d+)/);
  const currentListId = listMatch ? Number(listMatch[1]) : null;
  const currentList = lists.find((list) => list.id === currentListId);

  // Determine what to show in the trigger
  const isViewingList = currentListId !== null;

  const handleLibrarySwitch = useCallback(
    (newLibraryId: number) => {
      navigate(`/libraries/${newLibraryId}`);
    },
    [navigate],
  );

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          className="h-9 gap-2 text-muted-foreground hover:text-foreground cursor-pointer"
          variant="ghost"
        >
          {isViewingList ? (
            <>
              <List className="h-4 w-4" />
              {currentList?.name || "List"}
            </>
          ) : (
            <>
              <Library className="h-4 w-4" />
              {currentLibrary?.name || "Select Library"}
            </>
          )}
          <ChevronDown className="h-3 w-3 opacity-60" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-64 p-0 overflow-hidden">
        {/* Libraries Section */}
        <div className="p-1.5">
          <div className="px-2 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
            Libraries
          </div>
          <div className="space-y-0.5">
            {libraries.map((library) => {
              const isActive =
                !isViewingList && library.id === Number(libraryId);
              return (
                <button
                  className={`
                    group relative flex w-full items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors
                    ${
                      isActive
                        ? "bg-primary/10 text-primary dark:bg-primary/15 dark:text-violet-300"
                        : "text-foreground hover:bg-accent"
                    }
                  `}
                  key={library.id}
                  onClick={() => handleLibrarySwitch(library.id)}
                >
                  <div
                    className={`
                      flex h-7 w-7 shrink-0 items-center justify-center rounded-md
                      ${
                        isActive
                          ? "bg-primary text-primary-foreground dark:bg-violet-400 dark:text-neutral-900"
                          : "bg-muted text-muted-foreground group-hover:bg-muted/80"
                      }
                    `}
                  >
                    <Library className="h-3.5 w-3.5" />
                  </div>
                  <span className="flex-1 truncate text-left font-medium">
                    {library.name}
                  </span>
                  {isActive && (
                    <Check className="h-4 w-4 shrink-0 text-primary dark:text-violet-300" />
                  )}
                </button>
              );
            })}
            {libraries.length === 0 && (
              <div className="px-2 py-3 text-sm text-muted-foreground text-center">
                No libraries yet
              </div>
            )}
          </div>
        </div>

        {/* Lists Section */}
        {lists.length > 0 && (
          <div className="border-t border-border bg-muted/30 p-1.5">
            <div className="px-2 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
              Lists
            </div>
            <div className="space-y-0.5">
              {lists.slice(0, 5).map((list) => {
                const isActive = currentListId === list.id;
                return (
                  <Link
                    className={`
                      group relative flex w-full items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors
                      ${
                        isActive
                          ? "bg-primary/10 text-primary dark:bg-primary/15 dark:text-violet-300"
                          : "text-foreground hover:bg-accent/50"
                      }
                    `}
                    key={list.id}
                    to={`/lists/${list.id}`}
                  >
                    <div
                      className={`
                        flex h-7 w-7 shrink-0 items-center justify-center rounded-md
                        ${
                          isActive
                            ? "bg-primary text-primary-foreground dark:bg-violet-400 dark:text-neutral-900"
                            : "bg-background text-muted-foreground group-hover:bg-background/80"
                        }
                      `}
                    >
                      <List className="h-3.5 w-3.5" />
                    </div>
                    <span className="flex-1 truncate text-left font-medium">
                      {list.name}
                    </span>
                    <span
                      className={`
                        shrink-0 tabular-nums text-xs
                        ${isActive ? "text-primary/70 dark:text-violet-300/70" : "text-muted-foreground"}
                      `}
                    >
                      {list.book_count}
                    </span>
                    {isActive && (
                      <Check className="h-4 w-4 shrink-0 text-primary dark:text-violet-300" />
                    )}
                  </Link>
                );
              })}
              {lists.length > 5 && (
                <Link
                  className="flex w-full items-center justify-center rounded-md px-2 py-2 text-xs text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
                  to="/lists"
                >
                  View all {lists.length} lists →
                </Link>
              )}
            </div>
          </div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default LibraryListPicker;
