import { Settings } from "lucide-react";
import { Link } from "react-router-dom";

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
import { useLibraries } from "@/hooks/queries/libraries";

const LibraryList = () => {
  const librariesQuery = useLibraries({});

  if (librariesQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (librariesQuery.isError) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <div className="text-center">
            <h1 className="text-2xl font-bold mb-4">Error Loading Libraries</h1>
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
      <div className="max-w-7xl w-full p-8 m-auto">
        <div className="mb-8">
          <h1 className="text-3xl font-bold mb-2">Libraries</h1>
          <p className="text-muted-foreground">
            Select a library to browse your collection
          </p>
        </div>

        {libraries.length === 0 ? (
          <div className="text-center py-12">
            <h2 className="text-xl font-semibold mb-2">No Libraries Found</h2>
            <p className="text-muted-foreground mb-6">
              You haven't created any libraries yet. Create your first library
              to get started.
            </p>
            <Button asChild>
              <Link to="/config">Create Library</Link>
            </Button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {libraries.map((library) => (
              <Card
                className="hover:shadow-md transition-shadow"
                key={library.id}
              >
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-xl">{library.name}</CardTitle>
                    <Button asChild size="icon" variant="ghost">
                      <Link to={`/libraries/${library.id}/settings`}>
                        <Settings className="h-4 w-4" />
                      </Link>
                    </Button>
                  </div>
                  <CardDescription>
                    {library.library_paths?.length || 0} path
                    {library.library_paths?.length !== 1 ? "s" : ""} configured
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    {library.library_paths?.map((path, index) => (
                      <div
                        className="text-sm text-muted-foreground truncate"
                        key={index}
                        title={path.filepath}
                      >
                        {path.filepath}
                      </div>
                    ))}
                    <div className="pt-3">
                      <Button asChild className="w-full">
                        <Link to={`/libraries/${library.id}`}>
                          Browse Library
                        </Link>
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default LibraryList;
