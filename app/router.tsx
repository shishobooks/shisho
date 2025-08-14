import { createBrowserRouter } from "react-router-dom";

import BookDetail from "@/components/pages/BookDetail";
import Home from "@/components/pages/Home";
import Root from "@/components/pages/Root";

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
    ],
  },
]);
