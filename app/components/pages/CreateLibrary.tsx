import { ArrowLeft, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { useCreateLibrary } from "@/hooks/queries/libraries";

const CreateLibrary = () => {
  const navigate = useNavigate();
  const createLibraryMutation = useCreateLibrary();

  const [name, setName] = useState("");
  const [organizeFileStructure, setOrganizeFileStructure] = useState(true);
  const [libraryPaths, setLibraryPaths] = useState<string[]>([""]);

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
          library_paths: validPaths,
        },
      });

      toast.success("Library created!");
      navigate(`/libraries/${library.id}`);
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
            <Link to="/libraries">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Libraries
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
    </div>
  );
};

export default CreateLibrary;
