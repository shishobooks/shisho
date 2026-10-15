import fs from "fs";
import path from "path";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

const coverageDir = path.resolve(__dirname, "./coverage");
const coverageTmpDir = path.join(coverageDir, ".tmp");

// Vitest's coverage workers can race the initial temp-dir creation on short runs.
// Pre-creating the directory avoids intermittent ENOENT failures at write time.
fs.mkdirSync(coverageTmpDir, { recursive: true });

// Number and date formatting follow the host locale, and the tests expect
// en-US output ("18,307", "Jan"). Test workers start after this runs and
// inherit it, so a machine set to another locale gets the same results.
process.env.LANG = "en_US.UTF-8";
process.env.LC_ALL = "en_US.UTF-8";

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
    // Forks, not threads: threads share this process's locale, which was
    // fixed before the pin above ran.
    pool: "forks",
    include: ["app/**/*.test.{ts,tsx}", "website/*.test.ts"],
    // `mise check:quiet` runs this suite alongside the Go tests, both linters,
    // and the browser e2e pipelines, which pegs the CPU. Heavy jsdom tests take
    // 2-3s on an idle machine, so vitest's 5s default let a different one time
    // out on each loaded run. This only delays detection of a truly hung test.
    testTimeout: 15_000,
    coverage: {
      // CI neither uploads nor checks the report, so it skips the overhead.
      enabled: !process.env.CI,
      provider: "v8",
      reporter: ["text-summary", "lcov", "html"],
      reportsDirectory: coverageDir,
      include: ["app/**/*.{ts,tsx}"],
      exclude: ["app/**/*.test.{ts,tsx}", "app/types/generated/**"],
    },
  },
});
