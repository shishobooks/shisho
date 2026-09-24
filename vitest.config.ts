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
    // `mise check:quiet` runs this suite alongside the Go tests, both linters,
    // and the browser e2e pipelines, which pegs the CPU. Heavy jsdom tests take
    // 2-3s on an idle machine, so vitest's 5s default let a different one time
    // out on each loaded run. This only delays detection of a truly hung test.
    testTimeout: 15_000,
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
