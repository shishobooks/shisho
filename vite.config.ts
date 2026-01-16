import fs from "fs";
import path from "path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";

/**
 * Gets the API port with the following priority:
 * 1. API_PORT environment variable
 * 2. tmp/api.port file (written by the API server)
 * 3. Default port 3689
 */
function getApiPort(): number {
  // Check environment variable first
  if (process.env.API_PORT) {
    const port = parseInt(process.env.API_PORT, 10);
    if (!isNaN(port)) {
      return port;
    }
  }

  // Try reading from port file
  const portFilePath = path.resolve(__dirname, "tmp/api.port");
  try {
    if (fs.existsSync(portFilePath)) {
      const content = fs.readFileSync(portFilePath, "utf-8").trim();
      const port = parseInt(content, 10);
      if (!isNaN(port)) {
        return port;
      }
    }
  } catch {
    // Ignore errors, fall through to default
  }

  // Default port
  return 3689;
}

// https://vite.dev/config/
export default defineConfig({
  server: {
    host: "0.0.0.0",
    strictPort: false, // Allow Vite to auto-increment port if busy
    watch: {
      ignored: ["**/coverage/**"],
    },
    proxy: {
      "/api": {
        target: `http://localhost:${getApiPort()}`,
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
