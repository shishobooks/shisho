import equal from "fast-deep-equal";
import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { SortableEntityList } from "@/components/common/SortableEntityList";
import { SortNameInput } from "@/components/common/SortNameInput";
import { ExtractSubtitleButton } from "@/components/library/ExtractSubtitleButton";
import { ReviewPanel } from "@/components/library/ReviewPanel";
import { Button } from "@/components/ui/button";
import {
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MultiSelectCombobox } from "@/components/ui/MultiSelectCombobox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useUpdateBook } from "@/hooks/queries/books";
import { useGenresList } from "@/hooks/queries/genres";
import { usePeopleList } from "@/hooks/queries/people";
import { useSetBookReview } from "@/hooks/queries/review";
import { useSeriesList } from "@/hooks/queries/series";
import { useTagsList } from "@/hooks/queries/tags";
import { useDebounce } from "@/hooks/useDebounce";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import {
  AuthorRoleWriter,
  DataSourceManual,
  FileRoleMain,
  FileTypeCBZ,
  ReviewOverrideReviewed,
  type AuthorInput,
  type Book,
  type ReviewOverride,
  type SeriesInput,
} from "@/types";
import { AUTHOR_ROLES } from "@/utils/authorRoles";
import { forTitle } from "@/utils/sortname";

interface BookEditDialogProps {
  book: Book;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface SeriesEntry {
  name: string;
  number: string;
  unit: "" | "volume" | "chapter"; // "" means unspecified
}

// Adapter hooks: bridge useXxxList query hooks to EntityCombobox's `hook` prop
// signature. Defined at module scope so they're stable references and the same
// hooks run in the same order on every render.
//
// Each adapter maps the API list-result shape (PersonWithCounts, Series) to the
// parent list's item shape (AuthorInput, SeriesEntry). The combobox only needs
// `name` to display each option; the extra fields specific to the parent list
// (role, number) are filled in by `onAppend` when the user picks an option.

function usePeopleSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: AuthorInput[]; isLoading: boolean } {
  const { data, isLoading } = usePeopleList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  // PersonWithCounts has many extra fields; the combobox only needs `name`.
  // We expose AuthorInput-shaped objects so the SortableEntityList<AuthorInput>
  // type lines up cleanly. The id is preserved on the object so getOptionKey
  // can use it.
  const adapted = data?.people.map<AuthorInput & { id: number }>((p) => ({
    name: p.name,
    id: p.id,
  }));
  return { data: adapted, isLoading };
}

function useSeriesSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: SeriesEntry[]; isLoading: boolean } {
  const { data, isLoading } = useSeriesList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  // Adapt Series[] -> SeriesEntry[] so the type matches the parent list's item
  // shape. The combobox only displays `name`; `number` is filled in when added.
  const adapted = data?.series.map<SeriesEntry>((s) => ({
    name: s.name,
    number: "",
    unit: "",
  }));
  return { data: adapted, isLoading };
}

