import { ArrowLeft, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";

import LibraryLayout from "@/components/library/LibraryLayout";
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
import { useLibrary, useUpdateLibrary } from "@/hooks/queries/libraries";
import {
  DownloadFormatAsk,
  DownloadFormatKepub,
  DownloadFormatOriginal,
} from "@/types/generated/models";

const LibrarySettings = () => {
  const { libraryId } = useParams<{ libraryId: string }>();
  const libraryQuery = useLibrary(libraryId);
  const updateLibraryMutation = useUpdateLibrary();

  const [name, setName] = useState("");
  const [organizeFileStructure, setOrganizeFileStructure] = useState(true);
  const [coverAspectRatio, setCoverAspectRatio] = useState("book");
  const [downloadFormatPreference, setDownloadFormatPreference] = useState(
    DownloadFormatOriginal,
  );
  const [libraryPaths, setLibraryPaths] = useState<string[]>([""]);
  const [isInitialized, setIsInitialized] = useState(false);

  // Initialize form when library data loads
  if (libraryQuery.isSuccess && libraryQuery.data && !isInitialized) {
    setName(libraryQuery.data.name);
    setOrganizeFileStructure(libraryQuery.data.organize_file_structure);
    setCoverAspectRatio(libraryQuery.data.cover_aspect_ratio);
    setDownloadFormatPreference(
      libraryQuery.data.download_format_preference || DownloadFormatOriginal,
    );
    setLibraryPaths(
      libraryQuery.data.library_paths?.map((lp) => lp.filepath) || [""],
    );
    setIsInitialized(true);
  }

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
    </LibraryLayout>
  );
};

export default LibrarySettings;
