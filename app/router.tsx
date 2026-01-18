import { createBrowserRouter } from "react-router-dom";

import ProtectedRoute from "@/components/library/ProtectedRoute";
import AdminJobs from "@/components/pages/AdminJobs";
import AdminLayout from "@/components/pages/AdminLayout";
import AdminLibraries from "@/components/pages/AdminLibraries";
import AdminSettings from "@/components/pages/AdminSettings";
import AdminUsers from "@/components/pages/AdminUsers";
import BookDetail from "@/components/pages/BookDetail";
import CreateLibrary from "@/components/pages/CreateLibrary";
import CreateUser from "@/components/pages/CreateUser";
import GenreDetail from "@/components/pages/GenreDetail";
import GenresList from "@/components/pages/GenresList";
import Home from "@/components/pages/Home";
import ImprintDetail from "@/components/pages/ImprintDetail";
import ImprintsList from "@/components/pages/ImprintsList";
import JobDetail from "@/components/pages/JobDetail";
import LibraryRedirect from "@/components/pages/LibraryRedirect";
import LibrarySettings from "@/components/pages/LibrarySettings";
import Login from "@/components/pages/Login";
import PersonDetail from "@/components/pages/PersonDetail";
import PersonList from "@/components/pages/PersonList";
import PublisherDetail from "@/components/pages/PublisherDetail";
import PublishersList from "@/components/pages/PublishersList";
import Root from "@/components/pages/Root";
import SecuritySettings from "@/components/pages/SecuritySettings";
import SeriesDetail from "@/components/pages/SeriesDetail";
import SeriesList from "@/components/pages/SeriesList";
import Setup from "@/components/pages/Setup";
import TagDetail from "@/components/pages/TagDetail";
import TagsList from "@/components/pages/TagsList";
import UserDetail from "@/components/pages/UserDetail";
import UserSettings from "@/components/pages/UserSettings";

export const router = createBrowserRouter([
  // Public routes (no authentication required)
  {
    path: "/login",
    Component: Login,
  },
  {
    path: "/setup",
    Component: Setup,
  },
  // Settings routes with dedicated layout (formerly admin)
  {
    path: "/settings",
    element: (
      <ProtectedRoute>
        <AdminLayout />
      </ProtectedRoute>
    ),
    children: [
      {
        index: true,
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "config", operation: "read" }}
          >
            <AdminSettings />
          </ProtectedRoute>
        ),
      },
      {
        path: "server",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "config", operation: "read" }}
          >
            <AdminSettings />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "libraries", operation: "read" }}
          >
            <AdminLibraries />
          </ProtectedRoute>
        ),
      },
      {
        path: "users",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "users", operation: "read" }}
          >
            <AdminUsers />
          </ProtectedRoute>
        ),
      },
      {
        path: "users/create",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "users", operation: "write" }}
          >
            <CreateUser />
          </ProtectedRoute>
        ),
      },
      {
        path: "users/:id",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "users", operation: "read" }}
          >
            <UserDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "jobs",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "jobs", operation: "read" }}
          >
            <AdminJobs />
          </ProtectedRoute>
        ),
      },
      {
        path: "jobs/:id",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "jobs", operation: "read" }}
          >
            <JobDetail />
          </ProtectedRoute>
        ),
      },
    ],
  },
  // Protected routes (require authentication)
  {
    path: "/",
    Component: Root,
    children: [
      {
        path: "",
        element: (
          <ProtectedRoute>
            <LibraryRedirect />
          </ProtectedRoute>
        ),
      },
      {
        path: "user/settings",
        element: (
          <ProtectedRoute>
            <UserSettings />
          </ProtectedRoute>
        ),
      },
      {
        path: "user/security",
        element: (
          <ProtectedRoute>
            <SecuritySettings />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/create",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "libraries", operation: "write" }}
          >
            <CreateLibrary />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <Home />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/settings",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={{ resource: "libraries", operation: "write" }}
          >
            <LibrarySettings />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/books/:id",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <BookDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/series",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <SeriesList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/series/:id",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <SeriesDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/people",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <PersonList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/people/:id",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <PersonDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/genres",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <GenresList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/genres/:id",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <GenreDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/tags",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <TagsList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/tags/:id",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <TagDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/publishers",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <PublishersList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/publishers/:id",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <PublisherDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/imprints",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <ImprintsList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/imprints/:id",
        element: (
          <ProtectedRoute checkLibraryAccess>
            <ImprintDetail />
          </ProtectedRoute>
        ),
      },
    ],
  },
]);
