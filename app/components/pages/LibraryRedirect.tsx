import { useEffect } from "react";
import { Navigate } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { useLibraries } from "@/hooks/queries/libraries";

const LibraryRedirect = () => {
  const librariesQuery = useLibraries({});

  useEffect(() => {
    if (
      librariesQuery.isSuccess &&
      librariesQuery.data.libraries.length === 0
    ) {
      // TODO: Show create library interface or redirect to config
      console.warn(
        "No libraries found - need to implement library creation flow",
      );
    }
  }, [librariesQuery.isSuccess, librariesQuery.data?.libraries.length]);

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

  // If multiple libraries, redirect to library list
  if (libraries.length > 1) {
    return <Navigate replace to="/libraries" />;
  }

  // No libraries - for now just show a message
  // TODO: Implement library creation flow
  return (
    <div>
      <TopNav />
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">No Libraries Found</h1>
          <p className="text-muted-foreground">
            Please create a library to get started.
          </p>
        </div>
      </div>
    </div>
  );
};

export default LibraryRedirect;
