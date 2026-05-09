import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState, type ReactNode } from "react";

import LibraryBreadcrumbs from "@/components/library/LibraryBreadcrumbs";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import {
  MetadataEditDialog,
  type EntityType,
} from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useLibrary } from "@/hooks/queries/libraries";
import type { DataSource } from "@/types";

interface BreadcrumbItem {
  label: string;
  to?: string;
}

interface EditConfig {
  isPending: boolean;
  onSave: (data: {
    name: string;
    sort_name?: string;
    aliases?: string[];
  }) => Promise<void>;
  /** Show sort name field in edit dialog (for person/series) */
  sortName?: string;
  sortNameSource?: DataSource;
}

interface MergeConfig {
  entities: { id: number; name: string; count: number }[];
  isLoadingEntities: boolean;
  isPending: boolean;
  onMerge: (sourceId: number) => Promise<void>;
  onSearch: (search: string) => void;
}

interface DeleteConfig {
  isPending: boolean;
  onDelete: () => Promise<void>;
  /** When true, the Delete button is hidden (e.g. entity still has books) */
  disabled: boolean;
}

interface CountLabel {
  singular: string;
  plural: string;
}

interface ResourceDetailProps {
  libraryId: string;
  entityId: number;
  entityType: EntityType;
  name: string;
  /** Optional sort name displayed below the heading */
  sortName?: string;
  aliases: string[];
  bookCount: number;
  /** Label for the count badge. Defaults to { singular: "book", plural: "books" } */
  countLabel?: CountLabel;
  /** Additional badges rendered after the primary count badge */
  extraBadges?: ReactNode;
  breadcrumbItems: BreadcrumbItem[];
  editConfig: EditConfig;
  mergeConfig: MergeConfig;
  deleteConfig: DeleteConfig;
  /** Whether the main entity query is still loading */
  isLoading?: boolean;
  /** Whether the main entity query failed or returned no data */
  notFound?: boolean;
  /** Label for the not-found page heading (e.g. "Genre Not Found") */
  notFoundLabel?: string;
  children?: ReactNode;
}

const DEFAULT_COUNT_LABEL: CountLabel = { singular: "book", plural: "books" };

export function ResourceDetail({
  libraryId,
  entityId,
  entityType,
  name,
  sortName,
  aliases,
  bookCount,
  countLabel = DEFAULT_COUNT_LABEL,
  extraBadges,
  breadcrumbItems,
  editConfig,
  mergeConfig,
  deleteConfig,
  isLoading,
  notFound,
  notFoundLabel,
  children,
}: ResourceDetailProps) {
  const libraryQuery = useLibrary(libraryId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  if (isLoading) {
    return (
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (notFound) {
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">
            {notFoundLabel ?? "Not Found"}
          </h1>
          <p className="text-muted-foreground">
            The {entityType} you're looking for doesn't exist or may have been
            removed.
          </p>
        </div>
      </LibraryLayout>
    );
  }

  return (
    <LibraryLayout>
      <LibraryBreadcrumbs
        items={breadcrumbItems}
        libraryId={libraryId}
        libraryName={libraryQuery.data?.name}
      />

      {/* Header */}
      <div className="mb-6 md:mb-8">
        <div className="flex items-start justify-between gap-4 mb-2">
          <div className="min-w-0">
            <div className="flex items-baseline gap-2 flex-wrap">
              <h1 className="text-2xl font-semibold break-words">{name}</h1>
              {sortName && sortName !== name && (
                <span className="text-sm text-muted-foreground">
                  <span className="text-muted-foreground/50">·</span> {sortName}
                </span>
              )}
            </div>
          </div>
          <div className="flex gap-2 shrink-0">
            <Button
              onClick={() => setEditOpen(true)}
              size="sm"
              variant="outline"
            >
              <Edit className="h-4 w-4 mr-2" />
              Edit
            </Button>
            <Button
              onClick={() => setMergeOpen(true)}
              size="sm"
              variant="outline"
            >
              <GitMerge className="h-4 w-4 mr-2" />
              Merge
            </Button>
            {!deleteConfig.disabled && (
              <Button
                onClick={() => setDeleteOpen(true)}
                size="sm"
                variant="outline"
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Delete
              </Button>
            )}
          </div>
        </div>
        {aliases.length > 0 && (
          <p className="text-sm text-muted-foreground mb-2">
            {aliases.join(", ")}
          </p>
        )}
        <div className="flex items-center gap-2 flex-wrap">
          <Badge variant="secondary">
            {bookCount}{" "}
            {bookCount !== 1 ? countLabel.plural : countLabel.singular}
          </Badge>
          {extraBadges}
        </div>
      </div>

      {children}

      <MetadataEditDialog
        aliases={aliases}
        entityName={name}
        entityType={entityType}
        isPending={editConfig.isPending}
        onOpenChange={setEditOpen}
        onSave={editConfig.onSave}
        open={editOpen}
        sortName={editConfig.sortName}
        sortNameSource={editConfig.sortNameSource}
      />

      <MetadataMergeDialog
        entities={mergeConfig.entities}
        entityType={entityType}
        isLoadingEntities={mergeConfig.isLoadingEntities}
        isPending={mergeConfig.isPending}
        onMerge={async (sourceId) => {
          await mergeConfig.onMerge(sourceId);
          setMergeOpen(false);
        }}
        onOpenChange={setMergeOpen}
        onSearch={mergeConfig.onSearch}
        open={mergeOpen}
        targetId={entityId}
        targetName={name}
      />

      <MetadataDeleteDialog
        entityName={name}
        entityType={entityType}
        isPending={deleteConfig.isPending}
        onDelete={async () => {
          await deleteConfig.onDelete();
          setDeleteOpen(false);
        }}
        onOpenChange={setDeleteOpen}
        open={deleteOpen}
      />
    </LibraryLayout>
  );
}
