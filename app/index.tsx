import { QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router-dom";

import ThemeProvider from "@/components/contexts/Theme/ThemeProvider";
import { queryClient } from "@/libraries/query-client";
import { router } from "@/router";

const container = document.getElementById("root");
const root = createRoot(container!);

root.render(
  <StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
);
