/**
 * Custom Playwright test fixtures for E2E testing.
 *
 * Provides an `apiContext` fixture that automatically connects to the correct
 * API server based on the browser being tested. Each browser (chromium, firefox, etc.)
 * runs against its own isolated API server and database.
 *
 * Usage:
 *   import { expect, test } from "./fixtures";
 *
 *   test.beforeAll(async ({ apiContext }) => {
 *     await apiContext.delete("/test/users");
 *     await apiContext.post("/test/users", { data: { ... } });
 *   });
 */

import {
  test as base,
  request,
  type APIRequestContext,
} from "@playwright/test";

// Re-export expect and request for convenience
export { expect, request } from "@playwright/test";

interface BrowserApiConfig {
  apiPort: number;
}

/**
 * Get the API base URL for a browser. Use this in beforeAll hooks.
 *
 * @param browserName - The browser name (e.g., "chromium", "firefox")
 * @returns The API base URL (e.g., "http://localhost:12345")
 */
export function getApiBaseURL(browserName: string): string {
  const apiPort = getApiPortForBrowser(browserName);
  return `http://localhost:${apiPort}`;
}

/**
 * Get the API port for the current browser from environment.
 * The config is set by playwright.config.ts and keyed by browser name.
 */
function getApiPortForBrowser(browserName: string): number {
  const configJson = process.env.E2E_BROWSER_CONFIGS;
  if (!configJson) {
    throw new Error(
      "E2E_BROWSER_CONFIGS not set. Are you running tests via playwright.config.ts?",
    );
  }

  const configs: Record<string, BrowserApiConfig> = JSON.parse(configJson);
  const config = configs[browserName];

  if (!config) {
    throw new Error(
      `No API config found for browser "${browserName}". ` +
        `Available browsers: ${Object.keys(configs).join(", ")}`,
    );
  }

  return config.apiPort;
}

/**
 * Extended test fixtures with browser-aware API context.
 */
export const test = base.extend<{
  /**
   * API request context pre-configured with the correct base URL for the
   * current browser's API server. Use this in beforeAll/afterAll hooks
   * to set up test data.
   *
   * Note: This fixture creates a new context for each test. For beforeAll
   * hooks, use `apiContextFactory` instead.
   */
  apiContext: APIRequestContext;
  /**
   * Factory to create API request contexts. Use this in beforeAll hooks
   * since regular fixtures aren't available there.
   */
  apiContextFactory: () => Promise<APIRequestContext>;
}>({
  // eslint-disable-next-line no-empty-pattern
  apiContext: async ({}, use, testInfo) => {
    const browserName = testInfo.project.name;
    const apiPort = getApiPortForBrowser(browserName);
    const apiContext = await request.newContext({
      baseURL: `http://localhost:${apiPort}`,
    });
    await use(apiContext);
    await apiContext.dispose();
  },

  // eslint-disable-next-line no-empty-pattern
  apiContextFactory: async ({}, use, testInfo) => {
    const browserName = testInfo.project.name;
    const apiPort = getApiPortForBrowser(browserName);
    const contexts: APIRequestContext[] = [];

    const factory = async () => {
      const ctx = await request.newContext({
        baseURL: `http://localhost:${apiPort}`,
      });
      contexts.push(ctx);
      return ctx;
    };

    await use(factory);

    // Cleanup all created contexts
    for (const ctx of contexts) {
      await ctx.dispose();
    }
  },
});
