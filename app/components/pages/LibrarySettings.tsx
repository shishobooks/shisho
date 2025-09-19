import { ArrowLeft, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { useLibrary, useUpdateLibrary } from "@/hooks/queries/libraries";

const LibrarySettings = () => {
  const { libraryId } = useParams<{ libraryId: string }>();
  const libraryQuery = useLibrary(libraryId);
  const updateLibraryMutation = useUpdateLibrary();

  const [name, setName] = useState("");
  const [organizeFileStructure, setOrganizeFileStructure] = useState(true);
  const [libraryPaths, setLibraryPaths] = useState<string[]>([""]);
  const [isInitialized, setIsInitialized] = useState(false);

  // Initialize form when library data loads
  if (libraryQuery.isSuccess && libraryQuery.data && !isInitialized) {
    setName(libraryQuery.data.name);
    setOrganizeFileStructure(libraryQuery.data.organize_file_structure);
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
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!libraryQuery.isSuccess || !libraryQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <div className="text-center">
            <h1 className="text-2xl font-bold mb-4">Library Not Found</h1>
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
        </div>
      </div>
    );
  }

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full p-8 m-auto">
        <div className="mb-6">
          <Button asChild variant="ghost">
            <Link to={`/libraries/${libraryId}`}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Library
            </Link>
          </Button>
        </div>

        <div className="mb-8">
          <h1 className="text-3xl font-bold mb-2">Library Settings</h1>
          <p className="text-muted-foreground">
            Manage library name, paths, and scanning behavior
          </p>
        </div>

        <Card className="max-w-2xl">
          <CardHeader>
            <CardTitle>Library Configuration</CardTitle>
            <CardDescription>
              Configure how this library behaves and where it scans for content
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
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

            {/* Save Button */}
            <div className="flex justify-end">
              <Button
                disabled={updateLibraryMutation.isPending}
                onClick={handleSave}
              >
                {updateLibraryMutation.isPending
                  ? "Saving..."
                  : "Save Settings"}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default LibrarySettings;
