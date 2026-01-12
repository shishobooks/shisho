# Metadata Entity Editing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable direct editing, merging, and deletion of metadata entities (Person, Series, Genre, Tag) from their detail pages.

**Architecture:** Backend API endpoints already exist for PATCH, DELETE, and POST /merge on all entity types. The implementation focuses on creating frontend mutation hooks for Person/Series, building reusable edit/merge/delete dialogs, creating GenreDetail and TagDetail pages, and updating PersonDetail/SeriesDetail pages with action buttons.

**Tech Stack:** React 19, TypeScript, TanStack Query, Radix UI Dialog, TailwindCSS, Go/Echo backend (existing)

---

## Task 1: Add Person Mutation Hooks

**Files:**
- Modify: `app/hooks/queries/people.ts`
- Reference: `app/hooks/queries/genres.ts:76-109` (pattern to follow)

**Step 1: Read current people.ts file**

Verify the current state of the file to understand imports and structure.

**Step 2: Add imports for useMutation and useQueryClient**

```typescript
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
```

**Step 3: Add UpdatePersonPayload and MergePersonPayload types**

After the existing interfaces, add:

```typescript
export interface UpdatePersonPayload {
  name?: string;
  sort_name?: string;
}

export interface MergePersonPayload {
  source_id: number;
}
```

**Step 4: Add useUpdatePerson hook**

At the end of the file, add:

```typescript
export const useUpdatePerson = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      personId,
      payload,
    }: {
      personId: number;
      payload: UpdatePersonPayload;
    }) => {
      return API.request<PersonWithCounts>("PATCH", `/people/${personId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePerson, variables.personId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPeople] });
    },
  });
};
```

**Step 5: Add useMergePerson hook**

```typescript
export const useMergePerson = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/people/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePerson, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPeople] });
    },
  });
};
```

**Step 6: Add useDeletePerson hook**

```typescript
export const useDeletePerson = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ personId }: { personId: number }) => {
      return API.request<void>("DELETE", `/people/${personId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPeople] });
    },
  });
};
```

**Step 7: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 8: Commit**

```bash
git add app/hooks/queries/people.ts
git commit -m "$(cat <<'EOF'
[Frontend] Add Person mutation hooks for update, merge, and delete
EOF
)"
```

---

## Task 2: Add Series Mutation Hooks

**Files:**
- Modify: `app/hooks/queries/series.ts`
- Reference: `app/hooks/queries/genres.ts:76-109` (pattern to follow)

**Step 1: Add imports for useMutation and useQueryClient**

Update the import statement:

```typescript
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
```

**Step 2: Add UpdateSeriesPayload and MergeSeriesPayload types**

After the existing interfaces, add:

```typescript
export interface UpdateSeriesPayload {
  name?: string;
  sort_name?: string;
  description?: string;
}

export interface MergeSeriesPayload {
  source_id: number;
}
```

**Step 3: Add SeriesWithCount interface**

```typescript
export interface SeriesWithCount extends Series {
  book_count: number;
}
```

**Step 4: Update useSeries return type**

Update the useSeries hook to return SeriesWithCount:

```typescript
export const useSeries = (
  seriesId?: number,
  options: Omit<
    UseQueryOptions<SeriesWithCount, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<SeriesWithCount, ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(seriesId),
    ...options,
    queryKey: [QueryKey.RetrieveSeries, seriesId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/series/${seriesId}`, null, null, signal);
    },
  });
};
```

**Step 5: Add useUpdateSeries hook**

At the end of the file, add:

```typescript
export const useUpdateSeries = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      seriesId,
      payload,
    }: {
      seriesId: number;
      payload: UpdateSeriesPayload;
    }) => {
      return API.request<SeriesWithCount>("PATCH", `/series/${seriesId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveSeries, variables.seriesId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListSeries] });
    },
  });
};
```

**Step 6: Add useMergeSeries hook**

```typescript
export const useMergeSeries = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/series/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveSeries, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListSeries] });
    },
  });
};
```

**Step 7: Add useDeleteSeries hook**

```typescript
export const useDeleteSeries = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ seriesId }: { seriesId: number }) => {
      return API.request<void>("DELETE", `/series/${seriesId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListSeries] });
    },
  });
};
```

**Step 8: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 9: Commit**

```bash
git add app/hooks/queries/series.ts
git commit -m "$(cat <<'EOF'
[Frontend] Add Series mutation hooks for update, merge, and delete
EOF
)"
```

---

## Task 3: Add Merge Hooks for Genre and Tag

**Files:**
- Modify: `app/hooks/queries/genres.ts`
- Modify: `app/hooks/queries/tags.ts`

**Step 1: Add useMergeGenre hook to genres.ts**

Add at the end of the file:

```typescript
export const useMergeGenre = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/genres/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveGenre, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListGenres] });
    },
  });
};
```

**Step 2: Add useMergeTag hook to tags.ts**

Add at the end of the file:

```typescript
export const useMergeTag = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/tags/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveTag, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListTags] });
    },
  });
};
```

**Step 3: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 4: Commit**

```bash
git add app/hooks/queries/genres.ts app/hooks/queries/tags.ts
git commit -m "$(cat <<'EOF'
[Frontend] Add merge hooks for Genre and Tag
EOF
)"
```

---

## Task 4: Create MetadataEditDialog Component

**Files:**
- Create: `app/components/library/MetadataEditDialog.tsx`
- Reference: `app/components/library/BookEditDialog.tsx` (dialog pattern)

**Step 1: Create the MetadataEditDialog component**

```typescript
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export type EntityType = "person" | "series" | "genre" | "tag";

