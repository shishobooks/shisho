import { createBrowserRouter } from "react-router-dom";

import ProtectedRoute from "@/components/library/ProtectedRoute";
import AdminJobs from "@/components/pages/AdminJobs";
import AdminLayout from "@/components/pages/AdminLayout";
import AdminSettings from "@/components/pages/AdminSettings";
import AdminUsers from "@/components/pages/AdminUsers";
import BookDetail from "@/components/pages/BookDetail";
import CreateLibrary from "@/components/pages/CreateLibrary";
import CreateUser from "@/components/pages/CreateUser";
import Home from "@/components/pages/Home";
import LibraryList from "@/components/pages/LibraryList";
import LibraryRedirect from "@/components/pages/LibraryRedirect";
import LibrarySettings from "@/components/pages/LibrarySettings";
import Login from "@/components/pages/Login";
import PersonDetail from "@/components/pages/PersonDetail";
import PersonList from "@/components/pages/PersonList";
import Root from "@/components/pages/Root";
import SeriesDetail from "@/components/pages/SeriesDetail";
import SeriesList from "@/components/pages/SeriesList";
import Setup from "@/components/pages/Setup";
import UserDetail from "@/components/pages/UserDetail";

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
  // Admin routes with dedicated layout
  {
    path: "/admin",
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
        path: "settings",
        element: (
          <ProtectedRoute
            requiredPermission={{ resource: "config", operation: "read" }}
          >
            <AdminSettings />
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
        path: "libraries",
        element: (
          <ProtectedRoute>
            <LibraryList />
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
    ],
  },
]);