export function BookEditDialog({
  book,
  open,
  onOpenChange,
}: BookEditDialogProps) {
  const [title, setTitle] = useState(book.title);
  const [sortTitle, setSortTitle] = useState(book.sort_title || "");
  const [subtitle, setSubtitle] = useState(book.subtitle || "");
  const [description, setDescription] = useState(book.description || "");
  const [authors, setAuthors] = useState<AuthorInput[]>(
    book.authors?.map((a) => ({
      name: a.person?.name || "",
      role: a.role,
    })) || [],
  );
  const [seriesEntries, setSeriesEntries] = useState<SeriesEntry[]>(
    book.book_series?.map(
      (bs): SeriesEntry => ({
        name: bs.series?.name || "",
        number: bs.series_number?.toString() || "",
        unit: bs.series_number_unit ?? "",
      }),
    ) || [],
  );
  const [genres, setGenres] = useState<string[]>(
    book.book_genres?.map((bg) => bg.genre?.name || "").filter(Boolean) || [],
  );
  const [genreSearch, setGenreSearch] = useState("");
  const debouncedGenreSearch = useDebounce(genreSearch, 200);

  const [tags, setTags] = useState<string[]>(
    book.book_tags?.map((bt) => bt.tag?.name || "").filter(Boolean) || [],
  );
  const [tagSearch, setTagSearch] = useState("");
  const debouncedTagSearch = useDebounce(tagSearch, 200);

  const updateBookMutation = useUpdateBook();
  const setBookReviewMutation = useSetBookReview();

  // Check if book has CBZ files (determines whether to show role selection)
  const hasCBZFiles = book.files?.some((f) => f.file_type === FileTypeCBZ);

  // Query for genres in this library with server-side search
  const { data: genresData, isLoading: isLoadingGenres } = useGenresList(
    {
      library_id: book.library_id,
      limit: 50,
      search: debouncedGenreSearch || undefined,
    },
    { enabled: open && !!book.library_id },
  );

  // Query for tags in this library with server-side search
  const { data: tagsData, isLoading: isLoadingTags } = useTagsList(
    {
      library_id: book.library_id,
      limit: 50,
      search: debouncedTagSearch || undefined,
    },
    { enabled: open && !!book.library_id },
  );

  // Draft review override — toggling the panel updates this; the actual
  // setBookReview mutation only fires on Save.
  const [draftReviewOverride, setDraftReviewOverride] =
    useState<ReviewOverride | null>(null);

  // Store initial values for change detection
  const [initialValues, setInitialValues] = useState<{
    title: string;
    sortTitle: string;
    subtitle: string;
    description: string;
    authors: AuthorInput[];
    series: SeriesEntry[];
    genres: string[];
    tags: string[];
    /** null means "auto" — no explicit override on any main file. */
    reviewOverride: ReviewOverride | null;
  } | null>(null);

  // Track previous open state to detect open transitions.
  // Start with false so that if dialog starts open, we detect it as "just opened".
  const prevOpenRef = useRef(false);

  // Initialize form only when dialog opens (closed->open transition)
  // This preserves user edits when props change while dialog is open
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;

    // Only initialize when dialog just opened, not on every prop change
    if (!justOpened) return;

    const initialTitle = book.title;
    // Semantic value for state: "" when autogenerate is ON, actual value when manual
    const semanticSortTitle =
      book.sort_title_source === DataSourceManual ? book.sort_title || "" : "";
    // Effective value for comparison: what would be displayed (accounts for generated value)
    const effectiveSortTitle = book.sort_title || forTitle(initialTitle);
    const initialSubtitle = book.subtitle || "";
    const initialDescription = book.description || "";
    const initialAuthors =
      book.authors?.map((a) => ({
        name: a.person?.name || "",
        role: a.role,
      })) || [];
    const initialSeries: SeriesEntry[] =
      book.book_series?.map((bs) => ({
        name: bs.series?.name || "",
        number: bs.series_number?.toString() || "",
        unit: bs.series_number_unit ?? "",
      })) || [];
    const initialGenres =
      book.book_genres?.map((bg) => bg.genre?.name || "").filter(Boolean) || [];
    const initialTags =
      book.book_tags?.map((bt) => bt.tag?.name || "").filter(Boolean) || [];

    // Capture the actual override state (not the aggregate `reviewed`) so we
    // can distinguish "auto-reviewed" from "explicitly reviewed". If every
    // main file shares the same override value, use it; otherwise fall back
    // to null ("auto" / mixed). This preserves user intent: toggling a book
    // that's currently auto-reviewed actually persists the explicit override.
    const mainFiles =
      book.files?.filter((f) => f.file_role === FileRoleMain) ?? [];
    let initialReviewOverride: ReviewOverride | null = null;
    if (mainFiles.length > 0) {
      const first = mainFiles[0].review_override ?? null;
      const allMatch = mainFiles.every(
        (f) => (f.review_override ?? null) === first,
      );
      if (allMatch) initialReviewOverride = first;
    }

    setTitle(initialTitle);
    // Use semantic value for state (what we send to server)
    setSortTitle(semanticSortTitle);
    setSubtitle(initialSubtitle);
    setDescription(initialDescription);
    setAuthors(initialAuthors);
    setSeriesEntries(initialSeries);
    setGenres(initialGenres);
    setGenreSearch("");
    setTags(initialTags);
    setTagSearch("");
    setDraftReviewOverride(initialReviewOverride);

    // Store initial values for comparison (use effective sort title, not semantic)
    setInitialValues({
      title: initialTitle,
      sortTitle: effectiveSortTitle,
      subtitle: initialSubtitle,
      description: initialDescription,
      authors: initialAuthors,
      series: initialSeries,
      genres: [...initialGenres].sort(),
      tags: [...initialTags].sort(),
      reviewOverride: initialReviewOverride,
    });
  }, [open, book]);

  // Compute hasChanges by comparing current values to initial values
  const hasChanges = useMemo(() => {
    if (!initialValues) return false;
    // For sort title, compare effective values (what would be displayed), not semantic values.
    // sortTitle="" means auto mode, so effective value is generated from title.
    const effectiveSortTitle = sortTitle || forTitle(title);
    return (
      title !== initialValues.title ||
      effectiveSortTitle !== initialValues.sortTitle ||
      subtitle !== initialValues.subtitle ||
      description !== initialValues.description ||
      !equal(authors, initialValues.authors) ||
      !equal(seriesEntries, initialValues.series) ||
      !equal([...genres].sort(), initialValues.genres) ||
      !equal([...tags].sort(), initialValues.tags) ||
      draftReviewOverride !== initialValues.reviewOverride
    );
  }, [
    title,
    sortTitle,
    subtitle,
    description,
    authors,
    seriesEntries,
    genres,
    tags,
    draftReviewOverride,
    initialValues,
  ]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleRemoveAuthor = (index: number) => {
    setAuthors(authors.filter((_, i) => i !== index));
  };

  const handleAuthorRoleChange = (index: number, role: string | undefined) => {
    const updated = [...authors];
    updated[index] = { ...updated[index], role };
    setAuthors(updated);
  };

  const handleAppendAuthor = (next: AuthorInput | { __create: string }) => {
    const name = "__create" in next ? next.__create : next.name;
    if (!name.trim()) return;
    if (authors.some((a) => a.name === name)) return;
    setAuthors([...authors, { name, role: AuthorRoleWriter }]);
  };

  const handleAppendSeries = (next: SeriesEntry | { __create: string }) => {
    const name = "__create" in next ? next.__create : next.name;
    if (!name.trim()) return;
    if (seriesEntries.some((s) => s.name === name)) return;
    setSeriesEntries([...seriesEntries, { name, number: "", unit: "" }]);
  };

  const handleRemoveSeries = (index: number) => {
    setSeriesEntries(seriesEntries.filter((_, i) => i !== index));
  };

  const handleSeriesNumberChange = (index: number, value: string) => {
    const updated = [...seriesEntries];
    updated[index].number = value;
    setSeriesEntries(updated);
  };

  const handleSeriesUnitChange = (
    index: number,
    unit: "" | "volume" | "chapter",
  ) => {
    const updated = [...seriesEntries];
    updated[index] = { ...updated[index], unit };
    setSeriesEntries(updated);
  };

  const handleSubmit = async () => {
    const payload: {
      title?: string;
      sort_title?: string;
      subtitle?: string;
      description?: string;
      authors?: AuthorInput[];
      series?: SeriesInput[];
      genres?: string[];
      tags?: string[];
    } = {};

    // Only include changed fields
    if (title !== book.title) {
      payload.title = title;
    }
    // Compare effective sort title against initialValues.sortTitle (snapshot)
    // This is consistent with hasChanges computation and handles the case where
    // title changes affect the auto-generated sort title even when sortTitle state is ""
    const effectiveSortTitle = sortTitle || forTitle(title);
    if (effectiveSortTitle !== initialValues?.sortTitle) {
      payload.sort_title = sortTitle;
    }
    if (subtitle !== (book.subtitle || "")) {
      payload.subtitle = subtitle;
    }
    if (description !== (book.description || "")) {
      payload.description = description;
    }

    // Check if authors changed (compare name and role)
    const originalAuthors: AuthorInput[] =
      book.authors?.map((a) => ({
        name: a.person?.name || "",
        role: a.role,
      })) || [];
    if (JSON.stringify(authors) !== JSON.stringify(originalAuthors)) {
      payload.authors = authors.filter((a) => a.name.trim());
    }

    // Check if series changed
    const originalSeries: SeriesEntry[] =
      book.book_series?.map(
        (bs): SeriesEntry => ({
          name: bs.series?.name || "",
          number: bs.series_number?.toString() || "",
          unit: bs.series_number_unit ?? "",
        }),
      ) || [];
    if (JSON.stringify(seriesEntries) !== JSON.stringify(originalSeries)) {
      payload.series = seriesEntries
        .filter((s) => s.name.trim())
        .map((s) => ({
          name: s.name,
          number: s.number ? parseFloat(s.number) : undefined,
          series_number_unit: s.unit !== "" ? s.unit : undefined,
        }));
    }

    // Check if genres changed
    const originalGenres =
      book.book_genres?.map((bg) => bg.genre?.name || "").filter(Boolean) || [];
    if (
      JSON.stringify([...genres].sort()) !==
      JSON.stringify([...originalGenres].sort())
    ) {
      payload.genres = genres.filter((g) => g.trim());
    }

    // Check if tags changed
    const originalTags =
      book.book_tags?.map((bt) => bt.tag?.name || "").filter(Boolean) || [];
    if (
      JSON.stringify([...tags].sort()) !==
      JSON.stringify([...originalTags].sort())
    ) {
      payload.tags = tags.filter((t) => t.trim());
    }

    const reviewChanged =
      draftReviewOverride !== (initialValues?.reviewOverride ?? null) &&
      draftReviewOverride !== null;

    // Only submit if something changed
    if (Object.keys(payload).length === 0 && !reviewChanged) {
      onOpenChange(false);
      return;
    }

    if (Object.keys(payload).length > 0) {
      await updateBookMutation.mutateAsync({
        id: String(book.id),
        payload,
      });
    }

    if (reviewChanged) {
      await setBookReviewMutation.mutateAsync({
        bookId: book.id,
        override: draftReviewOverride,
      });
    }

    // Reset initial values so hasChanges becomes false, then close via effect
    // Use effective sort title (not semantic) to match hasChanges comparison
    setInitialValues({
      title,
      sortTitle: sortTitle || forTitle(title),
      subtitle,
      description,
      authors: [...authors],
      series: [...seriesEntries],
      genres: [...genres].sort(),
      tags: [...tags].sort(),
      reviewOverride: draftReviewOverride,
    });
    requestClose();
  };

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>Edit Book</DialogTitle>
          <DialogDescription>
            Update the book's title, authors, series, genres, tags, and other
            metadata.
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6">
          {/* Title */}
          <div className="space-y-2">
            <Label htmlFor="title">Title</Label>
            <Input
              id="title"
              onChange={(e) => setTitle(e.target.value)}
              value={title}
            />
            <ExtractSubtitleButton
              onExtract={(t, s) => {
                setTitle(t);
                setSubtitle(s);
              }}
              title={title}
            />
          </div>

          {/* Sort Title */}
          <div className="space-y-2">
            <Label>Sort Title</Label>
            <SortNameInput
              nameValue={title}
              onChange={setSortTitle}
              sortValue={book.sort_title}
              source={book.sort_title_source}
              type="title"
            />
          </div>

          {/* Subtitle */}
          <div className="space-y-2">
            <Label htmlFor="subtitle">Subtitle</Label>
            <Textarea
              id="subtitle"
              onChange={(e) => setSubtitle(e.target.value)}
              rows={2}
              value={subtitle}
            />
          </div>

          {/* Description */}
          <div className="space-y-2">
            <Label htmlFor="description">Description</Label>
            <Textarea
              id="description"
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Book description or summary..."
              rows={4}
              value={description}
            />
          </div>

          {/* Authors */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Authors</Label>
              {authors.length > 1 && (
                <button
                  className="text-xs text-muted-foreground hover:text-destructive cursor-pointer"
                  onClick={() => setAuthors([])}
                  type="button"
                >
                  Clear all
                </button>
              )}
            </div>
            <SortableEntityList<AuthorInput>
              comboboxProps={{
                // Author options carry an extra `id` field (see usePeopleSearch);
                // use it as the option key when present, fall back to name.
                getOptionKey: (p) =>
                  (p as AuthorInput & { id?: number }).id ?? p.name,
                getOptionLabel: (p) => p.name,
                hook: function useAuthorOptions(q) {
                  return usePeopleSearch(book.library_id, open, q);
                },
                label: "Author",
              }}
              items={authors}
              onAppend={handleAppendAuthor}
              onRemove={handleRemoveAuthor}
              onReorder={setAuthors}
              renderExtras={
                hasCBZFiles
                  ? (author, idx) => (
                      <div className="w-36">
                        <Select
                          onValueChange={(value) =>
                            handleAuthorRoleChange(
                              idx,
                              value === "none" ? undefined : value,
                            )
                          }
                          value={author.role || "none"}
                        >
                          <SelectTrigger className="cursor-pointer">
                            <SelectValue placeholder="Role" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem className="cursor-pointer" value="none">
                              No role
                            </SelectItem>
                            {AUTHOR_ROLES.map((role) => (
                              <SelectItem
                                className="cursor-pointer"
                                key={role.value}
                                value={role.value}
                              >
                                {role.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    )
                  : undefined
              }
            />
          </div>

          {/* Series */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Series</Label>
              {seriesEntries.length > 1 && (
                <button
                  className="text-xs text-muted-foreground hover:text-destructive cursor-pointer"
                  onClick={() => setSeriesEntries([])}
                  type="button"
                >
                  Clear all
                </button>
              )}
            </div>
            <SortableEntityList<SeriesEntry>
              comboboxProps={{
                getOptionKey: (s) => s.name,
                getOptionLabel: (s) => s.name,
                hook: function useSeriesOptions(q) {
                  return useSeriesSearch(book.library_id, open, q);
                },
                label: "Series",
              }}
              items={seriesEntries}
              onAppend={handleAppendSeries}
              onRemove={handleRemoveSeries}
              onReorder={setSeriesEntries}
              renderExtras={(entry, idx) => (
                <>
                  <Input
                    className="w-24"
                    onChange={(e) =>
                      handleSeriesNumberChange(idx, e.target.value)
                    }
                    placeholder="#"
                    type="number"
                    value={entry.number}
                  />
                  <div className="w-32">
                    <Select
                      onValueChange={(value) =>
                        handleSeriesUnitChange(
                          idx,
                          value === "unspecified"
                            ? ""
                            : (value as "volume" | "chapter"),
                        )
                      }
                      value={entry.unit === "" ? "unspecified" : entry.unit}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Unit" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="unspecified">Unspecified</SelectItem>
                        <SelectItem value="volume">Volume</SelectItem>
                        <SelectItem value="chapter">Chapter</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </>
              )}
            />
          </div>

          {/* Genres */}
          <div className="space-y-2">
            <Label>Genres</Label>
            <MultiSelectCombobox
              isLoading={isLoadingGenres}
              label="Genres"
              onChange={setGenres}
              onSearch={setGenreSearch}
              options={genresData?.genres.map((g) => g.name) || []}
              placeholder="Add genres..."
              searchValue={genreSearch}
              values={genres}
            />
          </div>

          {/* Tags */}
          <div className="space-y-2">
            <Label>Tags</Label>
            <MultiSelectCombobox
              isLoading={isLoadingTags}
              label="Tags"
              onChange={setTags}
              onSearch={setTagSearch}
              options={tagsData?.tags.map((t) => t.name) || []}
              placeholder="Add tags..."
              searchValue={tagSearch}
              values={tags}
            />
          </div>

          {/* Review Panel — controlled (deferred to Save button).
              When draftReviewOverride is null (auto), pass undefined so the
              panel falls back to the file-derived aggregate state. */}
          <ReviewPanel
            book={book}
            files={book.files ?? []}
            isPending={setBookReviewMutation.isPending}
            onChange={(override) => setDraftReviewOverride(override)}
            toggleValue={
              draftReviewOverride === null
                ? undefined
                : draftReviewOverride === ReviewOverrideReviewed
            }
          />
        </DialogBody>

        <DialogFooter>
          <Button
            onClick={() => onOpenChange(false)}
            size="sm"
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={updateBookMutation.isPending}
            onClick={handleSubmit}
            size="sm"
          >
            {updateBookMutation.isPending && (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            )}
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </FormDialog>
  );
}
