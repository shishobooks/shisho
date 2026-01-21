import { Plus } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import { CreateListDialog } from "@/components/library/CreateListDialog";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useCreateList,
  useCreateListFromTemplate,
  useListLists,
  useListTemplates,
  type ListTemplate,
  type ListWithCount,
} from "@/hooks/queries/lists";
import type { CreateListPayload } from "@/types";

const ListsIndex = () => {
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  const listsQuery = useListLists();
  const templatesQuery = useListTemplates();
  const createListMutation = useCreateList();
  const createFromTemplateMutation = useCreateListFromTemplate();

  const lists = listsQuery.data?.lists ?? [];
  const templates = templatesQuery.data ?? [];
  const hasLists = lists.length > 0;

  const handleCreate = async (payload: CreateListPayload) => {
    try {
      await createListMutation.mutateAsync(payload);
      toast.success(`Created "${payload.name}" list`);
      setCreateDialogOpen(false);
    } catch (error) {
      let message = "Failed to create list";
      if (error instanceof Error) {
        message = error.message;
      }
      toast.error(message);
    }
  };

  const handleCreateFromTemplate = async (template: ListTemplate) => {
    try {
      await createFromTemplateMutation.mutateAsync({
        templateName: template.name,
      });
      toast.success(`Created "${template.display_name}" list`);
    } catch (error) {
      let message = "Failed to create list";
      if (error instanceof Error) {
        message = error.message;
      }
      toast.error(message);
    }
  };

  const renderListCard = (list: ListWithCount) => {
    const bookCount = list.book_count ?? 0;

    return (
      <Link
        className="flex items-center justify-between p-4 rounded-lg border bg-card hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
        key={list.id}
        to={`/lists/${list.id}`}
      >
        <div className="flex flex-col gap-1">
          <span className="font-medium">{list.name}</span>
          {list.description && (
            <span className="text-sm text-muted-foreground line-clamp-1">
              {list.description}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
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
      <button
        className="flex flex-col items-start gap-2 p-4 rounded-lg border bg-card hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors text-left cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
        disabled={createFromTemplateMutation.isPending}
        key={template.name}
        onClick={() => handleCreateFromTemplate(template)}
        type="button"
      >
        <span className="font-medium">{template.display_name}</span>
        <span className="text-sm text-muted-foreground">
          {template.description}
        </span>
      </button>
    );
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-3xl w-full mx-auto px-6 py-8">
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl font-semibold mb-2">Lists</h1>
            <p className="text-muted-foreground">
              Organize your books into custom collections
            </p>
          </div>
          <Button onClick={() => setCreateDialogOpen(true)}>
            <Plus className="h-4 w-4" />
            Create List
          </Button>
        </div>

        {listsQuery.isLoading && (
          <div className="text-muted-foreground">Loading...</div>
        )}

        {listsQuery.isSuccess && !hasLists && (
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

        {listsQuery.isSuccess && hasLists && (
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
