import { Check, ChevronsUpDown, Loader2, Plus, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MultiSelectCombobox } from "@/components/ui/MultiSelectCombobox";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
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
import { useSeriesList } from "@/hooks/queries/series";
import { useTagsList } from "@/hooks/queries/tags";
import { useDebounce } from "@/hooks/useDebounce";
import {
  AuthorRoleColorist,
  AuthorRoleCoverArtist,
  AuthorRoleEditor,
  AuthorRoleInker,
  AuthorRoleLetterer,
  AuthorRolePenciller,
  AuthorRoleTranslator,
  AuthorRoleWriter,
  FileTypeCBZ,
  type AuthorInput,
  type Book,
  type SeriesInput,
} from "@/types";

interface BookEditDialogProps {
  book: Book;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface SeriesEntry {
  name: string;
  number: string;
}

// Author role options for CBZ files
const AUTHOR_ROLES = [
  { value: AuthorRoleWriter, label: "Writer" },
  { value: AuthorRolePenciller, label: "Penciller" },
  { value: AuthorRoleInker, label: "Inker" },
  { value: AuthorRoleColorist, label: "Colorist" },
  { value: AuthorRoleLetterer, label: "Letterer" },
  { value: AuthorRoleCoverArtist, label: "Cover Artist" },
  { value: AuthorRoleEditor, label: "Editor" },
  { value: AuthorRoleTranslator, label: "Translator" },
] as const;

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
  const [newAuthor, setNewAuthor] = useState("");
  const [newAuthorRole, setNewAuthorRole] = useState<string | undefined>(
    undefined,
  );
  const [seriesEntries, setSeriesEntries] = useState<SeriesEntry[]>(
    book.book_series?.map((bs) => ({
      name: bs.series?.name || "",
      number: bs.series_number?.toString() || "",
    })) || [],
  );
  const [seriesOpen, setSeriesOpen] = useState(false);
  const [seriesSearch, setSeriesSearch] = useState("");
  const debouncedSeriesSearch = useDebounce(seriesSearch, 200);

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

  // Check if book has CBZ files (determines whether to show role selection)
  const hasCBZFiles = book.files?.some((f) => f.file_type === FileTypeCBZ);

  // Query for series in this library with server-side search
  const { data: seriesData, isLoading: isLoadingSeries } = useSeriesList(
    {
      library_id: book.library_id,
      limit: 50,
      search: debouncedSeriesSearch || undefined,
    },
    { enabled: open && !!book.library_id },
  );

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

  // Reset form when dialog opens with new book data
  useEffect(() => {
    if (open) {
      setTitle(book.title);
      setSortTitle(book.sort_title || "");
      setSubtitle(book.subtitle || "");
      setDescription(book.description || "");
      setAuthors(
        book.authors?.map((a) => ({
          name: a.person?.name || "",
          role: a.role,
        })) || [],
      );
      setNewAuthor("");
      setNewAuthorRole(undefined);
      setSeriesEntries(
        book.book_series?.map((bs) => ({
          name: bs.series?.name || "",
          number: bs.series_number?.toString() || "",
        })) || [],
      );
      setGenres(
        book.book_genres?.map((bg) => bg.genre?.name || "").filter(Boolean) ||
          [],
      );
      setGenreSearch("");
      setTags(
        book.book_tags?.map((bt) => bt.tag?.name || "").filter(Boolean) || [],
      );
      setTagSearch("");
    }
  }, [open, book]);

  const handleAddAuthor = () => {
    const name = newAuthor.trim();
    if (
      name &&
      !authors.some((a) => a.name === name && a.role === newAuthorRole)
    ) {
      setAuthors([...authors, { name, role: newAuthorRole }]);
      setNewAuthor("");
      setNewAuthorRole(undefined);
    }
  };

  const handleRemoveAuthor = (index: number) => {
    setAuthors(authors.filter((_, i) => i !== index));
  };

  const handleAuthorRoleChange = (index: number, role: string | undefined) => {
    const updated = [...authors];
    updated[index] = { ...updated[index], role };
    setAuthors(updated);
  };

  const handleSelectSeries = (seriesName: string) => {
    // Check if series is already added
    if (!seriesEntries.find((s) => s.name === seriesName)) {
      setSeriesEntries([...seriesEntries, { name: seriesName, number: "" }]);
    }
    setSeriesOpen(false);
    setSeriesSearch("");
  };

