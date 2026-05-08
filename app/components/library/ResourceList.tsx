import type { UseQueryResult } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter from "@/components/library/PaginationFooter";
import { SearchInput } from "@/components/library/SearchInput";
import { Badge } from "@/components/ui/badge";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { useResourceListState } from "@/hooks/useResourceListState";
import type { ResourceListResponse } from "@/types";

const ITEMS_PER_PAGE = 50;

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

interface ResourceListProps<T> {
  title: string;
  subtitle: string;
  searchPlaceholder: string;
  query: UseQueryResult<ResourceListResponse<T>>;
  state: ReturnType<typeof useResourceListState>;
  itemConfig: (item: T) => ItemConfig;
  linkTo: (item: T, libraryId: string) => string;
  itemLabel: string;
  maxWidth?: string;
}

const ResourceList = <T,>({
  title,
  subtitle,
  searchPlaceholder,
  query,
  state,
  itemConfig,
  linkTo,
  itemLabel,
  maxWidth = "max-w-3xl",
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

  // Track the search value that produced the currently displayed data
  const [confirmedSearch, setConfirmedSearch] = useState<string | null>(null);

  useEffect(() => {
    if (query.isSuccess && !query.isFetching) {
      setConfirmedSearch(debouncedSearch);
    }
  }, [query.isSuccess, query.isFetching, debouncedSearch]);

  // Data is stale if search changed but query hasn't completed yet
  const isStaleData =
    confirmedSearch !== null && debouncedSearch !== confirmedSearch;

  const total = query.data?.total ?? 0;
  const totalPages = Math.ceil(total / ITEMS_PER_PAGE);

  const renderItem = (item: T, index: number) => {
    const config = itemConfig(item);

    return (
      <Link
        className="flex items-center justify-between p-3 rounded-md hover:bg-muted/50 transition-colors"
        key={index}
        to={linkTo(item, libraryId)}
      >
        <div className="min-w-0 flex-1 mr-3">
          <div className="flex items-baseline gap-0">
            <span className="font-semibold text-lg">{config.name}</span>
            {config.secondaryText && (
              <span className="text-sm text-muted-foreground ml-0">
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
          onDebouncedChange={handleDebouncedSearchChange}
          placeholder={searchPlaceholder}
        />
      </div>

      {(query.isLoading || query.isFetching || isStaleData) && (
        <LoadingSpinner />
      )}

      {query.isSuccess && !query.isFetching && !isStaleData && (
        <>
          {total > 0 && (
            <div className="mb-4 text-sm text-muted-foreground">
              Showing {offset + 1}-{Math.min(offset + limit, total)} of {total}{" "}
              {itemLabel}
            </div>
          )}

          {query.data.items.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              {confirmedSearch
                ? `No ${itemLabel} found matching your search.`
                : `No ${itemLabel} in this library yet.`}
            </div>
          ) : (
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
