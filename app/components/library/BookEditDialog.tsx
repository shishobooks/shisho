import { Check, ChevronsUpDown, Loader2, Plus, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { SortNameInput } from "@/components/common/SortNameInput";
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
import { usePeopleList } from "@/hooks/queries/people";
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
  const [seriesEntries, setSeriesEntries] = useState<SeriesEntry[]>(
    book.book_series?.map((bs) => ({
      name: bs.series?.name || "",
      number: bs.series_number?.toString() || "",
    })) || [],
  );
  const [seriesOpen, setSeriesOpen] = useState(false);
  const [seriesSearch, setSeriesSearch] = useState("");
  const [authorOpen, setAuthorOpen] = useState(false);
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

  const [authorSearch, setAuthorSearch] = useState("");
  const debouncedAuthorSearch = useDebounce(authorSearch, 200);

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

  // Query for people in this library with server-side search
  const { data: peopleData, isLoading: isLoadingPeople } = usePeopleList(
    {
      library_id: book.library_id,
      limit: 50,
      search: debouncedAuthorSearch || undefined,
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
      setAuthorSearch("");
    }
  }, [open, book]);

  const handleRemoveAuthor = (index: number) => {
    setAuthors(authors.filter((_, i) => i !== index));
  };

  const handleAuthorRoleChange = (index: number, role: string | undefined) => {
    const updated = [...authors];
    updated[index] = { ...updated[index], role };
    setAuthors(updated);
  };

  const handleSelectAuthor = (personName: string) => {
    // Check if author is already added (same name, any role)
    if (!authors.some((a) => a.name === personName)) {
      // Default to "Writer" role for CBZ
      setAuthors([...authors, { name: personName, role: AuthorRoleWriter }]);
    }
    setAuthorOpen(false);
    setAuthorSearch("");
  };

  const handleCreateAuthor = () => {
    const name = authorSearch.trim();
    if (name && !authors.some((a) => a.name === name)) {
      setAuthors([...authors, { name, role: AuthorRoleWriter }]);
    }
    setAuthorOpen(false);
    setAuthorSearch("");
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
    if (JSON.stringify(authors) !== JSON.stringify(originalAuthors)) {
      payload.authors = authors.filter((a) => a.name.trim());
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

  // Filter out already-selected authors from people options
  const filteredPeople = useMemo(() => {
    const allPeople = peopleData?.people || [];
    return allPeople.filter((p) => !authors.some((a) => a.name === p.name));
  }, [peopleData?.people, authors]);

  const showCreateAuthorOption =
    authorSearch.trim() &&
    !filteredPeople.find(
      (p) => p.name.toLowerCase() === authorSearch.toLowerCase(),
    ) &&
    !authors.find((a) => a.name.toLowerCase() === authorSearch.toLowerCase());

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
                {/* Author Combobox for CBZ */}
                <Popover modal onOpenChange={setAuthorOpen} open={authorOpen}>
                  <PopoverTrigger asChild>
                    <Button
                      aria-expanded={authorOpen}
                      className="w-full justify-between"
                      role="combobox"
                      variant="outline"
                    >
                      Add author...
                      <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent align="start" className="w-full p-0">
                    <Command shouldFilter={false}>
                      <CommandInput
                        onValueChange={setAuthorSearch}
                        placeholder="Search people..."
                        value={authorSearch}
                      />
                      <CommandList>
                        {isLoadingPeople && (
                          <div className="p-4 text-center text-sm text-muted-foreground">
                            Loading people...
                          </div>
                        )}
                        {!isLoadingPeople &&
                          filteredPeople.length === 0 &&
                          !showCreateAuthorOption && (
                            <div className="p-4 text-center text-sm text-muted-foreground">
                              {!debouncedAuthorSearch
                                ? "No people in this library. Type to create one."
                                : "No matching people."}
                            </div>
                          )}
                        {!isLoadingPeople && (
                          <CommandGroup>
                            {filteredPeople.map((p) => (
                              <CommandItem
                                key={p.id}
                                onSelect={() => handleSelectAuthor(p.name)}
                                value={p.name}
                              >
                                <Check className="mr-2 h-4 w-4 opacity-0 shrink-0" />
                                <span className="truncate" title={p.name}>
                                  {p.name}
                                </span>
                              </CommandItem>
                            ))}
                            {showCreateAuthorOption && (
                              <CommandItem
                                onSelect={handleCreateAuthor}
                                value={`create-${authorSearch}`}
                              >
                                <Plus className="mr-2 h-4 w-4 shrink-0" />
                                <span className="truncate">
                                  Create "{authorSearch}"
                                </span>
                              </CommandItem>
                            )}
                          </CommandGroup>
                        )}
                      </CommandList>
                    </Command>
                  </PopoverContent>
                </Popover>
              </div>
            ) : (
              // Non-CBZ files: use MultiSelectCombobox for authors
              <MultiSelectCombobox
                isLoading={isLoadingPeople}
                label="People"
                onChange={(names) =>
                  setAuthors(names.map((name) => ({ name })))
                }
                onSearch={setAuthorSearch}
                options={peopleData?.people.map((p) => p.name) || []}
                placeholder="Add author..."
                searchValue={authorSearch}
                values={authors.map((a) => a.name)}
              />
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
                            <Check className="mr-2 h-4 w-4 opacity-0 shrink-0" />
                            <span className="truncate" title={s.name}>
                              {s.name}
                            </span>
                          </CommandItem>
                        ))}
                        {showCreateOption && (
                          <CommandItem
                            onSelect={handleCreateSeries}
                            value={`create-${seriesSearch}`}
                          >
                            <Plus className="mr-2 h-4 w-4 shrink-0" />
                            <span className="truncate">
                              Create "{seriesSearch}"
                            </span>
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