  const handleCreateSeries = () => {
    if (
      seriesSearch.trim() &&
      !seriesEntries.find((s) => s.name === seriesSearch.trim())
    ) {
      setSeriesEntries([
        ...seriesEntries,
        { name: seriesSearch.trim(), number: "" },
      ]);
    }
    setSeriesOpen(false);
    setSeriesSearch("");
  };

  const handleRemoveSeries = (index: number) => {
    setSeriesEntries(seriesEntries.filter((_, i) => i !== index));
  };

  const handleSeriesNumberChange = (index: number, value: string) => {
    const updated = [...seriesEntries];
    updated[index].number = value;
    setSeriesEntries(updated);
  };

  const handleAuthorBlur = () => {
    const name = newAuthor.trim();
    if (
      name &&
      !authors.some((a) => a.name === name && a.role === newAuthorRole)
    ) {
      setAuthors([...authors, { name, role: newAuthorRole }]);
      setNewAuthor("");
      setNewAuthorRole(undefined);
    }
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

    // Include any pending author from the input field
    let finalAuthors = authors;
    const pendingName = newAuthor.trim();
    if (
      pendingName &&
      !authors.some((a) => a.name === pendingName && a.role === newAuthorRole)
    ) {
      finalAuthors = [...authors, { name: pendingName, role: newAuthorRole }];
    }

    // Only include changed fields
    if (title !== book.title) {
      payload.title = title;
    }
    if (sortTitle !== (book.sort_title || "")) {
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
    if (JSON.stringify(finalAuthors) !== JSON.stringify(originalAuthors)) {
      payload.authors = finalAuthors.filter((a) => a.name.trim());
    }

    // Check if series changed
    const originalSeries =
      book.book_series?.map((bs) => ({
        name: bs.series?.name || "",
        number: bs.series_number?.toString() || "",
      })) || [];
    if (JSON.stringify(seriesEntries) !== JSON.stringify(originalSeries)) {
      payload.series = seriesEntries
        .filter((s) => s.name.trim())
        .map((s) => ({
          name: s.name,
          number: s.number ? parseFloat(s.number) : undefined,
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

    // Only submit if something changed
    if (Object.keys(payload).length === 0) {
      onOpenChange(false);
      return;
    }

    await updateBookMutation.mutateAsync({
      id: String(book.id),
      payload,
    });

    onOpenChange(false);
  };

  // Filter out already-selected series (server handles the search filtering)
  const filteredSeries = useMemo(() => {
    const allSeries = seriesData?.series || [];
    return allSeries.filter(
      (s) => !seriesEntries.find((entry) => entry.name === s.name),
    );
  }, [seriesData?.series, seriesEntries]);

  const showCreateOption =
    seriesSearch.trim() &&
    !filteredSeries.find(
      (s) => s.name.toLowerCase() === seriesSearch.toLowerCase(),
    ) &&
    !seriesEntries.find(
      (s) => s.name.toLowerCase() === seriesSearch.toLowerCase(),
    );

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Edit Book</DialogTitle>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* Title */}
          <div className="space-y-2">
            <Label htmlFor="title">Title</Label>
            <Input
              id="title"
              onChange={(e) => setTitle(e.target.value)}
              value={title}
            />
          </div>

          {/* Sort Title */}
          <div className="space-y-2">
            <Label htmlFor="sort_title">Sort Title</Label>
            <Input
              id="sort_title"
              onChange={(e) => setSortTitle(e.target.value)}
              placeholder="Leave empty to auto-generate from title"
              value={sortTitle}
            />
            <p className="text-xs text-muted-foreground">
              Used for sorting. Clear to regenerate automatically.
            </p>
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
            <Label>Authors</Label>
            {hasCBZFiles ? (
              // CBZ files: show authors as rows with role selection
              <div className="space-y-2">
                {authors.map((author, index) => (
                  <div className="flex items-center gap-2" key={index}>
                    <div className="flex-1">
                      <Input disabled value={author.name} />
                    </div>
                    <div className="w-36">
                      <Select
                        onValueChange={(value) =>
                          handleAuthorRoleChange(
                            index,
                            value === "none" ? undefined : value,
                          )
                        }
                        value={author.role || "none"}
                      >
                        <SelectTrigger>
                          <SelectValue placeholder="Role" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="none">No role</SelectItem>
                          {AUTHOR_ROLES.map((role) => (
                            <SelectItem key={role.value} value={role.value}>
                              {role.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <Button
                      onClick={() => handleRemoveAuthor(index)}
                      size="icon"
                      type="button"
                      variant="ghost"
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
                <div className="flex gap-2">
                  <div className="flex-1">
                    <Input
                      onChange={(e) => setNewAuthor(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") {
                          e.preventDefault();
                          handleAddAuthor();
                        }
                      }}
                      placeholder="Add author..."
                      value={newAuthor}
                    />
                  </div>
                  <div className="w-36">
                    <Select
                      onValueChange={(value) =>
                        setNewAuthorRole(value === "none" ? undefined : value)
                      }
                      value={newAuthorRole || "none"}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Role" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">No role</SelectItem>
                        {AUTHOR_ROLES.map((role) => (
                          <SelectItem key={role.value} value={role.value}>
                            {role.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <Button
                    onClick={handleAddAuthor}
                    size="icon"
                    type="button"
                    variant="outline"
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ) : (
              // Non-CBZ files: simple badge-based author list
              <>
                <div className="flex flex-wrap gap-2 mb-2">
                  {authors.map((author, index) => (
                    <Badge
                      className="flex items-center gap-1"
                      key={index}
                      variant="secondary"
                    >
                      {author.name}
                      <button
                        className="ml-1 cursor-pointer hover:text-destructive"
                        onClick={() => handleRemoveAuthor(index)}
                        type="button"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </Badge>
                  ))}
                </div>
                <div className="flex gap-2">
                  <Input
                    onBlur={handleAuthorBlur}
                    onChange={(e) => setNewAuthor(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        handleAddAuthor();
                      }
                    }}
                    placeholder="Add author..."
                    value={newAuthor}
                  />
                  <Button
                    onClick={handleAddAuthor}
                    type="button"
                    variant="outline"
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              </>
            )}
          </div>

          {/* Series */}
          <div className="space-y-2">
            <Label>Series</Label>
            <div className="space-y-2">
              {seriesEntries.map((entry, index) => (
                <div className="flex items-center gap-2" key={index}>
                  <div className="flex-1">
                    <Input disabled value={entry.name} />
                  </div>
                  <div className="w-24">
                    <Input
                      onChange={(e) =>
                        handleSeriesNumberChange(index, e.target.value)
                      }
                      placeholder="#"
                      type="number"
                      value={entry.number}
                    />
                  </div>
                  <Button
                    onClick={() => handleRemoveSeries(index)}
                    size="icon"
                    type="button"
                    variant="ghost"
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>

            {/* Series Combobox */}
            <Popover modal onOpenChange={setSeriesOpen} open={seriesOpen}>
              <PopoverTrigger asChild>
                <Button
                  aria-expanded={seriesOpen}
                  className="w-full justify-between"
                  role="combobox"
                  variant="outline"
                >
                  Add series...
                  <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                </Button>
              </PopoverTrigger>
              <PopoverContent align="start" className="w-full p-0">
                <Command shouldFilter={false}>
                  <CommandInput
                    onValueChange={setSeriesSearch}
                    placeholder="Search series..."
                    value={seriesSearch}
                  />
                  <CommandList>
                    {isLoadingSeries && (
                      <div className="p-4 text-center text-sm text-muted-foreground">
                        Loading series...
                      </div>
                    )}
                    {!isLoadingSeries &&
                      filteredSeries.length === 0 &&
                      !showCreateOption && (
                        <div className="p-4 text-center text-sm text-muted-foreground">
                          {!debouncedSeriesSearch
                            ? "No series in this library. Type to create one."
                            : "No matching series."}
                        </div>
                      )}
                    {!isLoadingSeries && (
                      <CommandGroup>
                        {filteredSeries.map((s) => (
                          <CommandItem
                            key={s.id}
                            onSelect={() => handleSelectSeries(s.name)}
                            value={s.name}
                          >
                            <Check className="mr-2 h-4 w-4 opacity-0" />
                            {s.name}
                          </CommandItem>
                        ))}
                        {showCreateOption && (
                          <CommandItem
                            onSelect={handleCreateSeries}
                            value={`create-${seriesSearch}`}
                          >
                            <Plus className="mr-2 h-4 w-4" />
                            Create "{seriesSearch}"
                          </CommandItem>
                        )}
                      </CommandGroup>
                    )}
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>
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
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={updateBookMutation.isPending}
            onClick={handleSubmit}
          >
            {updateBookMutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
