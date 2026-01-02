import path from "path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";

// https://vite.dev/config/
export default defineConfig({
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:3689",
        rewrite: (path) => path.replace(/^\/api/, ""),
        headers: {
          "X-Forwarded-Prefix": "/api",
        },
      },
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./app"),
    },
  },
  build: {
    outDir: "./build/app",
    emptyOutDir: true, // also necessary
  },
  clearScreen: false,
  plugins: [react(), tailwindcss()],
});