interface MetadataEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  sortName?: string;
  onSave: (data: { name: string; sort_name?: string }) => Promise<void>;
  isPending: boolean;
}

const ENTITY_LABELS: Record<EntityType, string> = {
  person: "Person",
  series: "Series",
  genre: "Genre",
  tag: "Tag",
};

export function MetadataEditDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  sortName,
  onSave,
  isPending,
}: MetadataEditDialogProps) {
  const [name, setName] = useState(entityName);
  const [editSortName, setEditSortName] = useState(sortName || "");

  const hasSortName = entityType === "person" || entityType === "series";

  useEffect(() => {
    if (open) {
      setName(entityName);
      setEditSortName(sortName || "");
    }
  }, [open, entityName, sortName]);

  const handleSubmit = async () => {
    const data: { name: string; sort_name?: string } = { name };
    if (hasSortName) {
      data.sort_name = editSortName || undefined;
    }
    await onSave(data);
  };

  const hasChanges =
    name !== entityName || (hasSortName && editSortName !== (sortName || ""));

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Edit {ENTITY_LABELS[entityType]}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              onChange={(e) => setName(e.target.value)}
              value={name}
            />
          </div>

          {hasSortName && (
            <div className="space-y-2">
              <Label htmlFor="sort_name">Sort Name</Label>
              <Input
                id="sort_name"
                onChange={(e) => setEditSortName(e.target.value)}
                placeholder="Leave empty to auto-generate"
                value={editSortName}
              />
              <p className="text-xs text-muted-foreground">
                Used for sorting. Clear to regenerate automatically.
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={isPending || !hasChanges || !name.trim()}
            onClick={handleSubmit}
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 3: Commit**

```bash
git add app/components/library/MetadataEditDialog.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add MetadataEditDialog component for entity editing
EOF
)"
```

---

## Task 5: Create MetadataMergeDialog Component

**Files:**
- Create: `app/components/library/MetadataMergeDialog.tsx`

**Step 1: Create the MetadataMergeDialog component**

```typescript
import { Check, ChevronsUpDown, Loader2 } from "lucide-react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useDebounce } from "@/hooks/useDebounce";

import type { EntityType } from "./MetadataEditDialog";

interface EntityOption {
  id: number;
  name: string;
  count: number;
}

interface MetadataMergeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  targetName: string;
  targetId: number;
  onMerge: (sourceId: number) => Promise<void>;
  isPending: boolean;
  entities: EntityOption[];
  isLoadingEntities: boolean;
  onSearch: (search: string) => void;
}

const ENTITY_LABELS: Record<EntityType, string> = {
  person: "Person",
  series: "Series",
  genre: "Genre",
  tag: "Tag",
};

const ENTITY_PLURALS: Record<EntityType, string> = {
  person: "people",
  series: "series",
  genre: "genres",
  tag: "tags",
};

export function MetadataMergeDialog({
  open,
  onOpenChange,
  entityType,
  targetName,
  targetId,
  onMerge,
  isPending,
  entities,
  isLoadingEntities,
  onSearch,
}: MetadataMergeDialogProps) {
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [comboboxOpen, setComboboxOpen] = useState(false);
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebounce(search, 200);

  // Filter out the target entity from the list
  const availableEntities = useMemo(() => {
    return entities.filter((e) => e.id !== targetId);
  }, [entities, targetId]);

  const selectedEntity = availableEntities.find((e) => e.id === selectedId);

  const handleSearchChange = (value: string) => {
    setSearch(value);
    onSearch(value);
  };

  const handleMerge = async () => {
    if (selectedId) {
      await onMerge(selectedId);
      setSelectedId(null);
      setSearch("");
    }
  };

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setSelectedId(null);
      setSearch("");
    }
    onOpenChange(isOpen);
  };

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Merge into "{targetName}"</DialogTitle>
          <DialogDescription>
            Select a {entityType} to merge into this one. All associated books
            will be transferred and the selected {entityType} will be deleted.
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          <Popover modal onOpenChange={setComboboxOpen} open={comboboxOpen}>
            <PopoverTrigger asChild>
              <Button
                aria-expanded={comboboxOpen}
                className="w-full justify-between"
                role="combobox"
                variant="outline"
              >
                {selectedEntity
                  ? `${selectedEntity.name} (${selectedEntity.count} books)`
                  : `Select ${entityType} to merge...`}
                <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-full p-0">
              <Command shouldFilter={false}>
                <CommandInput
                  onValueChange={handleSearchChange}
                  placeholder={`Search ${ENTITY_PLURALS[entityType]}...`}
                  value={search}
                />
                <CommandList>
                  {isLoadingEntities && (
                    <div className="p-4 text-center text-sm text-muted-foreground">
                      Loading...
                    </div>
                  )}
                  {!isLoadingEntities && availableEntities.length === 0 && (
                    <div className="p-4 text-center text-sm text-muted-foreground">
                      No {ENTITY_PLURALS[entityType]} found
                    </div>
                  )}
                  {!isLoadingEntities && availableEntities.length > 0 && (
                    <CommandGroup>
                      {availableEntities.map((entity) => (
                        <CommandItem
                          key={entity.id}
                          onSelect={() => {
                            setSelectedId(entity.id);
                            setComboboxOpen(false);
                          }}
                          value={String(entity.id)}
                        >
                          <Check
                            className={`mr-2 h-4 w-4 ${
                              selectedId === entity.id
                                ? "opacity-100"
                                : "opacity-0"
                            }`}
                          />
                          <span className="flex-1">{entity.name}</span>
                          <span className="text-muted-foreground text-sm">
                            {entity.count} book{entity.count !== 1 ? "s" : ""}
                          </span>
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  )}
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>

          {selectedEntity && (
            <p className="mt-4 text-sm text-muted-foreground">
              This will move all {selectedEntity.count} book
              {selectedEntity.count !== 1 ? "s" : ""} from "
              {selectedEntity.name}" to "{targetName}" and delete "
              {selectedEntity.name}".
            </p>
          )}
        </div>

        <DialogFooter>
          <Button onClick={() => handleOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={isPending || !selectedId}
            onClick={handleMerge}
            variant="destructive"
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Merge
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 3: Commit**

```bash
git add app/components/library/MetadataMergeDialog.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add MetadataMergeDialog component for entity merging
EOF
)"
```

---

## Task 6: Create MetadataDeleteDialog Component

**Files:**
- Create: `app/components/library/MetadataDeleteDialog.tsx`

**Step 1: Create the MetadataDeleteDialog component**

```typescript
import { AlertTriangle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import type { EntityType } from "./MetadataEditDialog";

interface MetadataDeleteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  onDelete: () => Promise<void>;
  isPending: boolean;
}

const ENTITY_LABELS: Record<EntityType, string> = {
  person: "Person",
  series: "Series",
  genre: "Genre",
  tag: "Tag",
};

export function MetadataDeleteDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  onDelete,
  isPending,
}: MetadataDeleteDialogProps) {
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive" />
            Delete {ENTITY_LABELS[entityType]}
          </DialogTitle>
          <DialogDescription>
            Are you sure you want to delete "{entityName}"? This action cannot
            be undone.
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={isPending}
            onClick={onDelete}
            variant="destructive"
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 3: Commit**

```bash
git add app/components/library/MetadataDeleteDialog.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add MetadataDeleteDialog component for entity deletion
EOF
)"
```

---

## Task 7: Update PersonDetail with Edit/Merge/Delete Actions

**Files:**
- Modify: `app/components/pages/PersonDetail.tsx`
- Reference: Existing PersonDetail.tsx structure

**Step 1: Add imports for dialogs, hooks, and icons**

Add to imports:

```typescript
import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import { Button } from "@/components/ui/button";
import {
  useDeletePerson,
  useMergePerson,
  usePeopleList,
  useUpdatePerson,
} from "@/hooks/queries/people";
import { useDebounce } from "@/hooks/useDebounce";
```

**Step 2: Add state and hooks inside the component**

After the existing queries, add:

```typescript
const navigate = useNavigate();

const [editOpen, setEditOpen] = useState(false);
const [mergeOpen, setMergeOpen] = useState(false);
const [deleteOpen, setDeleteOpen] = useState(false);
const [mergeSearch, setMergeSearch] = useState("");
const debouncedMergeSearch = useDebounce(mergeSearch, 200);

const updatePersonMutation = useUpdatePerson();
const mergePersonMutation = useMergePerson();
const deletePersonMutation = useDeletePerson();

const peopleListQuery = usePeopleList(
  {
    library_id: personQuery.data?.library_id,
    limit: 50,
    search: debouncedMergeSearch || undefined,
  },
  { enabled: mergeOpen && !!personQuery.data?.library_id },
);
```

**Step 3: Add handler functions**

After the hooks, add:

```typescript
const handleEdit = async (data: { name: string; sort_name?: string }) => {
  if (!personId) return;
  await updatePersonMutation.mutateAsync({
    personId,
    payload: {
      name: data.name,
      sort_name: data.sort_name,
    },
  });
  setEditOpen(false);
};

const handleMerge = async (sourceId: number) => {
  if (!personId) return;
  await mergePersonMutation.mutateAsync({
    targetId: personId,
    sourceId,
  });
  setMergeOpen(false);
};

const handleDelete = async () => {
  if (!personId) return;
  await deletePersonMutation.mutateAsync({ personId });
  setDeleteOpen(false);
  navigate(`/libraries/${libraryId}/people`);
};

const canDelete =
  person.authored_book_count === 0 && person.narrated_file_count === 0;
```

**Step 4: Update the header section to include action buttons**

Replace the header section (inside the mb-8 div) with:

```tsx
<div className="mb-8">
  <div className="flex items-center justify-between mb-2">
    <h1 className="text-3xl font-bold">{person.name}</h1>
    <div className="flex gap-2">
      <Button onClick={() => setEditOpen(true)} size="sm" variant="outline">
        <Edit className="h-4 w-4 mr-2" />
        Edit
      </Button>
      <Button onClick={() => setMergeOpen(true)} size="sm" variant="outline">
        <GitMerge className="h-4 w-4 mr-2" />
        Merge
      </Button>
      {canDelete && (
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
  {person.sort_name !== person.name && (
    <p className="text-muted-foreground mb-2">
      Sort name: {person.sort_name}
    </p>
  )}
  <div className="flex gap-2">
    {person.authored_book_count > 0 && (
      <Badge variant="secondary">
        {person.authored_book_count} book
        {person.authored_book_count !== 1 ? "s" : ""} authored
      </Badge>
    )}
    {person.narrated_file_count > 0 && (
      <Badge variant="outline">
        {person.narrated_file_count} file
        {person.narrated_file_count !== 1 ? "s" : ""} narrated
      </Badge>
    )}
  </div>
</div>
```

**Step 5: Add dialog components before the closing div**

At the end of the component, before the final closing `</div>`, add:

```tsx
<MetadataEditDialog
  entityName={person.name}
  entityType="person"
  isPending={updatePersonMutation.isPending}
  onOpenChange={setEditOpen}
  onSave={handleEdit}
  open={editOpen}
  sortName={person.sort_name}
/>

<MetadataMergeDialog
  entities={
    peopleListQuery.data?.people.map((p) => ({
      id: p.id,
      name: p.name,
      count: p.authored_book_count + p.narrated_file_count,
    })) ?? []
  }
  entityType="person"
  isLoadingEntities={peopleListQuery.isLoading}
  isPending={mergePersonMutation.isPending}
  onMerge={handleMerge}
  onOpenChange={setMergeOpen}
  onSearch={setMergeSearch}
  open={mergeOpen}
  targetId={personId!}
  targetName={person.name}
/>

<MetadataDeleteDialog
  entityName={person.name}
  entityType="person"
  isPending={deletePersonMutation.isPending}
  onDelete={handleDelete}
  onOpenChange={setDeleteOpen}
  open={deleteOpen}
/>
```

**Step 6: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 7: Run linting**

Run: `yarn lint:eslint`
Expected: PASS with no errors

**Step 8: Commit**

```bash
git add app/components/pages/PersonDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add edit, merge, and delete actions to PersonDetail page
EOF
)"
```

---

## Task 8: Create Full SeriesDetail Page

**Files:**
- Modify: `app/components/pages/SeriesDetail.tsx`
- Reference: `app/components/pages/PersonDetail.tsx` (structure pattern)

**Step 1: Rewrite SeriesDetail as a full detail page**

Replace the entire file with:

```typescript
import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useDeleteSeries,
  useMergeSeries,
  useSeries,
  useSeriesBooks,
  useSeriesList,
  useUpdateSeries,
} from "@/hooks/queries/series";
import { useDebounce } from "@/hooks/useDebounce";

const SeriesDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const seriesId = id ? parseInt(id, 10) : undefined;

  const seriesQuery = useSeries(seriesId);
  const seriesBooksQuery = useSeriesBooks(seriesId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mergeSearch, setMergeSearch] = useState("");
  const debouncedMergeSearch = useDebounce(mergeSearch, 200);

  const updateSeriesMutation = useUpdateSeries();
  const mergeSeriesMutation = useMergeSeries();
  const deleteSeriesMutation = useDeleteSeries();

  const seriesListQuery = useSeriesList(
    {
      library_id: seriesQuery.data?.library_id,
      limit: 50,
      search: debouncedMergeSearch || undefined,
    },
    { enabled: mergeOpen && !!seriesQuery.data?.library_id },
  );

  const handleEdit = async (data: { name: string; sort_name?: string }) => {
    if (!seriesId) return;
    await updateSeriesMutation.mutateAsync({
      seriesId,
      payload: {
        name: data.name,
        sort_name: data.sort_name,
      },
    });
    setEditOpen(false);
  };

  const handleMerge = async (sourceId: number) => {
    if (!seriesId) return;
    await mergeSeriesMutation.mutateAsync({
      targetId: seriesId,
      sourceId,
    });
    setMergeOpen(false);
  };

  const handleDelete = async () => {
    if (!seriesId) return;
    await deleteSeriesMutation.mutateAsync({ seriesId });
    setDeleteOpen(false);
    navigate(`/libraries/${libraryId}/series`);
  };

  if (seriesQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!seriesQuery.isSuccess || !seriesQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Series Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The series you're looking for doesn't exist or may have been
              removed.
            </p>
            <Link
              className="text-primary hover:underline"
              to={`/libraries/${libraryId}/series`}
            >
              Back to Series
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const series = seriesQuery.data;
  const canDelete = series.book_count === 0;

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* Series Header */}
        <div className="mb-8">
          <div className="flex items-center justify-between mb-2">
            <h1 className="text-3xl font-bold">{series.name}</h1>
            <div className="flex gap-2">
              <Button onClick={() => setEditOpen(true)} size="sm" variant="outline">
                <Edit className="h-4 w-4 mr-2" />
                Edit
              </Button>
              <Button onClick={() => setMergeOpen(true)} size="sm" variant="outline">
                <GitMerge className="h-4 w-4 mr-2" />
                Merge
              </Button>
              {canDelete && (
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
          {series.sort_name !== series.name && (
            <p className="text-muted-foreground mb-2">
              Sort name: {series.sort_name}
            </p>
          )}
          {series.description && (
            <p className="text-muted-foreground mb-2">{series.description}</p>
          )}
          <Badge variant="secondary">
            {series.book_count} book{series.book_count !== 1 ? "s" : ""}
          </Badge>
        </div>

        {/* Books in Series */}
        {series.book_count > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Books in Series</h2>
            {seriesBooksQuery.isLoading && <LoadingSpinner />}
            {seriesBooksQuery.isSuccess && (
              <div className="flex flex-wrap gap-6">
                {seriesBooksQuery.data.map((book) => (
                  <BookItem book={book} key={book.id} libraryId={libraryId!} />
                ))}
              </div>
            )}
          </section>
        )}

        {/* No Books */}
        {series.book_count === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            This series has no associated books.
          </div>
        )}

        <MetadataEditDialog
          entityName={series.name}
          entityType="series"
          isPending={updateSeriesMutation.isPending}
          onOpenChange={setEditOpen}
          onSave={handleEdit}
          open={editOpen}
          sortName={series.sort_name}
        />

        <MetadataMergeDialog
          entities={
            seriesListQuery.data?.series.map((s) => ({
              id: s.id,
              name: s.name,
              count: s.book_count ?? 0,
            })) ?? []
          }
          entityType="series"
          isLoadingEntities={seriesListQuery.isLoading}
          isPending={mergeSeriesMutation.isPending}
          onMerge={handleMerge}
          onOpenChange={setMergeOpen}
          onSearch={setMergeSearch}
          open={mergeOpen}
          targetId={seriesId!}
          targetName={series.name}
        />

        <MetadataDeleteDialog
          entityName={series.name}
          entityType="series"
          isPending={deleteSeriesMutation.isPending}
          onDelete={handleDelete}
          onOpenChange={setDeleteOpen}
          open={deleteOpen}
        />
      </div>
    </div>
  );
};

export default SeriesDetail;
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 3: Run linting**

Run: `yarn lint:eslint`
Expected: PASS with no errors

**Step 4: Commit**

```bash
git add app/components/pages/SeriesDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Rewrite SeriesDetail as full page with edit, merge, delete actions
EOF
)"
```

---

## Task 9: Create GenreDetail Page

**Files:**
- Create: `app/components/pages/GenreDetail.tsx`
- Reference: `app/components/pages/PersonDetail.tsx` (structure pattern)

**Step 1: Create the GenreDetail component**

```typescript
import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useDeleteGenre,
  useGenre,
  useGenreBooks,
  useGenresList,
  useMergeGenre,
  useUpdateGenre,
} from "@/hooks/queries/genres";
import { useDebounce } from "@/hooks/useDebounce";

const GenreDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const genreId = id ? parseInt(id, 10) : undefined;

  const genreQuery = useGenre(genreId);
  const genreBooksQuery = useGenreBooks(genreId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mergeSearch, setMergeSearch] = useState("");
  const debouncedMergeSearch = useDebounce(mergeSearch, 200);

  const updateGenreMutation = useUpdateGenre();
  const mergeGenreMutation = useMergeGenre();
  const deleteGenreMutation = useDeleteGenre();

  const genresListQuery = useGenresList(
    {
      library_id: genreQuery.data?.library_id,
      limit: 50,
      search: debouncedMergeSearch || undefined,
    },
    { enabled: mergeOpen && !!genreQuery.data?.library_id },
  );

  const handleEdit = async (data: { name: string }) => {
    if (!genreId) return;
    await updateGenreMutation.mutateAsync({
      genreId,
      payload: { name: data.name },
    });
    setEditOpen(false);
  };

  const handleMerge = async (sourceId: number) => {
    if (!genreId) return;
    await mergeGenreMutation.mutateAsync({
      targetId: genreId,
      sourceId,
    });
    setMergeOpen(false);
  };

  const handleDelete = async () => {
    if (!genreId) return;
    await deleteGenreMutation.mutateAsync({ genreId });
    setDeleteOpen(false);
    navigate(`/libraries/${libraryId}/genres`);
  };

  if (genreQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!genreQuery.isSuccess || !genreQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Genre Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The genre you're looking for doesn't exist or may have been
              removed.
            </p>
            <Link
              className="text-primary hover:underline"
              to={`/libraries/${libraryId}/genres`}
            >
              Back to Genres
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const genre = genreQuery.data;
  const bookCount = genre.book_count ?? 0;
  const canDelete = bookCount === 0;

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* Genre Header */}
        <div className="mb-8">
          <div className="flex items-center justify-between mb-2">
            <h1 className="text-3xl font-bold">{genre.name}</h1>
            <div className="flex gap-2">
              <Button onClick={() => setEditOpen(true)} size="sm" variant="outline">
                <Edit className="h-4 w-4 mr-2" />
                Edit
              </Button>
              <Button onClick={() => setMergeOpen(true)} size="sm" variant="outline">
                <GitMerge className="h-4 w-4 mr-2" />
                Merge
              </Button>
              {canDelete && (
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
          <Badge variant="secondary">
            {bookCount} book{bookCount !== 1 ? "s" : ""}
          </Badge>
        </div>

        {/* Books with this Genre */}
        {bookCount > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Books</h2>
            {genreBooksQuery.isLoading && <LoadingSpinner />}
            {genreBooksQuery.isSuccess && (
              <div className="flex flex-wrap gap-6">
                {genreBooksQuery.data.map((book) => (
                  <BookItem book={book} key={book.id} libraryId={libraryId!} />
                ))}
              </div>
            )}
          </section>
        )}

        {/* No Books */}
        {bookCount === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            This genre has no associated books.
          </div>
        )}

        <MetadataEditDialog
          entityName={genre.name}
          entityType="genre"
          isPending={updateGenreMutation.isPending}
          onOpenChange={setEditOpen}
          onSave={handleEdit}
          open={editOpen}
        />

        <MetadataMergeDialog
          entities={
            genresListQuery.data?.genres.map((g) => ({
              id: g.id,
              name: g.name,
              count: g.book_count ?? 0,
            })) ?? []
          }
          entityType="genre"
          isLoadingEntities={genresListQuery.isLoading}
          isPending={mergeGenreMutation.isPending}
          onMerge={handleMerge}
          onOpenChange={setMergeOpen}
          onSearch={setMergeSearch}
          open={mergeOpen}
          targetId={genreId!}
          targetName={genre.name}
        />

        <MetadataDeleteDialog
          entityName={genre.name}
          entityType="genre"
          isPending={deleteGenreMutation.isPending}
          onDelete={handleDelete}
          onOpenChange={setDeleteOpen}
          open={deleteOpen}
        />
      </div>
    </div>
  );
};

