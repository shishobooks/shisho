import path from "path";

import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vitest/config";

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
      reportsDirectory: "./coverage",
      include: ["app/**/*.{ts,tsx}"],
      exclude: ["app/**/*.test.{ts,tsx}", "app/types/generated/**"],
    },
  },
});
