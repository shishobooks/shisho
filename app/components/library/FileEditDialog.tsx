import equal from "fast-deep-equal";
import {
  Check,
  ChevronsUpDown,
  GripVertical,
  Image,
  Loader2,
  Plus,
  Upload,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import CBZPagePicker from "@/components/files/CBZPagePicker";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { DatePicker } from "@/components/ui/date-picker";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
import {
  SortableList,
  type DragHandleProps,
} from "@/components/ui/SortableList";
import {
  useSetFileCoverPage,
  useUpdateFile,
  useUploadFileCover,
} from "@/hooks/queries/books";
import { useImprintsList } from "@/hooks/queries/imprints";
import { usePeopleList } from "@/hooks/queries/people";
import { usePluginIdentifierTypes } from "@/hooks/queries/plugins";
import { usePublishersList } from "@/hooks/queries/publishers";
import { useDebounce } from "@/hooks/useDebounce";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { cn } from "@/libraries/utils";
import {
  FileRoleMain,
  FileRoleSupplement,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeM4B,
  type File,
  type FileRole,
} from "@/types";
import { formatIdentifierType } from "@/utils/format";
import { validateIdentifier } from "@/utils/identifiers";

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
  const [narratorSearch, setNarratorSearch] = useState("");
  const debouncedNarratorSearch = useDebounce(narratorSearch, 200);
  const [narratorOpen, setNarratorOpen] = useState(false);
  const [coverCacheBuster, setCoverCacheBuster] = useState(() => Date.now());
  const [coverPagePickerOpen, setCoverPagePickerOpen] = useState(false);
  const [pendingCoverPage, setPendingCoverPage] = useState<number | null>(null);
  const [pendingCoverFile, setPendingCoverFile] =
    useState<globalThis.File | null>(null);
  const [pendingCoverPreview, setPendingCoverPreview] = useState<string | null>(
    null,
  );
  const pendingCoverPreviewRef = useRef<string | null>(null);

  // Identifier state
  const [identifiers, setIdentifiers] = useState<
    Array<{ type: string; value: string }>
  >(file.identifiers?.map((id) => ({ type: id.type, value: id.value })) || []);
  const [newIdentifierType, setNewIdentifierType] = useState<string>("isbn_13");
  const [newIdentifierValue, setNewIdentifierValue] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);

  // New file metadata fields
  const [name, setName] = useState(file.name || "");
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
  const [fileRole, setFileRole] = useState(file.file_role ?? FileRoleMain);
  const [showDowngradeConfirm, setShowDowngradeConfirm] = useState(false);

  const updateFileMutation = useUpdateFile();
  const uploadCoverMutation = useUploadFileCover();
  const setCoverPageMutation = useSetFileCoverPage();

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

  // Query for people in this library with server-side search (for narrators)
  const { data: peopleData, isLoading: isLoadingPeople } = usePeopleList(
    {
      library_id: file.library_id,
      limit: 50,
      search: debouncedNarratorSearch || undefined,
    },
    { enabled: open },
  );

  // Query for plugin-defined identifier types
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes();

  // Helper to set preview URL and handle cleanup of old URL
  const updatePendingCoverPreview = useCallback((url: string | null) => {
    if (pendingCoverPreviewRef.current) {
      URL.revokeObjectURL(pendingCoverPreviewRef.current);
    }
    pendingCoverPreviewRef.current = url;
    setPendingCoverPreview(url);
  }, []);

  // Cleanup pending cover preview URL on unmount
  useEffect(() => {
    return () => {
      if (pendingCoverPreviewRef.current) {
        URL.revokeObjectURL(pendingCoverPreviewRef.current);
      }
    };
  }, []);

  // Store initial values for change detection
  const [initialValues, setInitialValues] = useState<{
    narrators: string[];
    name: string;
    url: string;
    publisher: string;
    imprint: string;
    releaseDate: string;
    identifiers: Array<{ type: string; value: string }>;
    fileRole: string;
    coverPage: number | null;
  } | null>(null);

  // Track previous open state to detect open transitions.
  // Start with false so that if dialog starts open, we detect it as "just opened".
  const prevOpenRef = useRef(false);

  // Initialize form only when dialog opens (closed->open transition)
  // This preserves user edits when props change while dialog is open
  // Also cleanup blob URLs when dialog closes to prevent memory leaks
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    const justClosed = !open && prevOpenRef.current;
    prevOpenRef.current = open;

    // Cleanup blob URL when dialog closes to prevent memory leak
    if (justClosed) {
      updatePendingCoverPreview(null);
      return;
    }

    // Only initialize when dialog just opened, not on every prop change
    if (!justOpened) return;

    const initialNarrators =
      file.narrators?.map((n) => n.person?.name || "") || [];
    const initialName = file.name || "";
    const initialUrl = file.url || "";
    const initialPublisher = file.publisher?.name || "";
    const initialImprint = file.imprint?.name || "";
    const initialReleaseDate = formatDateForInput(file.release_date);
    const initialIdentifiers =
      file.identifiers?.map((id) => ({ type: id.type, value: id.value })) || [];
    const initialFileRole = file.file_role ?? FileRoleMain;

    setNarrators(initialNarrators);
    setNarratorSearch("");
    setName(initialName);
    setUrl(initialUrl);
    setPublisher(initialPublisher);
    setPublisherSearch("");
    setImprint(initialImprint);
    setImprintSearch("");
    setReleaseDate(initialReleaseDate);
    setIdentifiers(initialIdentifiers);
    setNewIdentifierType("isbn_13");
    setNewIdentifierValue("");
    setFileRole(initialFileRole);
    setShowDowngradeConfirm(false);
    setPendingCoverPage(null);
    setPendingCoverFile(null);
    updatePendingCoverPreview(null);

    // Store initial values for comparison
    setInitialValues({
      narrators: initialNarrators,
      name: initialName,
      url: initialUrl,
      publisher: initialPublisher,
      imprint: initialImprint,
      releaseDate: initialReleaseDate,
      identifiers: initialIdentifiers,
      fileRole: initialFileRole,
      coverPage: file.cover_page ?? null,
    });
  }, [open, file, updatePendingCoverPreview]);

  // Compute hasChanges by comparing current values to initial values
  const hasChanges = useMemo(() => {
    if (!initialValues) return false;
    return (
      !equal(narrators, initialValues.narrators) ||
      name !== initialValues.name ||
      url !== initialValues.url ||
      publisher !== initialValues.publisher ||
      imprint !== initialValues.imprint ||
      releaseDate !== initialValues.releaseDate ||
      !equal(identifiers, initialValues.identifiers) ||
      fileRole !== initialValues.fileRole ||
      pendingCoverFile !== null ||
      (pendingCoverPage !== null &&
        pendingCoverPage !== initialValues.coverPage)
    );
  }, [
    narrators,
    name,
    url,
    publisher,
    imprint,
    releaseDate,
    identifiers,
    fileRole,
    pendingCoverFile,
    pendingCoverPage,
    initialValues,
  ]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

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

  // Filter out already-selected narrators from the people list
  const filteredPeople = useMemo(() => {
    return peopleData?.people.filter((p) => !narrators.includes(p.name)) || [];
  }, [peopleData?.people, narrators]);

  // Show "create" option if search doesn't match existing people or already-selected narrators
  const showCreateNarratorOption = useMemo(() => {
    if (!narratorSearch.trim()) return false;
    const searchLower = narratorSearch.trim().toLowerCase();
    const matchesPeople = peopleData?.people.some(
      (p) => p.name.toLowerCase() === searchLower,
    );
    const matchesSelected = narrators.some(
      (n) => n.toLowerCase() === searchLower,
    );
    return !matchesPeople && !matchesSelected;
  }, [narratorSearch, peopleData?.people, narrators]);

  const handleSelectNarrator = (name: string) => {
    if (!narrators.includes(name)) {
      setNarrators([...narrators, name]);
    }
    setNarratorOpen(false);
    setNarratorSearch("");
  };

  const handleCreateNarrator = () => {
    if (narratorSearch.trim() && !narrators.includes(narratorSearch.trim())) {
      setNarrators([...narrators, narratorSearch.trim()]);
    }
    setNarratorOpen(false);
    setNarratorSearch("");
  };

  const handleRemoveNarrator = (index: number) => {
    setNarrators(narrators.filter((_, i) => i !== index));
  };

  const handleAddIdentifier = () => {
    if (!newIdentifierValue.trim()) return;

    const pluginType = pluginIdentifierTypes?.find(
      (pt) => pt.id === newIdentifierType,
    );
    const validation = validateIdentifier(
      newIdentifierType,
      newIdentifierValue.trim(),
      pluginType?.pattern ?? undefined,
    );
    if (!validation.valid) {
      toast.error(validation.error);
      return;
    }

    setIdentifiers([
      ...identifiers,
      { type: newIdentifierType, value: newIdentifierValue.trim() },
    ]);
    setNewIdentifierValue("");
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

  const handleCoverUpload = (event: React.ChangeEvent<HTMLInputElement>) => {
    const uploadedFile = event.target.files?.[0];
    if (!uploadedFile) return;

    // Store the file for upload on save
    setPendingCoverFile(uploadedFile);

    // Create preview URL (helper handles cleanup of old URL)
    updatePendingCoverPreview(URL.createObjectURL(uploadedFile));

    // Reset the file input
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

  const handleCoverPageSelect = (page: number) => {
    // Store the selected page for save
    setPendingCoverPage(page);
    setCoverPagePickerOpen(false);
  };

  const handleSubmit = async () => {
    const payload: {
      file_role?: string;
      name?: string;
      narrators?: string[];
      url?: string;
      publisher?: string;
      imprint?: string;
      release_date?: string;
      identifiers?: Array<{ type: string; value: string }>;
    } = {};

    // Check if file role changed
    if (fileRole !== (file.file_role ?? FileRoleMain)) {
      // If downgrading to supplement, require confirmation
      if (fileRole === FileRoleSupplement && !showDowngradeConfirm) {
        setShowDowngradeConfirm(true);
        return;
      }
      payload.file_role = fileRole;
    }

    // Check if name changed
    const originalName = file.name || "";
    if (name !== originalName) {
      // When cleared, set to null; otherwise use the trimmed value
      payload.name = name.trim() || undefined;
    }

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
      payload.release_date = releaseDate;
    }

    // Check if identifiers changed
    const originalIdentifiers =
      file.identifiers?.map((id) => ({ type: id.type, value: id.value })) || [];
    if (JSON.stringify(identifiers) !== JSON.stringify(originalIdentifiers)) {
      payload.identifiers = identifiers;
    }

    // Only submit if something changed
    if (Object.keys(payload).length > 0) {
      await updateFileMutation.mutateAsync({
        id: file.id,
        payload,
      });
    }

    // Apply pending cover changes
    if (pendingCoverFile) {
      await uploadCoverMutation.mutateAsync({
        id: file.id,
        file: pendingCoverFile,
      });
      setCoverCacheBuster(Date.now());
      setPendingCoverFile(null);
    }

    // Compare to initialValues.coverPage (snapshot) instead of file.cover_page (live prop)
    // to stay consistent with hasChanges logic and avoid race conditions with refetches
    if (
      pendingCoverPage !== null &&
      pendingCoverPage !== initialValues?.coverPage
    ) {
      await setCoverPageMutation.mutateAsync({
        id: file.id,
        page: pendingCoverPage,
      });
      setCoverCacheBuster(Date.now());
      setPendingCoverPage(null);
    }

    // Reset initial values so hasChanges becomes false, then close via effect
    // For coverPage, use pendingCoverPage if set, otherwise keep the current initial value
    setInitialValues({
      narrators: [...narrators],
      name,
      url,
      publisher,
      imprint,
      releaseDate,
      identifiers: [...identifiers],
      fileRole,
      coverPage: pendingCoverPage ?? initialValues?.coverPage ?? null,
    });
    requestClose();
  };

  const isLoading =
    updateFileMutation.isPending ||
    uploadCoverMutation.isPending ||
    setCoverPageMutation.isPending;

  const isSupplement = file.file_role === FileRoleSupplement;

  // Check if file type can be a main file (only cbz, epub, m4b are supported)
  const canBeMainFile = [FileTypeCBZ, FileTypeEPUB, FileTypeM4B].includes(
    file.file_type as typeof FileTypeCBZ,
  );

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-xl max-h-[90vh] overflow-y-auto overflow-x-hidden">
        <DialogHeader>
          <DialogTitle>Edit File</DialogTitle>
          <DialogDescription>
            Update file metadata including cover, identifiers, and publishing
            details.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4 min-w-0">
          {/* File Info */}
          <div className="space-y-2">
            <Label>File</Label>
            <div className="flex items-center gap-2 min-w-0">
              <Badge className="uppercase text-xs shrink-0" variant="secondary">
                {file.file_type}
              </Badge>
              <span
                className="text-sm text-muted-foreground truncate"
                title={file.filepath.split("/").pop()}
              >
                {file.filepath.split("/").pop()}
              </span>
            </div>
          </div>

          {/* Name */}
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              onChange={(e) => setName(e.target.value)}
              value={name}
            />
          </div>

          {/* File Role */}
          <div className="space-y-2">
            <Label>File Role</Label>
            <Select
              onValueChange={(v) => setFileRole(v as FileRole)}
              value={fileRole}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  disabled={isSupplement && !canBeMainFile}
                  value={FileRoleMain}
                >
                  Main File
                </SelectItem>
                <SelectItem value={FileRoleSupplement}>Supplement</SelectItem>
              </SelectContent>
            </Select>
            {showDowngradeConfirm && (
              <p className="text-sm text-destructive">
                Changing to supplement will clear all metadata (narrators,
                identifiers, publisher, etc.). Click Save again to confirm.
              </p>
            )}
            {isSupplement && !canBeMainFile && (
              <p className="text-sm text-muted-foreground">
                This file type ({file.file_type}) cannot be upgraded to a main
                file. Only cbz, epub, and m4b files can be main files.
              </p>
            )}
          </div>

          {/* Main file only sections */}
          {!isSupplement && fileRole !== FileRoleSupplement && (
            <>
              {/* Cover Image Section - Unified for all file types */}
              <div className="space-y-3">
                <Label>Cover Image</Label>
                <div className="flex items-start gap-4">
                  {/* Cover thumbnail */}
                  <div className="relative group w-28 shrink-0">
                    <div
                      className={cn(
                        "rounded-lg overflow-hidden border border-border bg-muted",
                        file.file_type === "m4b"
                          ? "aspect-square"
                          : "aspect-[2/3]",
                      )}
                    >
                      {/* Non-CBZ: Show pending preview or current cover */}
                      {file.file_type !== FileTypeCBZ && (
                        <>
                          {pendingCoverPreview ? (
                            <img
                              alt="Pending cover"
                              className="w-full h-full object-cover"
                              src={pendingCoverPreview}
                            />
                          ) : file.cover_mime_type ||
                            file.cover_image_filename ? (
                            <img
                              alt="File cover"
                              className="w-full h-full object-cover"
                              src={`/api/books/files/${file.id}/cover?t=${coverCacheBuster}`}
                            />
                          ) : (
                            <CoverPlaceholder
                              className="w-full h-full"
                              variant={
                                file.file_type === "m4b" ? "audiobook" : "book"
                              }
                            />
                          )}
                        </>
                      )}
                      {/* CBZ: Show pending page or current cover */}
                      {file.file_type === FileTypeCBZ && (
                        <>
                          {pendingCoverPage !== null &&
                          pendingCoverPage !== file.cover_page ? (
                            <img
                              alt="Pending cover page"
                              className="w-full h-full object-cover"
                              src={`/api/books/files/${file.id}/page/${pendingCoverPage}`}
                            />
                          ) : file.cover_mime_type ||
                            file.cover_image_filename ? (
                            <img
                              alt="File cover"
                              className="w-full h-full object-cover"
                              src={`/api/books/files/${file.id}/cover?t=${coverCacheBuster}`}
                            />
                          ) : (
                            <CoverPlaceholder
                              className="w-full h-full"
                              variant="book"
                            />
                          )}
                        </>
                      )}
                    </div>
                    {/* Page number badge for CBZ */}
                    {file.file_type === FileTypeCBZ &&
                      (pendingCoverPage ?? file.cover_page) != null && (
                        <div className="absolute bottom-1.5 left-1.5 px-1.5 py-0.5 rounded bg-black/70 text-white text-xs font-medium">
                          Page {(pendingCoverPage ?? file.cover_page)! + 1}
                        </div>
                      )}
                  </div>

                  {/* Action buttons and status */}
                  <div className="flex flex-col gap-2 pt-1">
                    {/* Non-CBZ: Upload button */}
                    {file.file_type !== FileTypeCBZ && (
                      <>
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
                          type="button"
                          variant="outline"
                        >
                          {uploadCoverMutation.isPending ? (
                            <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                          ) : (
                            <Upload className="h-4 w-4 mr-2" />
                          )}
                          {file.cover_mime_type ||
                          file.cover_image_filename ||
                          pendingCoverFile
                            ? "Replace cover"
                            : "Upload cover"}
                        </Button>
                      </>
                    )}
                    {/* CBZ: Select page button */}
                    {file.file_type === FileTypeCBZ && (
                      <Button
                        disabled={setCoverPageMutation.isPending}
                        onClick={() => setCoverPagePickerOpen(true)}
                        size="sm"
                        type="button"
                        variant="outline"
                      >
                        {setCoverPageMutation.isPending ? (
                          <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                        ) : (
                          <Image className="h-4 w-4 mr-2" />
                        )}
                        Select page
                      </Button>
                    )}
                    {/* Unsaved indicator */}
                    {((file.file_type !== FileTypeCBZ && pendingCoverFile) ||
                      (file.file_type === FileTypeCBZ &&
                        pendingCoverPage !== null &&
                        pendingCoverPage !== file.cover_page)) && (
                      <span className="text-xs text-orange-500 font-medium">
                        Unsaved changes
                      </span>
                    )}
                  </div>
                </div>
              </div>

              {/* CBZ Page Picker Dialog */}
              {file.file_type === FileTypeCBZ && file.page_count != null && (
                <CBZPagePicker
                  currentPage={pendingCoverPage ?? file.cover_page ?? null}
                  fileId={file.id}
                  onOpenChange={setCoverPagePickerOpen}
                  onSelect={handleCoverPageSelect}
                  open={coverPagePickerOpen}
                  pageCount={file.page_count}
                  title="Select Cover Page"
                />
              )}

              {/* Narrators (only for M4B files) */}
              {file.file_type === "m4b" && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label>Narrators</Label>
                    {narrators.length > 1 && (
                      <button
                        className="text-xs text-muted-foreground hover:text-destructive cursor-pointer"
                        onClick={() => setNarrators([])}
                        type="button"
                      >
                        Clear all
                      </button>
                    )}
                  </div>
                  <SortableList
                    getItemId={(name, index) => `${name}-${index}`}
                    items={narrators}
                    onReorder={setNarrators}
                    renderItem={(
                      name: string,
                      index: number,
                      dragHandleProps: DragHandleProps,
                    ) => (
                      <div className="flex items-center gap-2">
                        <button
                          className="cursor-grab touch-none text-muted-foreground hover:text-foreground"
                          type="button"
                          {...dragHandleProps.attributes}
                          {...dragHandleProps.listeners}
                        >
                          <GripVertical className="h-4 w-4" />
                        </button>
                        <div className="flex-1">
                          <Input disabled value={name} />
                        </div>
                        <Button
                          onClick={() => handleRemoveNarrator(index)}
                          size="icon"
                          type="button"
                          variant="ghost"
                        >
                          <X className="h-4 w-4" />
                        </Button>
                      </div>
                    )}
                  />
                  {/* Narrator Combobox */}
                  <Popover
                    modal
                    onOpenChange={setNarratorOpen}
                    open={narratorOpen}
                  >
                    <PopoverTrigger asChild>
                      <Button
                        aria-expanded={narratorOpen}
                        className="w-full justify-between"
                        role="combobox"
                        variant="outline"
                      >
                        Add narrator...
                        <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent align="start" className="w-full p-0">
                      <Command shouldFilter={false}>
                        <CommandInput
                          onValueChange={setNarratorSearch}
                          placeholder="Search people..."
                          value={narratorSearch}
                        />
                        <CommandList>
                          {isLoadingPeople && (
                            <div className="p-4 text-center text-sm text-muted-foreground">
                              Loading people...
                            </div>
                          )}
                          {!isLoadingPeople &&
                            filteredPeople.length === 0 &&
                            !showCreateNarratorOption && (
                              <div className="p-4 text-center text-sm text-muted-foreground">
                                {!debouncedNarratorSearch
                                  ? "No people in this library. Type to create one."
                                  : "No matching people."}
                              </div>
                            )}
                          {!isLoadingPeople && (
                            <CommandGroup>
                              {filteredPeople.map((p) => (
                                <CommandItem
                                  key={p.id}
                                  onSelect={() => handleSelectNarrator(p.name)}
                                  value={p.name}
                                >
                                  <Check className="mr-2 h-4 w-4 opacity-0 shrink-0" />
                                  <span className="truncate" title={p.name}>
                                    {p.name}
                                  </span>
                                </CommandItem>
                              ))}
                              {showCreateNarratorOption && (
                                <CommandItem
                                  onSelect={handleCreateNarrator}
                                  value={`create-${narratorSearch}`}
                                >
                                  <Plus className="mr-2 h-4 w-4 shrink-0" />
                                  <span className="truncate">
                                    Create "{narratorSearch}"
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
                    <Badge
                      className="flex items-center gap-1 max-w-full"
                      variant="secondary"
                    >
                      <span className="truncate" title={publisher}>
                        {publisher}
                      </span>
                      <button
                        className="ml-1 cursor-pointer hover:text-destructive shrink-0"
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
                                    className={`mr-2 h-4 w-4 shrink-0 ${
                                      publisher === p.name
                                        ? "opacity-100"
                                        : "opacity-0"
                                    }`}
                                  />
                                  <span className="truncate" title={p.name}>
                                    {p.name}
                                  </span>
                                </CommandItem>
                              ))}
                              {showCreatePublisherOption && (
                                <CommandItem
                                  onSelect={handleCreatePublisher}
                                  value={`create-${publisherSearch}`}
                                >
                                  <Plus className="mr-2 h-4 w-4 shrink-0" />
                                  <span className="truncate">
                                    Create "{publisherSearch}"
                                  </span>
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
                    <Badge
                      className="flex items-center gap-1 max-w-full"
                      variant="secondary"
                    >
                      <span className="truncate" title={imprint}>
                        {imprint}
                      </span>
                      <button
                        className="ml-1 cursor-pointer hover:text-destructive shrink-0"
                        onClick={handleClearImprint}
                        type="button"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </Badge>
                  </div>
                ) : (
                  <Popover
                    modal
                    onOpenChange={setImprintOpen}
                    open={imprintOpen}
                  >
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
                                    className={`mr-2 h-4 w-4 shrink-0 ${
                                      imprint === i.name
                                        ? "opacity-100"
                                        : "opacity-0"
                                    }`}
                                  />
                                  <span className="truncate" title={i.name}>
                                    {i.name}
                                  </span>
                                </CommandItem>
                              ))}
                              {showCreateImprintOption && (
                                <CommandItem
                                  onSelect={handleCreateImprint}
                                  value={`create-${imprintSearch}`}
                                >
                                  <Plus className="mr-2 h-4 w-4 shrink-0" />
                                  <span className="truncate">
                                    Create "{imprintSearch}"
                                  </span>
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
                <Label>Release Date</Label>
                <DatePicker
                  onChange={setReleaseDate}
                  placeholder="Pick a date"
                  value={releaseDate}
                />
              </div>

              {/* Identifiers */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>Identifiers</Label>
                  {identifiers.length > 1 && (
                    <button
                      className="text-xs text-muted-foreground hover:text-destructive cursor-pointer"
                      onClick={() => setIdentifiers([])}
                      type="button"
                    >
                      Clear all
                    </button>
                  )}
                </div>
                <div className="flex flex-wrap gap-2 mb-2">
                  {identifiers.map((id, idx) => (
                    <Badge
                      className="flex items-center gap-1 max-w-full"
                      key={idx}
                      variant="secondary"
                    >
                      <span className="text-xs">
                        {formatIdentifierType(id.type, pluginIdentifierTypes)}
                      </span>
                      : {id.value}
                      <button
                        className="ml-1 cursor-pointer hover:text-destructive shrink-0"
                        onClick={() => {
                          setIdentifiers(
                            identifiers.filter((_, i) => i !== idx),
                          );
                        }}
                        type="button"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </Badge>
                  ))}
                </div>
                <div className="flex gap-2">
                  <Select
                    onValueChange={setNewIdentifierType}
                    value={newIdentifierType}
                  >
                    <SelectTrigger className="w-32">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="isbn_10">ISBN-10</SelectItem>
                      <SelectItem value="isbn_13">ISBN-13</SelectItem>
                      <SelectItem value="asin">ASIN</SelectItem>
                      <SelectItem value="uuid">UUID</SelectItem>
                      <SelectItem value="goodreads">Goodreads</SelectItem>
                      <SelectItem value="google">Google</SelectItem>
                      <SelectItem value="other">Other</SelectItem>
                      {pluginIdentifierTypes
                        ?.filter(
                          (pt) =>
                            ![
                              "isbn_10",
                              "isbn_13",
                              "asin",
                              "uuid",
                              "goodreads",
                              "google",
                              "other",
                            ].includes(pt.id),
                        )
                        .map((pt) => (
                          <SelectItem key={pt.id} value={pt.id}>
                            {pt.name}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                  <Input
                    className="flex-1"
                    onChange={(e) => setNewIdentifierValue(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        handleAddIdentifier();
                      }
                    }}
                    placeholder="Enter value..."
                    value={newIdentifierValue}
                  />
                  <Button
                    onClick={handleAddIdentifier}
                    type="button"
                    variant="outline"
                  >
                    Add
                  </Button>
                </div>
              </div>
            </>
          )}
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
    </FormDialog>
  );
}