export default GenreDetail;
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 3: Commit**

```bash
git add app/components/pages/GenreDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add GenreDetail page with edit, merge, delete actions
EOF
)"
```

---

## Task 10: Create TagDetail Page

**Files:**
- Create: `app/components/pages/TagDetail.tsx`
- Reference: `app/components/pages/GenreDetail.tsx` (identical pattern)

**Step 1: Create the TagDetail component**

```typescript
import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useDeleteTag,
  useMergeTag,
  useTag,
  useTagBooks,
  useTagsList,
  useUpdateTag,
} from "@/hooks/queries/tags";
import { useDebounce } from "@/hooks/useDebounce";

const TagDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const tagId = id ? parseInt(id, 10) : undefined;

  const tagQuery = useTag(tagId);
  const tagBooksQuery = useTagBooks(tagId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mergeSearch, setMergeSearch] = useState("");
  const debouncedMergeSearch = useDebounce(mergeSearch, 200);

  const updateTagMutation = useUpdateTag();
  const mergeTagMutation = useMergeTag();
  const deleteTagMutation = useDeleteTag();

  const tagsListQuery = useTagsList(
    {
      library_id: tagQuery.data?.library_id,
      limit: 50,
      search: debouncedMergeSearch || undefined,
    },
    { enabled: mergeOpen && !!tagQuery.data?.library_id },
  );

  const handleEdit = async (data: { name: string }) => {
    if (!tagId) return;
    await updateTagMutation.mutateAsync({
      tagId,
      payload: { name: data.name },
    });
    setEditOpen(false);
  };

  const handleMerge = async (sourceId: number) => {
    if (!tagId) return;
    await mergeTagMutation.mutateAsync({
      targetId: tagId,
      sourceId,
    });
    setMergeOpen(false);
  };

  const handleDelete = async () => {
    if (!tagId) return;
    await deleteTagMutation.mutateAsync({ tagId });
    setDeleteOpen(false);
    navigate(`/libraries/${libraryId}/tags`);
  };

  if (tagQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!tagQuery.isSuccess || !tagQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Tag Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The tag you're looking for doesn't exist or may have been
              removed.
            </p>
            <Link
              className="text-primary hover:underline"
              to={`/libraries/${libraryId}/tags`}
            >
              Back to Tags
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const tag = tagQuery.data;
  const bookCount = tag.book_count ?? 0;
  const canDelete = bookCount === 0;

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* Tag Header */}
        <div className="mb-8">
          <div className="flex items-center justify-between mb-2">
            <h1 className="text-3xl font-bold">{tag.name}</h1>
            <div className="flex gap-2">
              <Button onClick={() => setEditOpen(true)} size="sm" variant="outline">
                <Edit className="h-4 w-4 mr-2" />
                Edit
              </Button>
              <Button onClick={() => setMergeOpen(true)} size="sm" variant="outline">
                <GitMerge className="h-4 w-4 mr-2" />
                Merge
              </Button>
              {canDelete && (
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
          <Badge variant="secondary">
            {bookCount} book{bookCount !== 1 ? "s" : ""}
          </Badge>
        </div>

        {/* Books with this Tag */}
        {bookCount > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Books</h2>
            {tagBooksQuery.isLoading && <LoadingSpinner />}
            {tagBooksQuery.isSuccess && (
              <div className="flex flex-wrap gap-6">
                {tagBooksQuery.data.map((book) => (
                  <BookItem book={book} key={book.id} libraryId={libraryId!} />
                ))}
              </div>
            )}
          </section>
        )}

        {/* No Books */}
        {bookCount === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            This tag has no associated books.
          </div>
        )}

        <MetadataEditDialog
          entityName={tag.name}
          entityType="tag"
          isPending={updateTagMutation.isPending}
          onOpenChange={setEditOpen}
          onSave={handleEdit}
          open={editOpen}
        />

        <MetadataMergeDialog
          entities={
            tagsListQuery.data?.tags.map((t) => ({
              id: t.id,
              name: t.name,
              count: t.book_count ?? 0,
            })) ?? []
          }
          entityType="tag"
          isLoadingEntities={tagsListQuery.isLoading}
          isPending={mergeTagMutation.isPending}
          onMerge={handleMerge}
          onOpenChange={setMergeOpen}
          onSearch={setMergeSearch}
          open={mergeOpen}
          targetId={tagId!}
          targetName={tag.name}
        />

        <MetadataDeleteDialog
          entityName={tag.name}
          entityType="tag"
          isPending={deleteTagMutation.isPending}
          onDelete={handleDelete}
          onOpenChange={setDeleteOpen}
          open={deleteOpen}
        />
      </div>
    </div>
  );
};

export default TagDetail;
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 3: Commit**

```bash
git add app/components/pages/TagDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add TagDetail page with edit, merge, delete actions
EOF
)"
```

---

## Task 11: Add Routes for GenreDetail and TagDetail

**Files:**
- Modify: `app/router.tsx`

**Step 1: Add imports for GenreDetail and TagDetail**

Add to the imports:

```typescript
import GenreDetail from "@/components/pages/GenreDetail";
import TagDetail from "@/components/pages/TagDetail";
```

**Step 2: Add route for GenreDetail after GenresList route**

After the `/libraries/:libraryId/genres` route (around line 215), add:

```typescript
{
  path: "libraries/:libraryId/genres/:id",
  element: (
    <ProtectedRoute checkLibraryAccess>
      <GenreDetail />
    </ProtectedRoute>
  ),
},
```

**Step 3: Add route for TagDetail after TagsList route**

After the `/libraries/:libraryId/tags` route (around line 222), add:

```typescript
{
  path: "libraries/:libraryId/tags/:id",
  element: (
    <ProtectedRoute checkLibraryAccess>
      <TagDetail />
    </ProtectedRoute>
  ),
},
```

**Step 4: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 5: Commit**

```bash
git add app/router.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add routes for GenreDetail and TagDetail pages
EOF
)"
```

---

## Task 12: Update GenresList to Link to GenreDetail

**Files:**
- Modify: `app/components/pages/GenresList.tsx`

**Step 1: Update the Link in renderGenreItem**

Change the link destination from filtering home page to the detail page:

```typescript
const renderGenreItem = (genre: Genre) => {
  const bookCount = genre.book_count ?? 0;

  return (
    <Link
      className="flex items-center justify-between p-3 rounded-md hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
      key={genre.id}
      to={`/libraries/${libraryId}/genres/${genre.id}`}
    >
      <span className="font-medium">{genre.name}</span>
      <Badge variant="secondary">
        {bookCount} book{bookCount !== 1 ? "s" : ""}
      </Badge>
    </Link>
  );
};
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 3: Commit**

