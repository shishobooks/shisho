import {
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/libraries/utils";

interface PaginationFooterProps {
  currentPage: number;
  totalPages: number;
  /** `replace` is set when the footer corrects a page past the end, so Back skips it. */
  onPageChange: (page: number, options?: { replace?: boolean }) => void;
  className?: string;
}

// Pages listed on each side of the current one before anything is typed.
const PAGE_WINDOW = 50;
// Cap on typed matches, so "1" on a huge list stays cheap to render.
const MAX_MATCHES = 100;

function pageOptions(
  query: string,
  currentPage: number,
  totalPages: number,
): number[] {
  const pages: number[] = [];
  const digits = query.replace(/\D/g, "");
  if (digits) {
    for (let p = 1; p <= totalPages && pages.length < MAX_MATCHES; p++) {
      if (String(p).startsWith(digits)) pages.push(p);
    }
    return pages;
  }
  const last = Math.min(totalPages, currentPage + PAGE_WINDOW);
  for (let p = Math.max(1, currentPage - PAGE_WINDOW); p <= last; p++) {
    pages.push(p);
  }
  return pages;
}

const NavButton = ({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string;
  disabled: boolean;
  onClick: () => void;
  children: ReactNode;
}) => (
  <Button
    aria-label={label}
    disabled={disabled}
    onClick={onClick}
    size="icon"
    variant="ghost"
  >
    {children}
  </Button>
);

const PaginationFooter = ({
  currentPage,
  totalPages,
  onPageChange,
  className,
}: PaginationFooterProps) => {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [highlighted, setHighlighted] = useState("");
  const currentRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  // A page past the end (a bookmarked URL, or items deleted since) would show
  // an empty page under a range like "Showing 401-367".
  useEffect(() => {
    if (totalPages >= 1 && currentPage > totalPages) {
      onPageChange(totalPages, { replace: true });
    }
  }, [currentPage, totalPages, onPageChange]);

  const options = useMemo(
    () => (open ? pageOptions(query, currentPage, totalPages) : []),
    [open, query, currentPage, totalPages],
  );

  useEffect(() => {
    if (!open) return;
    if (query) {
      setHighlighted(options.length > 0 ? String(options[0]) : "");
      // The list may still be scrolled to where the current page was.
      if (listRef.current) listRef.current.scrollTop = 0;
    } else {
      setHighlighted(String(currentPage));
      requestAnimationFrame(() =>
        currentRef.current?.scrollIntoView({ block: "center" }),
      );
    }
  }, [open, query, options, currentPage]);

  if (totalPages <= 1) return null;

  const goTo = (page: number) => {
    setOpen(false);
    setQuery("");
    if (page !== currentPage) onPageChange(page);
  };

  return (
    <nav
      aria-label="Pagination"
      className={cn("flex items-center justify-center gap-1", className)}
    >
      <NavButton
        disabled={currentPage <= 1}
        label="Go to first page"
        onClick={() => onPageChange(1)}
      >
        <ChevronsLeft />
      </NavButton>
      <NavButton
        disabled={currentPage <= 1}
        label="Go to previous page"
        onClick={() => onPageChange(currentPage - 1)}
      >
        <ChevronLeft />
      </NavButton>
      <Popover
        onOpenChange={(next) => {
          setOpen(next);
          if (!next) setQuery("");
        }}
        open={open}
      >
        <PopoverTrigger asChild>
          <Button className="mx-1 gap-2 tabular-nums" variant="outline">
            Page {currentPage.toLocaleString()} of {totalPages.toLocaleString()}
            <ChevronDown className="opacity-60" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-56 p-0">
          <Command
            label="Go to page"
            onValueChange={setHighlighted}
            shouldFilter={false}
            value={highlighted}
          >
            <CommandInput
              inputMode="numeric"
              onValueChange={setQuery}
              placeholder="Type a page number..."
              value={query}
            />
            <CommandList className="max-h-72" ref={listRef}>
              <CommandEmpty>No such page.</CommandEmpty>
              {options.map((page) => {
                const isCurrent = page === currentPage;
                return (
                  <CommandItem
                    aria-current={isCurrent ? "page" : undefined}
                    className="tabular-nums"
                    key={page}
                    onSelect={() => goTo(page)}
                    ref={isCurrent ? currentRef : undefined}
                    value={String(page)}
                  >
                    Page {page.toLocaleString()}
                    {isCurrent && <Check className="ml-auto" />}
                  </CommandItem>
                );
              })}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      <NavButton
        disabled={currentPage >= totalPages}
        label="Go to next page"
        onClick={() => onPageChange(currentPage + 1)}
      >
        <ChevronRight />
      </NavButton>
      <NavButton
        disabled={currentPage >= totalPages}
        label="Go to last page"
        onClick={() => onPageChange(totalPages)}
      >
        <ChevronsRight />
      </NavButton>
    </nav>
  );
};

export default PaginationFooter;
