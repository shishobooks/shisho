import {
  createBrowserRouter,
  Navigate,
  type RouteObject,
} from "react-router-dom";

import ProtectedRoute from "@/components/library/ProtectedRoute";
import AdminCache from "@/components/pages/AdminCache";
import AdminJobs from "@/components/pages/AdminJobs";
import AdminLayout from "@/components/pages/AdminLayout";
import AdminLibraries from "@/components/pages/AdminLibraries";
import AdminLogs from "@/components/pages/AdminLogs";
import AdminPlugins from "@/components/pages/AdminPlugins";
import AdminReviewCriteria from "@/components/pages/AdminReviewCriteria";
import AdminSettings from "@/components/pages/AdminSettings";
import AdminSharing from "@/components/pages/AdminSharing";
import AdminUsers from "@/components/pages/AdminUsers";
import BookDetail from "@/components/pages/BookDetail";
import CreateLibrary from "@/components/pages/CreateLibrary";
import CreateUser from "@/components/pages/CreateUser";
import FileDetail from "@/components/pages/FileDetail";
import FileReader from "@/components/pages/FileReader";
import GenreDetail from "@/components/pages/GenreDetail";
import GenresList from "@/components/pages/GenresList";
import Home from "@/components/pages/Home";
import JobDetail from "@/components/pages/JobDetail";
import LibraryRedirect from "@/components/pages/LibraryRedirect";
import LibrarySettings from "@/components/pages/LibrarySettings";
import ListDetail from "@/components/pages/ListDetail";
import ListsIndex from "@/components/pages/ListsIndex";
import Login from "@/components/pages/Login";
import PersonDetail from "@/components/pages/PersonDetail";
import PersonList from "@/components/pages/PersonList";
import { PluginDetail } from "@/components/pages/PluginDetail";
import PublisherDetail from "@/components/pages/PublisherDetail";
import PublishersList from "@/components/pages/PublishersList";
import Root from "@/components/pages/Root";
import SecuritySettings from "@/components/pages/SecuritySettings";
import SeriesDetail from "@/components/pages/SeriesDetail";
import SeriesList from "@/components/pages/SeriesList";
import SettingsIndexRedirect from "@/components/pages/SettingsIndexRedirect";
import Setup from "@/components/pages/Setup";
import { shareRoutes } from "@/components/pages/shareRoutes";
import TagDetail from "@/components/pages/TagDetail";
import TagsList from "@/components/pages/TagsList";
import UserDetail from "@/components/pages/UserDetail";
import UserSettings from "@/components/pages/UserSettings";
import { ROUTE_PERMISSIONS } from "@/utils/permissions";

export const routes: RouteObject[] = [
  // Public routes (no authentication required)
  {
    path: "/login",
    Component: Login,
  },
  {
    path: "/setup",
    Component: Setup,
  },
  ...shareRoutes,
  // Protected routes (require authentication)
  {
    path: "/",
    Component: Root,
    children: [
      // Settings routes with dedicated layout (formerly admin)
      {
        path: "settings",
        element: (
          <ProtectedRoute>
            <AdminLayout />
          </ProtectedRoute>
        ),
        children: [
          {
            index: true,
            element: <SettingsIndexRedirect />,
          },
          {
            path: "server",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <AdminSettings />
              </ProtectedRoute>
            ),
          },
          {
            path: "libraries",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsLibraries}
              >
                <AdminLibraries />
              </ProtectedRoute>
            ),
          },
          {
            path: "users",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsUsers}
              >
                <AdminUsers />
              </ProtectedRoute>
            ),
          },
          {
            path: "users/create",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsCreateUser}
              >
                <CreateUser />
              </ProtectedRoute>
            ),
          },
          {
            path: "users/:id",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsUsers}
              >
                <UserDetail />
              </ProtectedRoute>
            ),
          },
          {
            path: "jobs",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsJobs}
              >
                <AdminJobs />
              </ProtectedRoute>
            ),
          },
          {
            path: "jobs/:id",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsJobs}
              >
                <JobDetail />
              </ProtectedRoute>
            ),
          },
          {
            path: "plugins",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <AdminPlugins />
              </ProtectedRoute>
            ),
          },
          {
            path: "plugins/installed",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <AdminPlugins />
              </ProtectedRoute>
            ),
          },
          {
            path: "plugins/discover",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <AdminPlugins />
              </ProtectedRoute>
            ),
          },
          {
            path: "plugins/browse",
            element: <Navigate replace to="/settings/plugins/discover" />,
          },
          {
            path: "plugins/order",
            element: <Navigate replace to="/settings/plugins?advanced=order" />,
          },
          {
            path: "plugins/repositories",
            element: (
              <Navigate replace to="/settings/plugins?advanced=repositories" />
            ),
          },
          {
            path: "plugins/:scope/:id",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <PluginDetail />
              </ProtectedRoute>
            ),
          },
          {
            path: "cache",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <AdminCache />
              </ProtectedRoute>
            ),
          },
          {
            path: "logs",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <AdminLogs />
              </ProtectedRoute>
            ),
          },
          {
            path: "review-criteria",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <AdminReviewCriteria />
              </ProtectedRoute>
            ),
          },
          {
            path: "sharing",
            element: (
              <ProtectedRoute
                requiredPermission={ROUTE_PERMISSIONS.settingsConfig}
              >
                <AdminSharing />
              </ProtectedRoute>
            ),
          },
        ],
      },
      {
        path: "",
        element: (
          <ProtectedRoute>
            <LibraryRedirect />
          </ProtectedRoute>
        ),
      },
      {
        path: "lists",
        element: (
          <ProtectedRoute>
            <ListsIndex />
          </ProtectedRoute>
        ),
      },
      {
        path: "lists/:id",
        element: (
          <ProtectedRoute>
            <ListDetail />
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
          <ProtectedRoute unavailableInDemo>
            <SecuritySettings />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/create",
        element: (
          <ProtectedRoute requiredPermission={ROUTE_PERMISSIONS.createLibrary}>
            <CreateLibrary />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <Home />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/settings",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.librarySettings}
          >
            <LibrarySettings />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/books/:id",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <BookDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/books/:bookId/files/:fileId/:tab?",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <FileDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/books/:bookId/files/:fileId/read",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <FileReader />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/series",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.librarySeries}
          >
            <SeriesList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/series/:id",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.librarySeries}
          >
            <SeriesDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/people",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryPeople}
          >
            <PersonList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/people/:id",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryPeople}
          >
            <PersonDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/genres",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <GenresList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/genres/:id",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <GenreDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/tags",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <TagsList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/tags/:id",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <TagDetail />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/publishers",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <PublishersList />
          </ProtectedRoute>
        ),
      },
      {
        path: "libraries/:libraryId/publishers/:id",
        element: (
          <ProtectedRoute
            checkLibraryAccess
            requiredPermission={ROUTE_PERMISSIONS.libraryBooks}
          >
            <PublisherDetail />
          </ProtectedRoute>
        ),
      },
    ],
  },
];

export const router = createBrowserRouter(routes);
