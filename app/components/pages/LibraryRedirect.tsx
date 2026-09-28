import { Navigate } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { useLibraries } from "@/hooks/queries/libraries";
import { useAuth } from "@/hooks/useAuth";

const LibraryRedirect = () => {
  const { user, hasPermission } = useAuth();
  const canReadLibraries = hasPermission("libraries", "read");
  const librariesQuery = useLibraries({}, { enabled: canReadLibraries });

  if (!canReadLibraries) {
    // Without Libraries Read the library list is a 403, so pick the landing
    // page from the auth payload instead. A library page needs Books Read to
    // show anything. library_access is null for all libraries and empty for
    // none, and neither names an id to open, so those roles land on lists,
    // which every signed-in user can use.
    const accessibleIds = user?.library_access ?? [];
    if (hasPermission("books", "read") && accessibleIds.length > 0) {
      return <Navigate replace to={`/libraries/${accessibleIds[0]}`} />;
    }
    return <Navigate replace to="/lists" />;
  }

  if (librariesQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="flex h-[calc(100vh-var(--demo-banner-height,0px))] items-center justify-center">
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

  const libraries = librariesQuery.data?.items || [];

  // If no libraries, redirect to settings/libraries to create one
  if (libraries.length === 0) {
    return <Navigate replace to="/settings/libraries" />;
  }

  // Redirect to the first library (user can switch via dropdown)
  return <Navigate replace to={`/libraries/${libraries[0].id}`} />;
};

export default LibraryRedirect;
