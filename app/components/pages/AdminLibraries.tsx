import { BookPlus, Plus, Settings } from "lucide-react";
import { useCallback } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import { useCreateLibrary, useLibraries } from "@/hooks/queries/libraries";
import { useAuth } from "@/hooks/useAuth";
import type { Library } from "@/types";

interface LibraryRowProps {
  library: Library;
}

const LibraryRow = ({ library }: LibraryRowProps) => (
  <div className="flex items-center justify-between py-3 md:py-4 px-4 md:px-6 hover:bg-muted/50 transition-colors gap-3">
    <div className="flex-1 min-w-0">
      <Link
        className="font-medium text-foreground hover:underline truncate block"
        to={`/libraries/${library.id}`}
      >
        {library.name}
      </Link>
      <p className="text-sm text-muted-foreground mt-0.5 md:mt-1">
        {library.library_paths?.length || 0} path
        {library.library_paths?.length !== 1 ? "s" : ""}
      </p>
    </div>
    <div className="flex items-center gap-2 shrink-0">
      <Button asChild size="sm" variant="ghost">
        <Link to={`/libraries/${library.id}/settings`}>
          <Settings className="h-4 w-4 sm:mr-2" />
          <span className="hidden sm:inline">Settings</span>
        </Link>
      </Button>
    </div>
  </div>
);

const AdminLibraries = () => {
  const navigate = useNavigate();
  const { hasPermission } = useAuth();
  const { data, isLoading, error } = useLibraries({});
  const createLibraryMutation = useCreateLibrary();
  const isDevelopment = import.meta.env.DEV;

  const canCreateLibraries = hasPermission("libraries", "write");

  const handleCreateDefaultLibrary = useCallback(async () => {
    try {
      const library = await createLibraryMutation.mutateAsync({
        payload: {
          name: "Main",
          cover_aspect_ratio: "book",
          library_paths: [
            "/Users/robinjoseph/code/personal/shisho/tmp/library",
          ],
        },
      });
      toast.success("Default library created! Scanning for media...");
      navigate(`/libraries/${library.id}`);
    } catch (e) {
      let msg = "Something went wrong.";
      if (e instanceof Error) {
        msg = e.message;
      }
      toast.error(msg);
    }
  }, [createLibraryMutation, navigate]);

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (error) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Error Loading Libraries</h1>
        <p className="text-muted-foreground">{error.message}</p>
      </div>
    );
  }

  const libraries = data?.libraries ?? [];

  return (
    <div>
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 md:mb-8">
        <div>
          <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
            Libraries
          </h1>
          <p className="text-sm md:text-base text-muted-foreground">
            Manage your media libraries and their settings.
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {isDevelopment && (
            <Button
              disabled={createLibraryMutation.isPending}
              onClick={handleCreateDefaultLibrary}
              size="sm"
              variant="outline"
            >
              <BookPlus className="h-4 w-4 sm:mr-2" />
              <span className="hidden sm:inline">
                Create default library (dev)
              </span>
            </Button>
          )}
          {canCreateLibraries && (
            <Button asChild size="sm">
              <Link to="/libraries/create">
                <Plus className="h-4 w-4 sm:mr-2" />
                <span className="hidden sm:inline">Add Library</span>
              </Link>
            </Button>
          )}
        </div>
      </div>

      {libraries.length === 0 ? (
        <div className="border border-border rounded-md p-8 text-center">
          <p className="text-muted-foreground mb-4">No libraries found.</p>
          {canCreateLibraries && (
            <Button asChild size="sm">
              <Link to="/libraries/create">
                <Plus className="h-4 w-4 mr-2" />
                Create your first library
              </Link>
            </Button>
          )}
        </div>
      ) : (
        <div className="border border-border rounded-md divide-y divide-border">
          {libraries.map((library) => (
            <LibraryRow key={library.id} library={library} />
          ))}
        </div>
      )}
    </div>
  );
};

export default AdminLibraries;
