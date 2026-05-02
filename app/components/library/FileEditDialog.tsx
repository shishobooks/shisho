import equal from "fast-deep-equal";
import { Image, Loader2, Upload, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { EntityCombobox } from "@/components/common/EntityCombobox";
import { IdentifierEditor } from "@/components/common/IdentifierEditor";
import { SortableEntityList } from "@/components/common/SortableEntityList";
import PagePicker from "@/components/files/PagePicker";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { LanguageCombobox } from "@/components/library/LanguageCombobox";
import { ReviewPanel } from "@/components/library/ReviewPanel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useSetFileCoverPage,
  useUpdateFile,
  useUploadFileCover,
} from "@/hooks/queries/books";
import {
  useImprintSearch,
  usePeopleSearch,
  usePublisherSearch,
  type NameOption,
} from "@/hooks/queries/entity-search";
import { usePluginIdentifierTypes } from "@/hooks/queries/plugins";
import { useSetFileReview } from "@/hooks/queries/review";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { cn, isPageBasedFileType } from "@/libraries/utils";
import {
  FileRoleMain,
  FileRoleSupplement,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeM4B,
  ReviewOverrideReviewed,
  type Book,
  type File,
  type FileRole,
  type ReviewOverride,
} from "@/types";

interface FileEditDialogProps {
  file: File;
  /** Parent book — used by ReviewPanel for book-level field checks */
  book?: Book;
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
  book,
  open,
  onOpenChange,
}: FileEditDialogProps) {
  const [narrators, setNarrators] = useState<string[]>(
    file.narrators?.map((n) => n.person?.name || "") || [],
  );
  // Memoize the {name}-wrapped items so SortableEntityList sees stable
  // references across renders. Without this, the WeakMap-keyed row id
  // tracker (in SortableEntityList) would assign a new key every render
  // and confuse dnd-kit's drag tracking.
  const narratorItems = useMemo(
    () => narrators.map((n) => ({ name: n })),
    [narrators],
  );
  const isPageBased = isPageBasedFileType(file.file_type);
  // Dialog-local cover cache key — bumped synchronously after cover mutations
  // (upload / set-cover-page) so the preview `<img>` refreshes immediately on
  // save, without waiting for the parent query to refetch. Elsewhere in the
  // codebase we use `query.dataUpdatedAt` for this; intentional divergence.
  const [coverCacheKey, setCoverCacheKey] = useState(() => Date.now());
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
  const fileInputRef = useRef<HTMLInputElement>(null);

  // New file metadata fields
  const [name, setName] = useState(file.name || "");
  const [url, setUrl] = useState(file.url || "");
  const [publisher, setPublisher] = useState(file.publisher?.name || "");
  const [imprint, setImprint] = useState(file.imprint?.name || "");
  const [releaseDate, setReleaseDate] = useState(
    formatDateForInput(file.release_date),
  );
  const [fileRole, setFileRole] = useState(file.file_role ?? FileRoleMain);
  const [showDowngradeConfirm, setShowDowngradeConfirm] = useState(false);
  const [language, setLanguage] = useState(file.language || "");
  // Abridged is intentionally a binary checkbox in the UI, not a three-state
  // control, even though the data model supports true/false/null. Rationale:
  // most books are unabridged, so showing an "Unknown" default on every book
  // would be noisy and users would feel pressure to set it. Marking something
  // as explicitly unabridged through the UI is not exposed — parsers and
  // plugins can still set `false` at the data layer, and those values survive
  // round-trips, but UI-driven edits are opt-in to `true` only.
  //
  // Both `false` and `null` are normalized to the empty "clear" state at
  // init so that accidentally toggling the checkbox off and on doesn't clobber
  // a plugin's explicit `false` value. The backend only receives an update
  // when the user actively checks the box (to set it to `true`) or unchecks
  // after checking (which sends `""` to clear to `null`).
  //
  // If you're tempted to "fix" this by adding a three-state dropdown, please
  // read the design spec at docs/superpowers/specs/2026-04-07-language-
  // abridged-fields-design.md first — this was a deliberate UX simplification.
  const [abridged, setAbridged] = useState<string>(
    file.abridged === true ? "true" : "",
  );

  const updateFileMutation = useUpdateFile();
  const uploadCoverMutation = useUploadFileCover();
  const setCoverPageMutation = useSetFileCoverPage();
  const setFileReviewMutation = useSetFileReview();

  // Query for plugin-defined identifier types
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes();

  // All identifier types available for selection, in display order.
  // Memoized so it can be a stable useEffect dependency without causing
  // an infinite update loop. Plugin-defined types include their pattern so
  // IdentifierEditor can use it for validation.
  const availableIdentifierTypes = useMemo(
    () => [
      { id: "isbn_10", label: "ISBN-10" },
      { id: "isbn_13", label: "ISBN-13" },
      { id: "asin", label: "ASIN" },
      { id: "uuid", label: "UUID" },
      { id: "goodreads", label: "Goodreads" },
      { id: "google", label: "Google" },
      { id: "other", label: "Other" },
      ...(pluginIdentifierTypes
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
        .map((pt) => ({ id: pt.id, label: pt.name, pattern: pt.pattern })) ??
        []),
    ],
    [pluginIdentifierTypes],
  );

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

  // Draft review override — toggling the panel updates this; the actual
  // setFileReview mutation only fires on Save.
  // null means "auto" (no explicit override on the file).
  const [draftReviewOverride, setDraftReviewOverride] =
    useState<ReviewOverride | null>(null);

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
    language: string;
    abridged: string;
    /** null means "auto" — no explicit override on the file. */
    reviewOverride: ReviewOverride | null;
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
    const initialLanguage = file.language || "";
    // Normalize both `false` and `null` to "" — see the useState init above
    // for the reasoning.
    const initialAbridged = file.abridged === true ? "true" : "";

    // Capture the actual override (not the aggregate `reviewed`) so the user
    // can toggle a currently-auto-reviewed file and have that intent persist.
    // null = "auto" (no explicit override).
    const initialReviewOverride: ReviewOverride | null =
      file.review_override ?? null;

    setNarrators(initialNarrators);
    setName(initialName);
    setUrl(initialUrl);
    setPublisher(initialPublisher);
    setImprint(initialImprint);
    setReleaseDate(initialReleaseDate);
    setIdentifiers(initialIdentifiers);
    setFileRole(initialFileRole);
    setShowDowngradeConfirm(false);
    setLanguage(initialLanguage);
    setAbridged(initialAbridged);
    setPendingCoverPage(null);
    setPendingCoverFile(null);
    updatePendingCoverPreview(null);
    setDraftReviewOverride(initialReviewOverride);

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
      language: initialLanguage,
      abridged: initialAbridged,
      reviewOverride: initialReviewOverride,
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
      language !== initialValues.language ||
      abridged !== initialValues.abridged ||
      pendingCoverFile !== null ||
      (pendingCoverPage !== null &&
        pendingCoverPage !== initialValues.coverPage) ||
      draftReviewOverride !== initialValues.reviewOverride
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
    language,
    abridged,
    pendingCoverFile,
    pendingCoverPage,
    draftReviewOverride,
    initialValues,
  ]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

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
      language?: string;
      abridged?: string;
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

    // Check if language changed
    if (language !== initialValues?.language) {
      payload.language = language;
    }

    // Check if abridged changed
    if (abridged !== initialValues?.abridged) {
      payload.abridged = abridged;
    }

    // Check if identifiers changed
    const originalIdentifiers =
      file.identifiers?.map((id) => ({ type: id.type, value: id.value })) || [];
    if (JSON.stringify(identifiers) !== JSON.stringify(originalIdentifiers)) {
      payload.identifiers = identifiers;
    }

    // Only submit if something changed
    let updatedFile: File | undefined;
    if (Object.keys(payload).length > 0) {
      updatedFile = await updateFileMutation.mutateAsync({
        id: file.id,
        payload,
      });

      // The backend canonicalizes the language tag (e.g. "en-us" → "en-US")
      // and the abridged value. Sync local state with the server's response
      // so the dialog reflects the canonical form without needing a reopen.
      if (updatedFile) {
        const canonicalLanguage = updatedFile.language || "";
        if (canonicalLanguage !== language) {
          setLanguage(canonicalLanguage);
        }
        const canonicalAbridged = updatedFile.abridged === true ? "true" : "";
        if (canonicalAbridged !== abridged) {
          setAbridged(canonicalAbridged);
        }
      }
    }

    // Apply pending cover changes
    if (pendingCoverFile) {
      await uploadCoverMutation.mutateAsync({
        id: file.id,
        file: pendingCoverFile,
      });
      setCoverCacheKey(Date.now());
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
      setCoverCacheKey(Date.now());
      setPendingCoverPage(null);
    }

    // Apply pending review override change. Only fire if the user toggled
    // to an explicit value that differs from the file's saved override.
    // draftReviewOverride === null means "auto" — never set by the user
    // gesture, only by initial load when no override exists.
    if (
      draftReviewOverride !== null &&
      draftReviewOverride !== (initialValues?.reviewOverride ?? null)
    ) {
      await setFileReviewMutation.mutateAsync({
        fileId: file.id,
        override: draftReviewOverride,
      });
    }

    // Reset initial values so hasChanges becomes false, then close via effect.
    // For coverPage, use pendingCoverPage if set, otherwise keep the current
    // initial value. For language/abridged, use the canonicalized values
    // from the server response when available so hasChanges correctly
    // reflects what's actually persisted.
    const canonicalLanguage = updatedFile?.language || language;
    const canonicalAbridged =
      updatedFile !== undefined
        ? updatedFile.abridged === true
          ? "true"
          : ""
        : abridged;
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
      language: canonicalLanguage,
      abridged: canonicalAbridged,
      reviewOverride: draftReviewOverride,
    });
    requestClose();
  };

  const isLoading =
    updateFileMutation.isPending ||
    uploadCoverMutation.isPending ||
    setCoverPageMutation.isPending;

  const isSupplement = file.file_role === FileRoleSupplement;
  const isM4b = file.file_type === FileTypeM4B;

  // Check if file type can be a main file (only cbz, epub, m4b are supported)
  const canBeMainFile = [FileTypeCBZ, FileTypeEPUB, FileTypeM4B].includes(
    file.file_type as typeof FileTypeCBZ,
  );

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-3xl overflow-x-hidden">
        <DialogHeader>
          <DialogTitle>Edit File</DialogTitle>
          <DialogDescription>
            Update file metadata including cover, identifiers, and publishing
            details.
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6 min-w-0">
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
                      {/* Non-page-based: Show pending preview or current cover */}
                      {!isPageBased && (
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
                              key={`${file.id}-${coverCacheKey}`}
                              src={`/api/books/files/${file.id}/cover?v=${coverCacheKey}`}
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
                      {/* Page-based: Show pending page or current cover */}
                      {isPageBased && (
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
                              key={`${file.id}-${coverCacheKey}`}
                              src={`/api/books/files/${file.id}/cover?v=${coverCacheKey}`}
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
                    {/* Page number badge */}
                    {isPageBased &&
                      (pendingCoverPage ?? file.cover_page) != null && (
                        <div className="absolute bottom-1.5 left-1.5 px-1.5 py-0.5 rounded bg-black/70 text-white text-xs font-medium">
                          Page {(pendingCoverPage ?? file.cover_page)! + 1}
                        </div>
                      )}
                  </div>

                  {/* Action buttons and status */}
                  <div className="flex flex-col gap-2 pt-1">
                    {/* Upload button — hidden for files with page-derived covers */}
                    {!isPageBased && (
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
                    {/* Select page button */}
                    {isPageBased && (
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
                    {((!isPageBased && pendingCoverFile) ||
                      (isPageBased &&
                        pendingCoverPage !== null &&
                        pendingCoverPage !== file.cover_page)) && (
                      <span className="text-xs text-orange-500 font-medium">
                        Unsaved changes
                      </span>
                    )}
                  </div>
                </div>
              </div>

              {/* Page Picker Dialog */}
              {isPageBased && file.page_count != null && (
                <PagePicker
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
              {isM4b && (
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
                  <SortableEntityList<NameOption>
                    comboboxProps={{
                      getOptionKey: (p) => p.name,
                      getOptionLabel: (p) => p.name,
                      hook: function useNarratorOptions(q) {
                        return usePeopleSearch(file.library_id, open, q);
                      },
                      label: "Narrator",
                    }}
                    items={narratorItems}
                    onAppend={(next) => {
                      const nextName =
                        "__create" in next ? next.__create : next.name;
                      if (!nextName.trim()) return;
                      if (narrators.includes(nextName)) return;
                      setNarrators([...narrators, nextName]);
                    }}
                    onRemove={(idx) =>
                      setNarrators(narrators.filter((_, i) => i !== idx))
                    }
                    onReorder={(next) => setNarrators(next.map((n) => n.name))}
                  />
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

              {/* Language */}
              <div className="space-y-2">
                <Label>Language</Label>
                <LanguageCombobox
                  libraryId={file.library_id}
                  onChange={setLanguage}
                  value={language}
                />
              </div>

              {/* Abridged */}
              <div className="space-y-2">
                <Label>Abridged</Label>
                <div className="flex items-center gap-2">
                  <Checkbox
                    checked={abridged === "true"}
                    id="abridged"
                    onCheckedChange={(checked) =>
                      setAbridged(checked ? "true" : "")
                    }
                  />
                  <Label
                    className="cursor-pointer font-normal text-muted-foreground"
                    htmlFor="abridged"
                  >
                    This is an abridged edition
                  </Label>
                </div>
              </div>

              {/* Publisher */}
              <div className="space-y-2">
                <Label>Publisher</Label>
                <div className="flex items-center gap-2">
                  <div className="flex-1">
                    <EntityCombobox<NameOption>
                      getOptionKey={(item) => item.name}
                      getOptionLabel={(item) => item.name}
                      hook={function usePublisherOptions(q) {
                        return usePublisherSearch(file.library_id, open, q);
                      }}
                      label="Publisher"
                      onChange={(next) => {
                        const nextName =
                          "__create" in next ? next.__create : next.name;
                        setPublisher(nextName);
                      }}
                      value={publisher ? { name: publisher } : null}
                    />
                  </div>
                  {publisher && (
                    <Button
                      aria-label="Clear publisher"
                      className="cursor-pointer shrink-0"
                      onClick={() => setPublisher("")}
                      size="icon"
                      type="button"
                      variant="ghost"
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </div>

              {/* Imprint */}
              <div className="space-y-2">
                <Label>Imprint</Label>
                <div className="flex items-center gap-2">
                  <div className="flex-1">
                    <EntityCombobox<NameOption>
                      getOptionKey={(item) => item.name}
                      getOptionLabel={(item) => item.name}
                      hook={function useImprintOptions(q) {
                        return useImprintSearch(file.library_id, open, q);
                      }}
                      label="Imprint"
                      onChange={(next) => {
                        const nextName =
                          "__create" in next ? next.__create : next.name;
                        setImprint(nextName);
                      }}
                      value={imprint ? { name: imprint } : null}
                    />
                  </div>
                  {imprint && (
                    <Button
                      aria-label="Clear imprint"
                      className="cursor-pointer shrink-0"
                      onClick={() => setImprint("")}
                      size="icon"
                      type="button"
                      variant="ghost"
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  )}
                </div>
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
              <IdentifierEditor
                identifierTypes={availableIdentifierTypes}
                onChange={setIdentifiers}
                value={identifiers}
              />
            </>
          )}

          {/* Review Panel — controlled (deferred to Save button).
              When draftReviewOverride is null (auto), pass undefined so the
              panel falls back to the file-derived `reviewed` state. */}
          {(book ?? file.book) && (
            <ReviewPanel
              book={(book ?? file.book)!}
              files={[file]}
              isPending={setFileReviewMutation.isPending}
              onChange={(override) => setDraftReviewOverride(override)}
              toggleValue={
                draftReviewOverride === null
                  ? undefined
                  : draftReviewOverride === ReviewOverrideReviewed
              }
            />
          )}
        </DialogBody>

        <DialogFooter>
          <Button
            onClick={() => onOpenChange(false)}
            size="sm"
            variant="outline"
          >
            Cancel
          </Button>
          <Button disabled={isLoading} onClick={handleSubmit} size="sm">
            {isLoading && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </FormDialog>
  );
}
