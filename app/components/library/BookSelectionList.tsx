import { Loader2, Plus, Search } from "lucide-react";
import { useState } from "react";

import { Input } from "@/components/ui/input";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useBooks } from "@/hooks/queries/books";
import { useDebounce } from "@/hooks/useDebounce";
import { cn } from "@/libraries/utils";
import type { Book } from "@/types";

interface BookSelectionListProps {
  /** Library ID to fetch books from */
  libraryId: number;
  /** Book ID to exclude from the list (e.g., source book) */
  excludeBookId?: number;
  /** Currently selected book ID, or "new" for create new book option */
  selectedBookId: string;
  /** Callback when a book is selected */
  onSelectBook: (bookId: string) => void;
  /** Whether to show the "Create new book" option */
  showCreateNew?: boolean;
  /** Whether the parent dialog is open (controls query enabled state) */
  enabled?: boolean;
}

export function BookSelectionList({
  libraryId,
  excludeBookId,
  selectedBookId,
  onSelectBook,
  showCreateNew = false,
  enabled = true,
}: BookSelectionListProps) {
  const [searchInput, setSearchInput] = useState("");
  const debouncedSearch = useDebounce(searchInput, 300);

  const booksQuery = useBooks(
    {
      library_id: libraryId,
      limit: 50,
      search: debouncedSearch || undefined,
    },
    { enabled },
  );

  const availableBooks =
    booksQuery.data?.books?.filter((book) => book.id !== excludeBookId) ?? [];

  return (
    <RadioGroup onValueChange={onSelectBook} value={selectedBookId}>
      {showCreateNew && (
        <label
          className="flex items-center space-x-2 p-2 rounded-md hover:bg-muted/50 cursor-pointer"
          htmlFor="new"
        >
          <RadioGroupItem className="shrink-0" id="new" value="new" />
          <Plus className="h-4 w-4 shrink-0" />
          <span>Create new book</span>
        </label>
      )}

      {booksQuery.isLoading && (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {!booksQuery.isLoading && (
        <>
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              className="pl-9"
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="Search books..."
              value={searchInput}
            />
            {booksQuery.isFetching && (
              <Loader2 className="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 animate-spin text-muted-foreground" />
            )}
          </div>

          <ScrollArea className="h-48 border rounded-md p-2">
            {availableBooks.length === 0 ? (
              <div className="text-sm text-muted-foreground text-center py-4">
                {debouncedSearch
                  ? "No books match your search"
                  : "No other books in this library"}
              </div>
            ) : (
              availableBooks.map((book) => (
                <BookSelectionItem
                  book={book}
                  isSelected={String(book.id) === selectedBookId}
                  key={book.id}
                />
              ))
            )}
          </ScrollArea>
        </>
      )}
    </RadioGroup>
  );
}

interface BookSelectionItemProps {
  book: Book;
  isSelected: boolean;
}

function BookSelectionItem({ book, isSelected }: BookSelectionItemProps) {
  const authorsText = [
    ...new Set(book.authors?.map((a) => a.person?.name).filter(Boolean)),
  ].join(", ");

  return (
    <label
      className={cn(
        "flex items-start gap-3 p-2 rounded-md cursor-pointer transition-colors overflow-hidden",
        isSelected ? "bg-primary/10" : "hover:bg-muted/50",
      )}
      htmlFor={`book-${book.id}`}
    >
      <RadioGroupItem
        className="mt-0.5 shrink-0"
        id={`book-${book.id}`}
        value={String(book.id)}
      />
      <div className="min-w-0 flex-1 overflow-hidden">
        <div className="font-medium truncate" title={book.title}>
          {book.title}
        </div>
        <div className="flex items-center gap-1 text-sm text-muted-foreground">
          <span className="shrink-0">
            {book.files?.length || 0} file
            {(book.files?.length || 0) !== 1 ? "s" : ""}
          </span>
          {authorsText && (
            <>
              <span className="shrink-0 text-muted-foreground/50">·</span>
              <span className="truncate" title={authorsText}>
                {authorsText}
              </span>
            </>
          )}
        </div>
      </div>
    </label>
  );
}
