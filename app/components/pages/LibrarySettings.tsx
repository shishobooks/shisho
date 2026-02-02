import equal from "fast-deep-equal";
import { ArrowLeft, FolderOpen, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";

import DirectoryPickerDialog from "@/components/library/DirectoryPickerDialog";
import LibraryLayout from "@/components/library/LibraryLayout";
import LibraryPluginsTab from "@/components/library/LibraryPluginsTab";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import { useLibrary, useUpdateLibrary } from "@/hooks/queries/libraries";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import {
  DownloadFormatAsk,
  DownloadFormatKepub,
  DownloadFormatOriginal,
} from "@/types/generated/models";

const LibrarySettings = () => {
  const { libraryId } = useParams<{ libraryId: string }>();
  const libraryQuery = useLibrary(libraryId);
  const updateLibraryMutation = useUpdateLibrary();

  usePageTitle(
    libraryQuery.data?.name
      ? `${libraryQuery.data.name} Settings`
      : "Library Settings",
  );

  const [name, setName] = useState("");
  const [organizeFileStructure, setOrganizeFileStructure] = useState(true);
  const [coverAspectRatio, setCoverAspectRatio] = useState("book");
  const [downloadFormatPreference, setDownloadFormatPreference] = useState(
    DownloadFormatOriginal,
  );
  const [libraryPaths, setLibraryPaths] = useState<string[]>([""]);
  const [isInitialized, setIsInitialized] = useState(false);
  const [pluginsHaveChanges, setPluginsHaveChanges] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerTargetIndex, setPickerTargetIndex] = useState<number | null>(
    null,
  );

  // Store initial values for change detection
  const [initialValues, setInitialValues] = useState<{
    name: string;
    organizeFileStructure: boolean;
    coverAspectRatio: string;
    downloadFormatPreference: string;
    libraryPaths: string[];
  } | null>(null);

  // Reset initialization state when library changes
  useEffect(() => {
    setIsInitialized(false);
  }, [libraryId]);

  // Initialize form when library data loads
  // Check data.id matches libraryId to prevent race condition when navigating
  // between libraries (stale cached data could initialize the form incorrectly)
  useEffect(() => {
    if (
      libraryQuery.isSuccess &&
      libraryQuery.data &&
      libraryQuery.data.id === Number(libraryId) &&
      !isInitialized
    ) {
      const initialName = libraryQuery.data.name;
      const initialOrganize = libraryQuery.data.organize_file_structure;
      const initialCover = libraryQuery.data.cover_aspect_ratio;
      const initialDownload =
        libraryQuery.data.download_format_preference || DownloadFormatOriginal;
      const initialPaths = libraryQuery.data.library_paths?.map(
        (lp) => lp.filepath,
      ) || [""];

      setName(initialName);
      setOrganizeFileStructure(initialOrganize);
      setCoverAspectRatio(initialCover);
      setDownloadFormatPreference(initialDownload);
      setLibraryPaths(initialPaths);
      setIsInitialized(true);

      // Store initial values for comparison
      setInitialValues({
        name: initialName,
        organizeFileStructure: initialOrganize,
        coverAspectRatio: initialCover,
        downloadFormatPreference: initialDownload,
        libraryPaths: initialPaths,
      });
    }
  }, [libraryQuery.isSuccess, libraryQuery.data, isInitialized, libraryId]);

  // Compute hasChanges by comparing current values to initial values
  const formHasChanges = useMemo(() => {
    if (!initialValues || !isInitialized) return false;
    return (
      name !== initialValues.name ||
      organizeFileStructure !== initialValues.organizeFileStructure ||
      coverAspectRatio !== initialValues.coverAspectRatio ||
      downloadFormatPreference !== initialValues.downloadFormatPreference ||
      !equal(libraryPaths, initialValues.libraryPaths)
    );
  }, [
    name,
    organizeFileStructure,
    coverAspectRatio,
    downloadFormatPreference,
    libraryPaths,
    isInitialized,
    initialValues,
  ]);

  // Combine form changes and plugin changes
  const hasChanges = formHasChanges || pluginsHaveChanges;

  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasChanges);

  const handleAddPath = () => {
    setLibraryPaths([...libraryPaths, ""]);
  };

  const handleRemovePath = (index: number) => {
    if (libraryPaths.length > 1) {
      setLibraryPaths(libraryPaths.filter((_, i) => i !== index));
    }
  };

  const handlePathChange = (index: number, value: string) => {
    const newPaths = [...libraryPaths];
    newPaths[index] = value;
    setLibraryPaths(newPaths);
  };

  const handleOpenPicker = (index: number) => {
    setPickerTargetIndex(index);
    setPickerOpen(true);
  };

  const handlePickerSelect = (paths: string[]) => {
    if (paths.length === 0) return;

    if (pickerTargetIndex !== null) {
      // Replace the target input with the first selected path.
      const newPaths = [...libraryPaths];
      newPaths[pickerTargetIndex] = paths[0];

      // If multiple paths selected, add the rest as new entries.
      if (paths.length > 1) {
        const additionalPaths = paths.slice(1);
        // Insert after the target index.
        newPaths.splice(pickerTargetIndex + 1, 0, ...additionalPaths);
      }

      setLibraryPaths(newPaths);
    }
    setPickerTargetIndex(null);
  };

  const handleSave = async () => {
    if (!libraryId) return;

    try {
      const validPaths = libraryPaths.filter((path) => path.trim() !== "");
      if (validPaths.length === 0) {
        toast.error("At least one library path is required");
        return;
      }

      await updateLibraryMutation.mutateAsync({
        id: libraryId,
        payload: {
          name: name.trim(),
          organize_file_structure: organizeFileStructure,
          cover_aspect_ratio: coverAspectRatio,
          download_format_preference: downloadFormatPreference,
          library_paths: validPaths,
        },
      });

      toast.success("Library settings saved!");

      // Update form state to match saved values (trimmed name, filtered paths)
      const trimmedName = name.trim();
      setName(trimmedName);
      setLibraryPaths(validPaths);

      // Update initial values to match saved values so hasChanges becomes false
      setInitialValues({
        name: trimmedName,
        organizeFileStructure,
        coverAspectRatio,
        downloadFormatPreference,
        libraryPaths: validPaths,
      });
    } catch (e) {
      let msg = "Something went wrong.";
      if (e instanceof Error) {
        msg = e.message;
      }
      toast.error(msg);
    }
  };

  if (libraryQuery.isLoading) {
    return (
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (!libraryQuery.isSuccess || !libraryQuery.data) {
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">Library Not Found</h1>
          <p className="text-muted-foreground mb-6">
            The library you're looking for doesn't exist or may have been
            removed.
          </p>
          <Button asChild>
            <Link to="/">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Home
            </Link>
          </Button>
        </div>
      </LibraryLayout>
    );
  }

  return (
    <LibraryLayout>
      <div className="mb-6">
        <Button asChild variant="ghost">
          <Link to={`/libraries/${libraryId}`}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Library
          </Link>
        </Button>
      </div>

      <div className="mb-8">
        <h1 className="text-2xl font-semibold mb-2">Library Settings</h1>
        <p className="text-muted-foreground">
          Manage library name, paths, and scanning behavior
        </p>
      </div>

      <div className="max-w-2xl space-y-6 border border-border rounded-md p-6">
        {/* Library Name */}
        <div className="space-y-2">
          <Label htmlFor="library-name">Library Name</Label>
          <Input
            id="library-name"
            onChange={(e) => setName(e.target.value)}
            placeholder="Enter library name"
            value={name}
          />
        </div>

        <Separator />

        {/* Library Paths */}
        <div className="space-y-4">
          <Label>Library Paths</Label>
          <p className="text-sm text-muted-foreground">
            Directories where Shisho will scan for books and media files
          </p>
          {libraryPaths.map((path, index) => (
            <div className="flex items-center gap-2" key={index}>
              <Input
                className="flex-1"
                onChange={(e) => handlePathChange(index, e.target.value)}
                placeholder="Enter directory path"
                value={path}
              />
              <Button
                onClick={() => handleOpenPicker(index)}
                size="icon"
                title="Browse directories"
                variant="outline"
              >
                <FolderOpen className="h-4 w-4" />
              </Button>
              {libraryPaths.length > 1 && (
                <Button
                  onClick={() => handleRemovePath(index)}
                  size="icon"
                  variant="outline"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              )}
            </div>
          ))}
          <Button onClick={handleAddPath} type="button" variant="outline">
            Add Path
          </Button>
        </div>

        <Separator />

        {/* Organize File Structure Setting */}
        <div className="space-y-4 flex flex-col gap-0.5">
          <Label>Scanning Options</Label>
          <div className="flex flex-col leading-none">
            <div className="flex items-center space-x-2">
              <Checkbox
                checked={organizeFileStructure}
                id="organize-files"
                onCheckedChange={(checked) =>
                  setOrganizeFileStructure(checked as boolean)
                }
              />
              <Label
                className="text-sm font-normal cursor-pointer"
                htmlFor="organize-files"
              >
                Organize file structure during scans
              </Label>
            </div>
            <p className="text-xs text-muted-foreground">
              When enabled, Shisho will reorganize files into a standardized
              directory structure during scanning operations.
            </p>
          </div>
        </div>

        <Separator />

        {/* Cover Aspect Ratio Setting */}
        <div className="space-y-2">
          <Label htmlFor="cover-aspect-ratio">Cover Display Aspect Ratio</Label>
          <p className="text-sm text-muted-foreground">
            How book and series covers should be displayed in gallery views
          </p>
          <Select onValueChange={setCoverAspectRatio} value={coverAspectRatio}>
            <SelectTrigger className="w-full" id="cover-aspect-ratio">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="book">Book Cover (2:3)</SelectItem>
              <SelectItem value="audiobook">Audiobook Cover (1:1)</SelectItem>
              <SelectItem value="book_fallback_audiobook">
                Book Cover (2:3), fallback to Audiobook (1:1)
              </SelectItem>
              <SelectItem value="audiobook_fallback_book">
                Audiobook Cover (1:1), fallback to Book (2:3)
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <Separator />

        {/* Download Format Preference Setting */}
        <div className="space-y-2">
          <Label htmlFor="download-format">Download Format Preference</Label>
          <p className="text-sm text-muted-foreground">
            How EPUB and CBZ files should be downloaded for e-readers
          </p>
          <Select
            onValueChange={setDownloadFormatPreference}
            value={downloadFormatPreference}
          >
            <SelectTrigger className="w-full" id="download-format">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={DownloadFormatOriginal}>
                Original format
              </SelectItem>
              <SelectItem value={DownloadFormatKepub}>
                KePub (Kobo-optimized)
              </SelectItem>
              <SelectItem value={DownloadFormatAsk}>Ask on download</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            KePub format improves reading statistics and page turning on Kobo
            devices. Only affects EPUB and CBZ files.
          </p>
        </div>

        <Separator />

        {/* Per-Library Plugin Order */}
        <div className="space-y-4">
          <Label>Plugin Order</Label>
          <p className="text-sm text-muted-foreground">
            Customize which plugins run and in what order for this library. By
            default, the global plugin order is used.
          </p>
          {libraryId && (
            <LibraryPluginsTab
              libraryId={libraryId}
              onHasChangesChange={setPluginsHaveChanges}
            />
          )}
        </div>

        <Separator />

        {/* Save Button */}
        <div className="flex justify-end pt-4">
          <Button
            disabled={updateLibraryMutation.isPending}
            onClick={handleSave}
          >
            {updateLibraryMutation.isPending ? "Saving..." : "Save Settings"}
          </Button>
        </div>
      </div>

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />

      <DirectoryPickerDialog
        initialPath={
          pickerTargetIndex !== null && libraryPaths[pickerTargetIndex]
            ? libraryPaths[pickerTargetIndex]
            : "/"
        }
        onOpenChange={setPickerOpen}
        onSelect={handlePickerSelect}
        open={pickerOpen}
      />
    </LibraryLayout>
  );
};

export default LibrarySettings;
