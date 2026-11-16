import { List, LockKeyhole } from "lucide-react";
import { Link, Navigate } from "react-router-dom";

import EmptyState from "@/components/library/EmptyState";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { ContentWidth } from "@/contexts/ContentWidth";
import { useNavLibraries } from "@/hooks/queries/libraries";
import { useUserSettings } from "@/hooks/queries/settings";
import { useAuth } from "@/hooks/useAuth";
import { useCan } from "@/hooks/useCan";
import { usePageTitle } from "@/hooks/usePageTitle";

// What a reader sees at `/` before an admin gives them any library. Lists
// stay reachable, from here and the user menu, for anything shared with them.
const NoLibraryAccess = () => {
  usePageTitle("Libraries");
  return (
    <div>
      <TopNav />
      <ContentWidth className="px-4 py-4 md:px-6 md:py-8">
        <EmptyState
          action={
            <Button asChild size="sm" variant="outline">
              <Link to="/lists">
                <List aria-hidden="true" className="h-4 w-4" />
                Open Lists
              </Link>
            </Button>
          }
          description="Ask your server admin to give you access."
          headingLevel={1}
          icon={LockKeyhole}
          title="You don't have access to any libraries yet"
        />
      </ContentWidth>
    </div>
  );
};

const LibraryRedirect = () => {
  // A library page needs Books Read, so a role without it lands on lists,
  // which every signed-in user can use.
  const canReadBooks = useCan("books:read");
  const canReadLibraries = useCan("libraries:read");
  // A user limited to some libraries sees only those on the admin page too,
  // so an empty list there would wrongly read as none created.
  const { user } = useAuth();
  const hasAllLibraryAccess = !user?.library_access;
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

  // With no libraries, a role that can manage every library goes to create
  // one. Anyone else is told why there is nothing to open, rather than
  // landing on Lists with no explanation.
  if (libraries.length === 0) {
    return canReadLibraries && hasAllLibraryAccess ? (
      <Navigate replace to="/admin/libraries" />
    ) : (
      <NoLibraryAccess />
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
