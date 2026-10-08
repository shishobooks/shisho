import type { UseQueryResult } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter from "@/components/library/PaginationFooter";
import QueryError from "@/components/library/QueryError";
import { SearchInput } from "@/components/library/SearchInput";
import { Badge } from "@/components/ui/badge";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { useResourceListState } from "@/hooks/useResourceListState";
import type { ResourceListResponse } from "@/types";

interface BadgeConfig {
  label: string;
  count: number;
  variant?: "default" | "secondary" | "outline" | "destructive";
}

interface ItemConfig {
  name: string;
  secondaryText?: string;
  aliases: string[];
  badges: BadgeConfig[];
}

interface ResourceListProps<T extends { id: number }> {
  title: string;
  subtitle: string;
  /** Names the search box ("Search genres"). */
  searchLabel: string;
  /** Defaults to the label followed by "...". */
  searchPlaceholder?: string;
  query: UseQueryResult<ResourceListResponse<T>>;
  state: ReturnType<typeof useResourceListState>;
  itemConfig: (item: T) => ItemConfig;
  linkTo: (item: T, libraryId: string) => string;
  itemLabel: string;
  maxWidth?: string;
}

const ResourceList = <T extends { id: number }>({
  title,
  subtitle,
  searchLabel,
  searchPlaceholder,
  query,
  state,
  itemConfig,
  linkTo,
  itemLabel,
  maxWidth = "max-w-4xl",
}: ResourceListProps<T>) => {
  usePageTitle(title);

  const {
    libraryId,
    currentPage,
    searchQuery,
    debouncedSearch,
    handleDebouncedSearchChange,
    limit,
    offset,
    handlePageChange,
  } = state;

  // Track the search value that produced the currently displayed data or
  // error. A failed search settles it too, so the spinner gives way to the
  // error instead of waiting for a success that never comes.
  const [confirmedSearch, setConfirmedSearch] = useState<string | null>(null);
  const settled = query.isSuccess || query.isError;

  useEffect(() => {
    if (settled && !query.isFetching) {
      setConfirmedSearch(debouncedSearch);
    }
  }, [settled, query.isFetching, debouncedSearch]);

  // Data is stale if search changed but query hasn't completed yet
  const isStaleData =
    confirmedSearch !== null && debouncedSearch !== confirmedSearch;

  const total = query.data?.total ?? 0;
  const totalPages = Math.ceil(total / limit);
  // Loaded results for the current search, not a page still loading.
  const showsResults = !query.isFetching && !isStaleData;

  const renderItem = (item: T) => {
    const config = itemConfig(item);

    return (
      <Link
        className="flex items-center justify-between p-3 rounded-md hover:bg-muted/50 transition-colors"
        key={item.id}
        to={linkTo(item, libraryId)}
      >
        <div className="min-w-0 flex-1 mr-3">
          <div className="flex items-baseline">
            <span className="font-semibold text-lg">{config.name}</span>
            {config.secondaryText && (
              <span className="text-sm text-muted-foreground">
                <span className="mx-1.5 text-muted-foreground/50">·</span>
                {config.secondaryText}
              </span>
            )}
          </div>
          {config.aliases.length > 0 && (
            <div
              className="text-sm text-muted-foreground truncate"
              title={config.aliases.join(", ")}
            >
              {config.aliases.join(", ")}
            </div>
          )}
        </div>
        <div className="flex gap-2 shrink-0">
          {config.badges.map((badge) => (
            <Badge key={badge.label} variant={badge.variant ?? "secondary"}>
              {badge.count} {badge.label}
            </Badge>
          ))}
        </div>
      </Link>
    );
  };

  return (
    <LibraryLayout maxWidth={maxWidth}>
      <div className="mb-6">
        <h1 className="text-2xl font-semibold mb-2">{title}</h1>
        <p className="text-muted-foreground">{subtitle}</p>
      </div>

      <div className="mb-6">
        <SearchInput
          initialValue={searchQuery}
          label={searchLabel}
          onDebouncedChange={handleDebouncedSearchChange}
          placeholder={searchPlaceholder ?? `${searchLabel}...`}
        />
      </div>

      {/* One live region that stays mounted while the results load and
          change, so a new count or the empty state is announced. A region
          mounted with its text is often not. */}
      <div role="status">
        {showsResults && query.data && total > 0 && (
          <div className="mb-4 text-sm text-muted-foreground">
            Showing {offset + 1}-{Math.min(offset + limit, total)} of {total}{" "}
            {itemLabel}
          </div>
        )}
        {showsResults && query.data?.items.length === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            {confirmedSearch
              ? `No ${itemLabel} found matching your search.`
              : `No ${itemLabel} in this library yet.`}
          </div>
        )}
      </div>

      {(query.isLoading || query.isFetching || isStaleData) && (
        <LoadingSpinner />
      )}

      {query.error && !query.data && !query.isFetching && !isStaleData && (
        <QueryError fallback={`Failed to load ${itemLabel}`} query={query} />
      )}

      {showsResults && query.data && (
        <>
          {query.data.items.length > 0 && (
            <div className="space-y-1 mb-6">
              {query.data.items.map(renderItem)}
            </div>
          )}

          <PaginationFooter
            currentPage={currentPage}
            onPageChange={handlePageChange}
            totalPages={totalPages}
          />
        </>
      )}
    </LibraryLayout>
  );
};

export default ResourceList;