```bash
git add app/components/pages/GenresList.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Update GenresList to link to GenreDetail pages
EOF
)"
```

---

## Task 13: Update TagsList to Link to TagDetail

**Files:**
- Modify: `app/components/pages/TagsList.tsx`

**Step 1: Update the Link in renderTagItem**

Change the link destination from filtering home page to the detail page:

```typescript
const renderTagItem = (tag: Tag) => {
  const bookCount = tag.book_count ?? 0;

  return (
    <Link
      className="flex items-center justify-between p-3 rounded-md hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
      key={tag.id}
      to={`/libraries/${libraryId}/tags/${tag.id}`}
    >
      <span className="font-medium">{tag.name}</span>
      <Badge variant="secondary">
        {bookCount} book{bookCount !== 1 ? "s" : ""}
      </Badge>
    </Link>
  );
};
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS with no type errors

**Step 3: Commit**

```bash
git add app/components/pages/TagsList.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Update TagsList to link to TagDetail pages
EOF
)"
```

---

## Task 14: Run Full Validation

**Step 1: Run make check**

Run: `make check`
Expected: All tests and linting pass

**Step 2: If any errors, fix them and re-run**

**Step 3: Final commit if needed**

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Frontend] Fix any remaining lint/type issues
EOF
)"
```

---

