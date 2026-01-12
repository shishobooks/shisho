import { Check, ChevronsUpDown, Loader2, Plus, Upload, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

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
import { useUpdateFile, useUploadFileCover } from "@/hooks/queries/books";
import { useImprintsList } from "@/hooks/queries/imprints";
import { usePublishersList } from "@/hooks/queries/publishers";
import { useDebounce } from "@/hooks/useDebounce";
import { FileTypeCBZ, type File } from "@/types";

interface FileEditDialogProps {
  file: File;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Helper to format date to YYYY-MM-DD for input[type="date"]
const formatDateForInput = (dateString: string | undefined): string => {
  if (!dateString) return "";
  try {
    const date = new Date(dateString);
    return date.toISOString().split("T")[0];
  } catch {
    return "";
  }
};

export function FileEditDialog({
  file,
  open,
  onOpenChange,
}: FileEditDialogProps) {
  const [narrators, setNarrators] = useState<string[]>(
    file.narrators?.map((n) => n.person?.name || "") || [],
  );
  const [newNarrator, setNewNarrator] = useState("");
  const [coverCacheBuster, setCoverCacheBuster] = useState(Date.now());
  const fileInputRef = useRef<HTMLInputElement>(null);

  // New file metadata fields
  const [url, setUrl] = useState(file.url || "");
  const [publisher, setPublisher] = useState(file.publisher?.name || "");
  const [publisherOpen, setPublisherOpen] = useState(false);
  const [publisherSearch, setPublisherSearch] = useState("");
  const debouncedPublisherSearch = useDebounce(publisherSearch, 200);
  const [imprint, setImprint] = useState(file.imprint?.name || "");
  const [imprintOpen, setImprintOpen] = useState(false);
  const [imprintSearch, setImprintSearch] = useState("");
  const debouncedImprintSearch = useDebounce(imprintSearch, 200);
  const [releaseDate, setReleaseDate] = useState(
    formatDateForInput(file.release_date),
  );

  const updateFileMutation = useUpdateFile();
  const uploadCoverMutation = useUploadFileCover();

  // Query for publishers in this library with server-side search
  const { data: publishersData, isLoading: isLoadingPublishers } =
    usePublishersList(
      {
        library_id: file.library_id,
        limit: 50,
        search: debouncedPublisherSearch || undefined,
      },
      { enabled: open },
    );

  // Query for imprints in this library with server-side search
  const { data: imprintsData, isLoading: isLoadingImprints } = useImprintsList(
    {
      library_id: file.library_id,
      limit: 50,
      search: debouncedImprintSearch || undefined,
    },
    { enabled: open },
  );

  // Reset form when dialog opens with new file data
  useEffect(() => {
    if (open) {
      setNarrators(file.narrators?.map((n) => n.person?.name || "") || []);
      setUrl(file.url || "");
      setPublisher(file.publisher?.name || "");
      setPublisherSearch("");
      setImprint(file.imprint?.name || "");
      setImprintSearch("");
      setReleaseDate(formatDateForInput(file.release_date));
    }
  }, [open, file]);

  const handleAddNarrator = () => {
    if (newNarrator.trim() && !narrators.includes(newNarrator.trim())) {
      setNarrators([...narrators, newNarrator.trim()]);
      setNewNarrator("");
    }
  };

  const handleRemoveNarrator = (index: number) => {
    setNarrators(narrators.filter((_, i) => i !== index));
  };

  const handleSelectPublisher = (name: string) => {
    setPublisher(name);
    setPublisherOpen(false);
    setPublisherSearch("");
  };

  const handleCreatePublisher = () => {
    if (publisherSearch.trim()) {
      setPublisher(publisherSearch.trim());
    }
    setPublisherOpen(false);
    setPublisherSearch("");
  };

  const handleClearPublisher = () => {
    setPublisher("");
  };

  const handleSelectImprint = (name: string) => {
    setImprint(name);
    setImprintOpen(false);
    setImprintSearch("");
  };

  const handleCreateImprint = () => {
    if (imprintSearch.trim()) {
      setImprint(imprintSearch.trim());
    }
    setImprintOpen(false);
    setImprintSearch("");
  };

  const handleClearImprint = () => {
    setImprint("");
  };

  // Filter publishers - show all from search, or current selection if set
  const filteredPublishers = useMemo(() => {
    return publishersData?.publishers || [];
  }, [publishersData?.publishers]);

  const showCreatePublisherOption =
    publisherSearch.trim() &&
    !filteredPublishers.find(
      (p) => p.name.toLowerCase() === publisherSearch.toLowerCase(),
    );

  // Filter imprints - show all from search, or current selection if set
  const filteredImprints = useMemo(() => {
    return imprintsData?.imprints || [];
  }, [imprintsData?.imprints]);

  const showCreateImprintOption =
    imprintSearch.trim() &&
    !filteredImprints.find(
      (i) => i.name.toLowerCase() === imprintSearch.toLowerCase(),
    );

  const handleCoverUpload = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const uploadedFile = event.target.files?.[0];
    if (!uploadedFile) return;

    await uploadCoverMutation.mutateAsync({
      id: file.id,
      file: uploadedFile,
    });

    // Update cache buster to force image refresh
    setCoverCacheBuster(Date.now());

    // Reset the file input
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

  const handleSubmit = async () => {
    const payload: {
      narrators?: string[];
      url?: string;
      publisher?: string;
      imprint?: string;
      release_date?: string;
    } = {};

    // Check if narrators changed
    const originalNarrators =
      file.narrators?.map((n) => n.person?.name || "") || [];
    if (JSON.stringify(narrators) !== JSON.stringify(originalNarrators)) {
      payload.narrators = narrators;
    }

    // Check if URL changed
    const originalUrl = file.url || "";
    if (url !== originalUrl) {
      payload.url = url;
    }

    // Check if publisher changed
    const originalPublisher = file.publisher?.name || "";
    if (publisher !== originalPublisher) {
      payload.publisher = publisher;
    }

    // Check if imprint changed
    const originalImprint = file.imprint?.name || "";
    if (imprint !== originalImprint) {
      payload.imprint = imprint;
    }

    // Check if release date changed
    const originalReleaseDate = formatDateForInput(file.release_date);
    if (releaseDate !== originalReleaseDate) {
      payload.release_date = releaseDate || undefined;
    }

    // Only submit if something changed
    if (Object.keys(payload).length > 0) {
      await updateFileMutation.mutateAsync({
        id: file.id,
        payload,
      });
    }

    onOpenChange(false);
  };

  const isLoading =
    updateFileMutation.isPending || uploadCoverMutation.isPending;

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit File</DialogTitle>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* File Info */}
          <div className="space-y-2">
            <Label>File</Label>
            <div className="flex items-center gap-2">
              <Badge className="uppercase text-xs" variant="secondary">
                {file.file_type}
              </Badge>
              <span className="text-sm text-muted-foreground truncate">
                {file.filepath.split("/").pop()}
              </span>
            </div>
          </div>

          {/* Cover Upload (not available for CBZ - cover is page-based) */}
          {file.file_type !== FileTypeCBZ && (
            <div className="space-y-2">
              <Label>Cover Image</Label>
              <div className="w-32 relative group">
                {file.cover_mime_type ? (
                  <img
                    alt="File cover"
                    className="w-full h-auto rounded border border-border"
                    src={`/api/books/files/${file.id}/cover?t=${coverCacheBuster}`}
                  />
                ) : (
                  <div className="w-full aspect-square rounded border border-dashed border-border flex items-center justify-center text-muted-foreground text-xs bg-muted/30">
                    No cover
                  </div>
                )}
                {/* Cover upload overlay */}
                <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity rounded flex items-center justify-center">
                  <input
                    accept="image/jpeg,image/png,image/webp"
                    className="hidden"
                    onChange={handleCoverUpload}
                    ref={fileInputRef}
                    type="file"
                  />
                  <Button
                    disabled={uploadCoverMutation.isPending}
                    onClick={() => fileInputRef.current?.click()}
                    size="sm"
                    variant="secondary"
                  >
                    {uploadCoverMutation.isPending ? (
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    ) : (
                      <Upload className="h-4 w-4 mr-2" />
                    )}
                    {file.cover_mime_type ? "Replace" : "Upload"}
                  </Button>
                </div>
              </div>
            </div>
          )}

          {/* Narrators (only for M4B files) */}
          {file.file_type === "m4b" && (
            <div className="space-y-2">
              <Label>Narrators</Label>
              <div className="flex flex-wrap gap-2 mb-2">
                {narrators.map((narrator, index) => (
                  <Badge
                    className="flex items-center gap-1"
                    key={index}
                    variant="secondary"
                  >
                    {narrator}
                    <button
                      className="ml-1 cursor-pointer hover:text-destructive"
                      onClick={() => handleRemoveNarrator(index)}
                      type="button"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </Badge>
                ))}
              </div>
              <div className="flex gap-2">
                <Input
                  onChange={(e) => setNewNarrator(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      handleAddNarrator();
                    }
                  }}
                  placeholder="Add narrator..."
                  value={newNarrator}
                />
                <Button
                  onClick={handleAddNarrator}
                  type="button"
                  variant="outline"
                >
                  <Plus className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}

          {/* URL */}
          <div className="space-y-2">
            <Label htmlFor="url">URL</Label>
            <Input
              id="url"
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://..."
              type="url"
              value={url}
            />
          </div>

          {/* Publisher */}
          <div className="space-y-2">
            <Label>Publisher</Label>
            {publisher ? (
              <div className="flex items-center gap-2">
                <Badge className="flex items-center gap-1" variant="secondary">
                  {publisher}
                  <button
                    className="ml-1 cursor-pointer hover:text-destructive"
                    onClick={handleClearPublisher}
                    type="button"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </Badge>
              </div>
            ) : (
              <Popover
                modal
                onOpenChange={setPublisherOpen}
                open={publisherOpen}
              >
                <PopoverTrigger asChild>
                  <Button
                    aria-expanded={publisherOpen}
                    className="w-full justify-between"
                    role="combobox"
                    variant="outline"
                  >
                    Select publisher...
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="start" className="w-full p-0">
                  <Command shouldFilter={false}>
                    <CommandInput
                      onValueChange={setPublisherSearch}
                      placeholder="Search publishers..."
                      value={publisherSearch}
                    />
                    <CommandList>
                      {isLoadingPublishers && (
                        <div className="p-4 text-center text-sm text-muted-foreground">
                          Loading publishers...
                        </div>
                      )}
                      {!isLoadingPublishers &&
                        filteredPublishers.length === 0 &&
                        !showCreatePublisherOption && (
                          <div className="p-4 text-center text-sm text-muted-foreground">
                            {!debouncedPublisherSearch
                              ? "No publishers. Type to create one."
                              : "No matching publishers."}
                          </div>
                        )}
                      {!isLoadingPublishers && (
                        <CommandGroup>
                          {filteredPublishers.map((p) => (
                            <CommandItem
                              key={p.id}
                              onSelect={() => handleSelectPublisher(p.name)}
                              value={p.name}
                            >
                              <Check
                                className={`mr-2 h-4 w-4 ${
                                  publisher === p.name
                                    ? "opacity-100"
                                    : "opacity-0"
                                }`}
                              />
                              {p.name}
                            </CommandItem>
                          ))}
                          {showCreatePublisherOption && (
                            <CommandItem
                              onSelect={handleCreatePublisher}
                              value={`create-${publisherSearch}`}
                            >
                              <Plus className="mr-2 h-4 w-4" />
                              Create "{publisherSearch}"
                            </CommandItem>
                          )}
                        </CommandGroup>
                      )}
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
            )}
          </div>

          {/* Imprint */}
          <div className="space-y-2">
            <Label>Imprint</Label>
            {imprint ? (
              <div className="flex items-center gap-2">
                <Badge className="flex items-center gap-1" variant="secondary">
                  {imprint}
                  <button
                    className="ml-1 cursor-pointer hover:text-destructive"
                    onClick={handleClearImprint}
                    type="button"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </Badge>
              </div>
            ) : (
              <Popover modal onOpenChange={setImprintOpen} open={imprintOpen}>
                <PopoverTrigger asChild>
                  <Button
                    aria-expanded={imprintOpen}
                    className="w-full justify-between"
                    role="combobox"
                    variant="outline"
                  >
                    Select imprint...
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="start" className="w-full p-0">
                  <Command shouldFilter={false}>
                    <CommandInput
                      onValueChange={setImprintSearch}
                      placeholder="Search imprints..."
                      value={imprintSearch}
                    />
                    <CommandList>
                      {isLoadingImprints && (
                        <div className="p-4 text-center text-sm text-muted-foreground">
                          Loading imprints...
                        </div>
                      )}
                      {!isLoadingImprints &&
                        filteredImprints.length === 0 &&
                        !showCreateImprintOption && (
                          <div className="p-4 text-center text-sm text-muted-foreground">
                            {!debouncedImprintSearch
                              ? "No imprints. Type to create one."
                              : "No matching imprints."}
                          </div>
                        )}
                      {!isLoadingImprints && (
                        <CommandGroup>
                          {filteredImprints.map((i) => (
                            <CommandItem
                              key={i.id}
                              onSelect={() => handleSelectImprint(i.name)}
                              value={i.name}
                            >
                              <Check
                                className={`mr-2 h-4 w-4 ${
                                  imprint === i.name
                                    ? "opacity-100"
                                    : "opacity-0"
                                }`}
                              />
                              {i.name}
                            </CommandItem>
                          ))}
                          {showCreateImprintOption && (
                            <CommandItem
                              onSelect={handleCreateImprint}
                              value={`create-${imprintSearch}`}
                            >
                              <Plus className="mr-2 h-4 w-4" />
                              Create "{imprintSearch}"
                            </CommandItem>
                          )}
                        </CommandGroup>
                      )}
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
            )}
          </div>

          {/* Release Date */}
          <div className="space-y-2">
            <Label htmlFor="releaseDate">Release Date</Label>
            <Input
              id="releaseDate"
              onChange={(e) => setReleaseDate(e.target.value)}
              type="date"
              value={releaseDate}
            />
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button disabled={isLoading} onClick={handleSubmit}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
