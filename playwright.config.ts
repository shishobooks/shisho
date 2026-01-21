import fs from "fs";
import net from "net";
import os from "os";
import path from "path";

import { defineConfig } from "@playwright/test";

// =============================================================================
// Browser Configuration
// =============================================================================
// Add new browsers here - each browser gets its own isolated server and database.
// Tests within a browser run serially, but different browsers run in parallel.

const BROWSERS = ["chromium", "firefox"] as const;
type BrowserName = (typeof BROWSERS)[number];

// =============================================================================
// Port Allocation
// =============================================================================

/**
 * Find an available port by briefly binding to port 0 (OS assigns a free port).
 */
function findAvailablePort(): Promise<number> {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      const port = (server.address() as net.AddressInfo).port;
      server.close(() => resolve(port));
    });
  });
}

// =============================================================================
// Per-Browser Configuration
// =============================================================================

interface BrowserConfig {
  browser: BrowserName;
  apiPort: number;
  frontendPort: number;
  tmpDir: string;
  dbPath: string;
  cacheDir: string;
}

interface E2EConfig {
  browsers: BrowserConfig[];
}

/**
 * Get or create E2E configuration for all browsers.
 * Uses a file keyed by a unique ID to ensure all Playwright workers share
 * the same ports and temp directories.
 */
async function getOrCreateE2EConfig(): Promise<E2EConfig> {
  const configKey = process.env.E2E_CONFIG_KEY || String(process.pid);
  process.env.E2E_CONFIG_KEY = configKey;

  const configFile = path.join(
    os.tmpdir(),
    `shisho-e2e-config-${configKey}.json`,
  );

  if (fs.existsSync(configFile)) {
    return JSON.parse(fs.readFileSync(configFile, "utf-8"));
  }

  // First invocation - allocate ports and create temp directories for each browser
  const browsers = await Promise.all(
    BROWSERS.map(async (browser): Promise<BrowserConfig> => {
      const apiPort = await findAvailablePort();
      const frontendPort = await findAvailablePort();
      const tmpDir = fs.mkdtempSync(
        path.join(os.tmpdir(), `shisho-e2e-${browser}-`),
      );
      const dbPath = path.join(tmpDir, "data.sqlite");
      const cacheDir = path.join(tmpDir, "cache");

      // Ensure cache directory exists
      fs.mkdirSync(cacheDir, { recursive: true });

      return { browser, apiPort, frontendPort, tmpDir, dbPath, cacheDir };
    }),
  );

  const config: E2EConfig = { browsers };
  fs.writeFileSync(configFile, JSON.stringify(config));
  return config;
}

// =============================================================================
// Generate Playwright Configuration
// =============================================================================

const e2eConfig = await getOrCreateE2EConfig();

// Store all browser configs in environment for test access
// Tests will look up their browser's API port using testInfo.project.name
process.env.E2E_BROWSER_CONFIGS = JSON.stringify(
  Object.fromEntries(
    e2eConfig.browsers.map((b) => [b.browser, { apiPort: b.apiPort }]),
  ),
);

interface WebServerConfig {
  command: string;
  url: string;
  reuseExistingServer: boolean;
  timeout: number;
  env: Record<string, string>;
}

/**
 * Generate webServer entries for a browser (API + frontend).
 */
function createWebServers(config: BrowserConfig): WebServerConfig[] {
  // Allow reusing servers started by a concurrent run
  // (yarn test:e2e runs chromium and firefox in parallel, both try to start all servers)
  const reuseExistingServer = true;

  return [
    {
      command: "make start:api",
      url: `http://localhost:${config.apiPort}/health`,
      reuseExistingServer,
      timeout: 60000,
      env: {
        DATABASE_FILE_PATH: config.dbPath,
        SERVER_PORT: String(config.apiPort),
        CACHE_DIR: config.cacheDir,
        JWT_SECRET: `e2e-test-secret-${config.browser}`,
        ENVIRONMENT: "test", // Enables test-only API endpoints
      },
    },
    {
      command: `yarn start --port ${config.frontendPort}`,
      url: `http://localhost:${config.frontendPort}`,
      reuseExistingServer,
      timeout: 30000,
      env: {
        API_PORT: String(config.apiPort),
      },
    },
  ];
}

/**
 * Generate a Playwright project for a browser.
 */
function createProject(config: BrowserConfig) {
  return {
    name: config.browser,
    use: {
      browserName: config.browser,
      baseURL: `http://localhost:${config.frontendPort}`,
    },
  };
}

// Check if a specific project was requested via --project flag
function getRequestedProject(): string | null {
  const projectIndex = process.argv.findIndex((arg) => arg === "--project");
  if (projectIndex !== -1 && process.argv[projectIndex + 1]) {
    return process.argv[projectIndex + 1];
  }
  const projectArg = process.argv.find((arg) => arg.startsWith("--project="));
  if (projectArg) {
    return projectArg.split("=")[1];
  }
  return null;
}

const requestedProject = getRequestedProject();

// Only start servers for the requested project, or all if none specified
const browsersToStart = requestedProject
  ? e2eConfig.browsers.filter((b) => b.browser === requestedProject)
  : e2eConfig.browsers;

const webServers = browsersToStart.flatMap(createWebServers);
const projects = e2eConfig.browsers.map(createProject);

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  retries: 0,
  // Single worker ensures all tests run sequentially, avoiding database
  // race conditions between test files. Browsers run one after another.
  workers: 1,
  use: {
    trace: "on-first-retry",
  },
  projects,
  webServer: webServers,
});
