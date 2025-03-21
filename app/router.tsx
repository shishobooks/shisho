import { createBrowserRouter } from "react-router-dom";

import Root from "@/components/pages/Root";

export const router = createBrowserRouter([
  {
    path: "/",
    Component: Root,
  },
]);
