import { Navigate } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import TopNav from "@/components/library/TopNav";
import { useNavLibraries } from "@/hooks/queries/libraries";
import { useUserSettings } from "@/hooks/queries/settings";
import { useCan } from "@/hooks/useCan";

const LibraryRedirect = () => {
  // A library page needs Books Read, so a role without it lands on lists,
  // which every signed-in user can use.
  const canReadBooks = useCan("books:read");
  const canReadLibraries = useCan("libraries:read");
  const librariesQuery = useNavLibraries();
  // Settings that fail to load only cost the user their default library, so
  // an error falls through to the first library rather than blocking `/`.
  const userSettingsQuery = useUserSettings({ enabled: canReadBooks });

  if (!canReadBooks) {
    return <Navigate replace to="/lists" />;
  }

  if (librariesQuery.isLoading || userSettingsQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="flex h-[calc(100vh-var(--demo-banner-height,0px))] items-center justify-center">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (librariesQuery.error && !librariesQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="w-full px-4 md:px-6 py-4 md:py-8">
          <QueryError
            fallback="Failed to load libraries"
            query={librariesQuery}
          />
        </div>
      </div>
    );
  }

  const libraries = librariesQuery.data ?? [];

  // With no libraries, a role that can manage them goes to create one, and
  // any other role lands on lists.
  if (libraries.length === 0) {
    return (
      <Navigate replace to={canReadLibraries ? "/admin/libraries" : "/lists"} />
    );
  }

  // Open the user's default library while they can still access it, and
  // otherwise the first library (user can switch via dropdown).
  const defaultLibraryId = userSettingsQuery.data?.default_library_id;
  const library =
    libraries.find((l) => l.id === defaultLibraryId) ?? libraries[0];
  return <Navigate replace to={`/libraries/${library.id}`} />;
};

export default LibraryRedirect;
