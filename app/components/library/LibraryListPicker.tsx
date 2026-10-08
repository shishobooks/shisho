import { Check, ChevronDown, Library, List } from "lucide-react";
import { Link, useLocation, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useNavLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";
import { useCan } from "@/hooks/useCan";
import { cn } from "@/libraries/utils";

const LibraryListPicker = () => {
  const { libraryId } = useParams();
  const location = useLocation();
  // Library pages need Books Read, so a role without it has no library to
  // switch to. Lists stay reachable from the user menu.
  const canReadBooks = useCan("books:read");

  // Load the role's accessible libraries for the switcher
  const libraries = useNavLibraries().data ?? [];

  // Load lists for sidebar navigation
  const listsQuery = useListLists();
  const lists = listsQuery.data?.items || [];
  const currentLibrary = libraries.find((lib) => lib.id === Number(libraryId));

  // Check if we're currently viewing a list
  const listMatch = location.pathname.match(/^\/lists\/(\d+)/);
  const currentListId = listMatch ? Number(listMatch[1]) : null;
  const currentList = lists.find((list) => list.id === currentListId);

  // Determine what to show in the trigger
  const isViewingList = currentListId !== null;

  if (!canReadBooks) {
    return null;
  }

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
        {/* Entries are DropdownMenuItems so the menu's arrow keys reach them.
            Radix focuses an item on hover, so hover styles go on focus:
            variants, and the current row darkens its tint on focus so focus
            stays visible there. DropdownMenuItem also resizes and recolors
            any svg without a size- or text- class, so the row icons carry
            both. */}
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
                <DropdownMenuItem
                  asChild
                  className={cn(
                    "group relative flex w-full items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors",
                    isActive
                      ? "bg-primary/10 text-primary focus:bg-primary/15 focus:text-primary"
                      : "text-foreground focus:bg-accent focus:text-foreground",
                  )}
                  key={library.id}
                >
                  <Link
                    aria-current={isActive ? "true" : undefined}
                    to={`/libraries/${library.id}`}
                  >
                    <div
                      className={cn(
                        "flex h-7 w-7 shrink-0 items-center justify-center rounded-md",
                        isActive
                          ? "bg-primary text-primary-foreground"
                          : "bg-muted text-muted-foreground group-hover:bg-muted/80",
                      )}
                    >
                      <Library className="size-3.5 text-current" />
                    </div>
                    <span className="flex-1 truncate text-left font-medium">
                      {library.name}
                    </span>
                    {isActive && (
                      <Check className="size-4 shrink-0 text-primary" />
                    )}
                  </Link>
                </DropdownMenuItem>
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
                  <DropdownMenuItem
                    asChild
                    className={cn(
                      "group relative flex w-full items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors",
                      isActive
                        ? "bg-primary/10 text-primary focus:bg-primary/15 focus:text-primary"
                        : "text-foreground focus:bg-accent/50 focus:text-foreground",
                    )}
                    key={list.id}
                  >
                    <Link
                      aria-current={isActive ? "page" : undefined}
                      to={`/lists/${list.id}`}
                    >
                      <div
                        className={cn(
                          "flex h-7 w-7 shrink-0 items-center justify-center rounded-md",
                          isActive
                            ? "bg-primary text-primary-foreground"
                            : "bg-background text-muted-foreground group-hover:bg-background/80",
                        )}
                      >
                        <List className="size-3.5 text-current" />
                      </div>
                      <span className="flex-1 truncate text-left font-medium">
                        {list.name}
                      </span>
                      <span
                        className={cn(
                          "shrink-0 tabular-nums text-xs",
                          isActive
                            ? "text-primary/70"
                            : "text-muted-foreground",
                        )}
                      >
                        {list.book_count}
                      </span>
                      {isActive && (
                        <Check className="size-4 shrink-0 text-primary" />
                      )}
                    </Link>
                  </DropdownMenuItem>
                );
              })}
              {lists.length > 5 && (
                <DropdownMenuItem
                  asChild
                  className="flex w-full items-center justify-center rounded-md px-2 py-2 text-xs text-muted-foreground transition-colors focus:bg-accent/50 focus:text-foreground"
                >
                  <Link to="/lists">View all {lists.length} lists →</Link>
                </DropdownMenuItem>
              )}
            </div>
          </div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default LibraryListPicker;
