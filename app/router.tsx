import { createBrowserRouter } from "react-router-dom";

import BookDetail from "@/components/pages/BookDetail";
import Config from "@/components/pages/Config";
import Home from "@/components/pages/Home";
import LibraryList from "@/components/pages/LibraryList";
import LibraryRedirect from "@/components/pages/LibraryRedirect";
import LibrarySettings from "@/components/pages/LibrarySettings";
import Root from "@/components/pages/Root";
import SeriesDetail from "@/components/pages/SeriesDetail";
import SeriesList from "@/components/pages/SeriesList";

export const router = createBrowserRouter([
  {
    path: "/",
    Component: Root,
    children: [
      {
        path: "",
        Component: LibraryRedirect,
      },
      {
        path: "libraries",
        Component: LibraryList,
      },
      {
        path: "libraries/:libraryId",
        Component: Home,
      },
      {
        path: "libraries/:libraryId/settings",
        Component: LibrarySettings,
      },
      {
        path: "libraries/:libraryId/books/:id",
        Component: BookDetail,
      },
      {
        path: "libraries/:libraryId/series",
        Component: SeriesList,
      },
      {
        path: "libraries/:libraryId/series/:id",
        Component: SeriesDetail,
      },
      {
        path: "config",
        Component: Config,
      },
    ],
  },
]);
