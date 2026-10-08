import { Plus } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import { CreateListDialog } from "@/components/library/CreateListDialog";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useCreateList,
  useCreateListFromTemplate,
  useListLists,
  useListTemplates,
} from "@/hooks/queries/lists";
import { usePageTitle } from "@/hooks/usePageTitle";
import { toastRequestError } from "@/libraries/api";
import type { CreateListPayload, ListResponse, ListTemplate } from "@/types";

const ListsIndex = () => {
  usePageTitle("Lists");

  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  const listsQuery = useListLists();
  const templatesQuery = useListTemplates();
  const createListMutation = useCreateList();
  const createFromTemplateMutation = useCreateListFromTemplate();

  const lists = listsQuery.data?.items ?? [];
  const templates = templatesQuery.data ?? [];
  const hasLists = lists.length > 0;

  const handleCreate = async (payload: CreateListPayload) => {
    try {
      await createListMutation.mutateAsync(payload);
      toast.success(`Created "${payload.name}" list`);
    } catch (error) {
      toastRequestError(error, "Failed to create list");
      throw error; // Let CreateListDialog preserve the draft on failure.
    }
  };

  const handleCreateFromTemplate = async (template: ListTemplate) => {
    try {
      await createFromTemplateMutation.mutateAsync({
        templateName: template.name,
      });
      toast.success(`Created "${template.display_name}" list`);
    } catch (error) {
      toastRequestError(error, "Failed to create list");
    }
  };

  const renderListCard = (list: ListResponse) => {
    const bookCount = list.book_count ?? 0;

    return (
      <Link
        className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 sm:gap-4 p-3 md:p-4 rounded-md border bg-card hover:bg-muted/50 transition-colors"
        key={list.id}
        to={`/lists/${list.id}`}
      >
        <div className="flex flex-col gap-1 min-w-0">
          <span className="font-medium text-sm md:text-base">{list.name}</span>
          {list.description && (
            <span className="text-xs md:text-sm text-muted-foreground line-clamp-1">
              {list.description}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {list.permission !== "owner" && (
            <Badge variant="outline">{list.permission}</Badge>
          )}
          <Badge variant="secondary">
            {bookCount} book{bookCount !== 1 ? "s" : ""}
          </Badge>
        </div>
      </Link>
    );
  };

  const renderTemplateCard = (template: ListTemplate) => {
    return (
      <Button
        className="flex flex-col items-start gap-2 p-4 rounded-md border bg-card hover:bg-muted/50 transition-colors text-left"
        disabled={createFromTemplateMutation.isPending}
        key={template.name}
        onClick={() => handleCreateFromTemplate(template)}
        variant="unstyled"
      >
        <span className="font-medium">{template.display_name}</span>
        <span className="text-sm text-muted-foreground">
          {template.description}
        </span>
      </Button>
    );
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-3xl w-full mx-auto px-4 md:px-6 py-4 md:py-8">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 md:mb-8">
          <div>
            <h1 className="text-2xl font-semibold mb-1 md:mb-2">Lists</h1>
            <p className="text-sm md:text-base text-muted-foreground">
              Organize your books into custom collections
            </p>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Button onClick={() => setCreateDialogOpen(true)} size="sm">
              <Plus className="h-4 w-4 sm:mr-2" />
              <span className="hidden sm:inline">Create List</span>
            </Button>
          </div>
        </div>

        {listsQuery.isLoading && <LoadingSpinner />}

        {listsQuery.error && !listsQuery.data && (
          <QueryError fallback="Failed to load lists" query={listsQuery} />
        )}

        {listsQuery.data && !hasLists && (
          <div className="space-y-6">
            <div className="text-center py-8">
              <p className="text-muted-foreground mb-4">
                You don't have any lists yet. Get started with a template or
                create a custom list.
              </p>
            </div>

            {templatesQuery.isSuccess && templates.length > 0 && (
              <div>
                <h2 className="text-lg font-medium mb-4">Quick Start</h2>
                <div className="grid gap-3 sm:grid-cols-2">
                  {templates.map(renderTemplateCard)}
                </div>
              </div>
            )}
          </div>
        )}

        {listsQuery.data && hasLists && (
          <div className="space-y-2">{lists.map(renderListCard)}</div>
        )}
      </div>

      <CreateListDialog
        isPending={createListMutation.isPending}
        onCreate={handleCreate}
        onOpenChange={setCreateDialogOpen}
        open={createDialogOpen}
      />
    </div>
  );
};

export default ListsIndex;
