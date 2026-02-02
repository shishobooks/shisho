import { ArrowLeft, FolderOpen, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import DirectoryPickerDialog from "@/components/library/DirectoryPickerDialog";
import TopNav from "@/components/library/TopNav";
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
import { useCreateLibrary } from "@/hooks/queries/libraries";
import { useNavigateAfterSave } from "@/hooks/useNavigateAfterSave";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import {
  DownloadFormatAsk,
  DownloadFormatKepub,
  DownloadFormatOriginal,
} from "@/types/generated/models";

// Initial values for the create form - stored once to compare against
const INITIAL_VALUES = {
  name: "",
  organizeFileStructure: true,
  coverAspectRatio: "book",
  downloadFormatPreference: DownloadFormatOriginal,
  libraryPaths: [""],
};

const CreateLibrary = () => {
  usePageTitle("Create Library");

  const createLibraryMutation = useCreateLibrary();

  const [name, setName] = useState(INITIAL_VALUES.name);
  const [organizeFileStructure, setOrganizeFileStructure] = useState(
    INITIAL_VALUES.organizeFileStructure,
  );
  const [coverAspectRatio, setCoverAspectRatio] = useState(
    INITIAL_VALUES.coverAspectRatio,
  );
  const [downloadFormatPreference, setDownloadFormatPreference] = useState(
    INITIAL_VALUES.downloadFormatPreference,
  );
  const [libraryPaths, setLibraryPaths] = useState<string[]>([
    ...INITIAL_VALUES.libraryPaths,
  ]);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerTargetIndex, setPickerTargetIndex] = useState<number | null>(
    null,
  );
  const [changesSaved, setChangesSaved] = useState(false);

  // Track whether user has unsaved changes by comparing against initial values
  const hasUnsavedChanges = useMemo(() => {
    if (changesSaved) return false;
    return (
      name.trim() !== INITIAL_VALUES.name ||
      organizeFileStructure !== INITIAL_VALUES.organizeFileStructure ||
      coverAspectRatio !== INITIAL_VALUES.coverAspectRatio ||
      downloadFormatPreference !== INITIAL_VALUES.downloadFormatPreference ||
      libraryPaths.some((p) => p.trim() !== "")
    );
  }, [
    name,
    organizeFileStructure,
    coverAspectRatio,
    downloadFormatPreference,
    libraryPaths,
    changesSaved,
  ]);

  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasUnsavedChanges);

  const { requestNavigate } = useNavigateAfterSave(hasUnsavedChanges);

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

  const handleCreate = async () => {
    const validPaths = libraryPaths.filter((path) => path.trim() !== "");
    if (validPaths.length === 0) {
      toast.error("At least one library path is required");
      return;
    }

    if (!name.trim()) {
      toast.error("Library name is required");
      return;
    }

    try {
      const library = await createLibraryMutation.mutateAsync({
        payload: {
          name: name.trim(),
          organize_file_structure: organizeFileStructure,
          cover_aspect_ratio: coverAspectRatio,
          download_format_preference: downloadFormatPreference,
          library_paths: validPaths,
        },
      });

      // Backend automatically triggers a scan after library creation
      toast.success("Library created! Scanning for media...");
      setChangesSaved(true);
      requestNavigate(`/libraries/${library.id}`);
    } catch (e) {
      let msg = "Something went wrong.";
      if (e instanceof Error) {
        msg = e.message;
      }
      toast.error(msg);
    }
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        <div className="mb-6">
          <Button asChild variant="ghost">
            <Link to="/">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back
            </Link>
          </Button>
        </div>

        <div className="mb-8">
          <h1 className="text-2xl font-semibold mb-2">Create Library</h1>
          <p className="text-muted-foreground">
            Create a new library to organize your book collection
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
            <div>
              <Label>Library Paths</Label>
              <p className="text-sm text-muted-foreground mt-1">
                Directories where Shisho will scan for books and media files
              </p>
            </div>
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
              <Plus className="mr-2 h-4 w-4" />
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
            <Label htmlFor="cover-aspect-ratio">
              Cover Display Aspect Ratio
            </Label>
            <p className="text-sm text-muted-foreground">
              How book and series covers should be displayed in gallery views
            </p>
            <Select
              onValueChange={setCoverAspectRatio}
              value={coverAspectRatio}
            >
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
                <SelectItem value={DownloadFormatAsk}>
                  Ask on download
                </SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              KePub format improves reading statistics and page turning on Kobo
              devices. Only affects EPUB and CBZ files.
            </p>
          </div>

          <Separator />

          {/* Create Button */}
          <div className="flex justify-end pt-4">
            <Button
              disabled={createLibraryMutation.isPending}
              onClick={handleCreate}
            >
              {createLibraryMutation.isPending
                ? "Creating..."
                : "Create Library"}
            </Button>
          </div>
        </div>
      </div>

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

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </div>
  );
};

export default CreateLibrary;
