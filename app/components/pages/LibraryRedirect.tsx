import { Navigate } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { useNavLibraries } from "@/hooks/queries/libraries";
import { useCan } from "@/hooks/useCan";

const LibraryRedirect = () => {
  // A library page needs Books Read, so a role without it lands on lists,
  // which every signed-in user can use.
  const canReadBooks = useCan("books:read");
  const canReadLibraries = useCan("libraries:read");
  const librariesQuery = useNavLibraries();

  if (!canReadBooks) {
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

  const libraries = librariesQuery.data ?? [];

  // With no libraries, a role that can manage them goes to create one, and
  // any other role lands on lists.
  if (libraries.length === 0) {
    return (
      <Navigate
        replace
        to={canReadLibraries ? "/settings/libraries" : "/lists"}
      />
    );
  }

  // Redirect to the first library (user can switch via dropdown)
  return <Navigate replace to={`/libraries/${libraries[0].id}`} />;
};

export default LibraryRedirect;
