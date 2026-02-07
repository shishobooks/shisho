import fs from "fs";
import path from "path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";

// Read version from package.json
const packageJson = JSON.parse(
  fs.readFileSync(path.resolve(__dirname, "package.json"), "utf-8"),
);
const appVersion = packageJson.version || "dev";

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
      ignored: ["**/coverage/**", "**/website/**"],
    },
    proxy: {
      "/api": {
        target: `http://localhost:${getApiPort()}`,
        rewrite: (path) => path.replace(/^\/api/, ""),
        headers: {
          "X-Forwarded-Prefix": "/api",
        },
      },
      // eReader browser UI routes (API key auth for stock browser support)
      "/ereader": {
        target: `http://localhost:${getApiPort()}`,
      },
      // Short URL resolution for eReader setup
      "/e": {
        target: `http://localhost:${getApiPort()}`,
      },
      // Kobo sync routes (API key auth for Kobo device sync)
      "/kobo": {
        target: `http://localhost:${getApiPort()}`,
        // Don't change the Host header - we want to preserve the original
        changeOrigin: false,
        configure: (proxy) => {
          // Forward the original Host header to the backend so it can build correct URLs
          // The proxyReq event fires when the proxy request is about to be sent
          proxy.on("proxyReq", (proxyReq, req) => {
            const host = req.headers.host;
            if (host) {
              proxyReq.setHeader("X-Forwarded-Host", host);
            }
            // Also send the port Vite is listening on - some clients (like Kobo)
            // don't include the port in the Host header even for non-standard ports
            const localPort = (req.socket as { localPort?: number }).localPort;
            if (localPort) {
              proxyReq.setHeader("X-Forwarded-Port", String(localPort));
            }
          });
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
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
});
