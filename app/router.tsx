import { createBrowserRouter } from "react-router-dom";

import BookDetail from "@/components/pages/BookDetail";
import Home from "@/components/pages/Home";
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
        Component: Home,
      },
      {
        path: "books/:id",
        Component: BookDetail,
      },
      {
        path: "series",
        Component: SeriesList,
      },
      {
        path: "series/:name",
        Component: SeriesDetail,
      },
    ],
  },
]);
