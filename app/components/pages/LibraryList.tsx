import { Plus, Settings } from "lucide-react";
import { Link } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { useLibraries } from "@/hooks/queries/libraries";

const LibraryList = () => {
  const librariesQuery = useLibraries({});

  if (librariesQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (librariesQuery.isError) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">
              Error Loading Libraries
            </h1>
            <p className="text-muted-foreground">
              There was an error loading your libraries. Please try again.
            </p>
          </div>
        </div>
      </div>
    );
  }

  const libraries = librariesQuery.data?.libraries || [];

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        <div className="mb-8">
          <h1 className="text-2xl font-semibold mb-2">Libraries</h1>
          <p className="text-muted-foreground">
            Select a library to browse your collection
          </p>
        </div>

        <div className="mb-6 flex items-center justify-between">
          <div></div>
          <Button asChild>
            <Link to="/libraries/create">
              <Plus className="mr-2 h-4 w-4" />
              Create Library
            </Link>
          </Button>
        </div>

        {libraries.length === 0 ? (
          <div className="text-center py-12">
            <h2 className="text-xl font-semibold mb-2">No Libraries Found</h2>
            <p className="text-muted-foreground mb-6">
              You haven't created any libraries yet. Create your first library
              to get started.
            </p>
            <Button asChild>
              <Link to="/libraries/create">Create Library</Link>
            </Button>
          </div>
        ) : (
          <div className="space-y-4">
            {libraries.map((library) => (
              <div
                className="border border-border rounded-md p-6 hover:bg-accent/50 transition-colors"
                key={library.id}
              >
                <div className="flex items-start justify-between mb-4">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <h2 className="text-xl font-semibold">{library.name}</h2>
                      <Button
                        asChild
                        className="h-8 w-8"
                        size="icon"
                        variant="ghost"
                      >
                        <Link to={`/libraries/${library.id}/settings`}>
                          <Settings className="h-4 w-4" />
                        </Link>
                      </Button>
                    </div>
                    <p className="text-sm text-muted-foreground mb-3">
                      {library.library_paths?.length || 0} path
                      {library.library_paths?.length !== 1 ? "s" : ""}{" "}
                      configured
                    </p>
                    {library.library_paths &&
                      library.library_paths.length > 0 && (
                        <div className="space-y-1">
                          {library.library_paths.map((path, index) => (
                            <div
                              className="text-sm text-muted-foreground truncate font-mono"
                              key={index}
                              title={path.filepath}
                            >
                              {path.filepath}
                            </div>
                          ))}
                        </div>
                      )}
                  </div>
                  <Button asChild className="ml-4">
                    <Link to={`/libraries/${library.id}`}>Browse Library</Link>
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default LibraryList;
