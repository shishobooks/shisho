import fs from "fs";
import path from "path";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

const coverageDir = path.resolve(__dirname, "./coverage");
const coverageTmpDir = path.join(coverageDir, ".tmp");

// Vitest's coverage workers can race the initial temp-dir creation on short runs.
// Pre-creating the directory avoids intermittent ENOENT failures at write time.
fs.mkdirSync(coverageTmpDir, { recursive: true });

export default defineConfig({
  plugins: [react() as never],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./app"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["app/**/*.test.{ts,tsx}"],
    coverage: {
      enabled: true,
      provider: "v8",
      reporter: ["text-summary", "lcov", "html"],
      reportsDirectory: coverageDir,
      include: ["app/**/*.{ts,tsx}"],
      exclude: ["app/**/*.test.{ts,tsx}", "app/types/generated/**"],
    },
  },
});