## Task 15: Manual Testing Checklist

Test each feature manually in the browser:

**Person:**
- [ ] Navigate to a person detail page
- [ ] Click Edit - verify dialog opens with name and sort_name
- [ ] Change name and save - verify update persists
- [ ] Click Merge - verify dropdown shows other people in library
- [ ] Select a person and merge - verify books transfer
- [ ] Delete button only visible when person has 0 books/files
- [ ] Delete an orphan person - verify redirect to list

**Series:**
- [ ] Navigate to a series detail page (no longer redirects)
- [ ] Verify books in series are displayed
- [ ] Click Edit - verify name, sort_name, description fields
- [ ] Click Merge - verify dropdown excludes current series
- [ ] Delete button only visible when series has 0 books

**Genre:**
- [ ] Click a genre from GenresList - navigates to GenreDetail
- [ ] Edit, merge, delete work as expected

**Tag:**
- [ ] Click a tag from TagsList - navigates to TagDetail
- [ ] Edit, merge, delete work as expected

---

## Summary

This plan implements the metadata entity editing feature through:

1. **Tasks 1-3:** Add mutation hooks (update, merge, delete) for all entity types
2. **Tasks 4-6:** Create reusable dialog components for edit, merge, and delete operations
3. **Tasks 7-8:** Update PersonDetail and SeriesDetail with action buttons
4. **Tasks 9-10:** Create new GenreDetail and TagDetail pages
5. **Tasks 11-13:** Add routes and update list pages to link to detail pages
6. **Tasks 14-15:** Final validation and manual testing

The backend API already supports all operations, so no backend changes are needed.
