import { Navigate } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { useLibraries } from "@/hooks/queries/libraries";

const LibraryRedirect = () => {
  const librariesQuery = useLibraries({});

  if (librariesQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="flex items-center justify-center h-screen">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (librariesQuery.isError) {
    return (
      <div>
        <TopNav />
        <div className="flex items-center justify-center min-h-[60vh]">
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

  // If single library, redirect to it
  if (libraries.length === 1) {
    return <Navigate replace to={`/libraries/${libraries[0].id}`} />;
  }

  // If multiple libraries or no libraries, redirect to library list
  return <Navigate replace to="/libraries" />;
};

export default LibraryRedirect;
