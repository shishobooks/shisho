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
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Textarea } from "@/components/ui/textarea";
import { useUpdateBook } from "@/hooks/queries/books";
import { useSeriesList } from "@/hooks/queries/series";
import { useDebounce } from "@/hooks/useDebounce";
import type { Book, SeriesInput } from "@/types";

interface BookEditDialogProps {
  book: Book;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface SeriesEntry {
  name: string;
  number: string;
}

export function BookEditDialog({
  book,
  open,
  onOpenChange,
}: BookEditDialogProps) {
  const [title, setTitle] = useState(book.title);
  const [sortTitle, setSortTitle] = useState(book.sort_title || "");
  const [subtitle, setSubtitle] = useState(book.subtitle || "");
  const [authors, setAuthors] = useState<string[]>(
    book.authors?.map((a) => a.person?.name || "") || [],
  );
  const [newAuthor, setNewAuthor] = useState("");
  const [seriesEntries, setSeriesEntries] = useState<SeriesEntry[]>(
    book.book_series?.map((bs) => ({
      name: bs.series?.name || "",
      number: bs.series_number?.toString() || "",
    })) || [],
  );
  const [seriesOpen, setSeriesOpen] = useState(false);
  const [seriesSearch, setSeriesSearch] = useState("");
  const debouncedSeriesSearch = useDebounce(seriesSearch, 200);

  const updateBookMutation = useUpdateBook();

  // Query for series in this library with server-side search
  const { data: seriesData, isLoading: isLoadingSeries } = useSeriesList(
    {
      library_id: book.library_id,
      limit: 50,
      search: debouncedSeriesSearch || undefined,
    },
    { enabled: open && !!book.library_id },
  );

  // Reset form when dialog opens with new book data
  useEffect(() => {
    if (open) {
      setTitle(book.title);
      setSortTitle(book.sort_title || "");
      setSubtitle(book.subtitle || "");
      setAuthors(book.authors?.map((a) => a.person?.name || "") || []);
      setSeriesEntries(
        book.book_series?.map((bs) => ({
          name: bs.series?.name || "",
          number: bs.series_number?.toString() || "",
        })) || [],
      );
    }
  }, [open, book]);

  const handleAddAuthor = () => {
    if (newAuthor.trim() && !authors.includes(newAuthor.trim())) {
      setAuthors([...authors, newAuthor.trim()]);
      setNewAuthor("");
    }
  };

  const handleRemoveAuthor = (index: number) => {
    setAuthors(authors.filter((_, i) => i !== index));
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
    if (newAuthor.trim() && !authors.includes(newAuthor.trim())) {
      setAuthors([...authors, newAuthor.trim()]);
      setNewAuthor("");
    }
  };

  const handleSubmit = async () => {
    const payload: {
      title?: string;
      sort_title?: string;
      subtitle?: string;
      authors?: string[];
      series?: SeriesInput[];
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

    // Check if authors changed
    const originalAuthors =
      book.authors?.map((a) => a.person?.name || "") || [];
    if (JSON.stringify(authors) !== JSON.stringify(originalAuthors)) {
      payload.authors = authors;
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

          {/* Authors */}
          <div className="space-y-2">
            <Label>Authors</Label>
            <div className="flex flex-wrap gap-2 mb-2">
              {authors.map((author, index) => (
                <Badge
                  className="flex items-center gap-1"
                  key={index}
                  variant="secondary"
                >
                  {author}
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
              <Button onClick={handleAddAuthor} type="button" variant="outline">
                <Plus className="h-4 w-4" />
              </Button>
            </div>
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
