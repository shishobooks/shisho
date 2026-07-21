import fs from "node:fs";
import net from "node:net";
import os from "node:os";
import path from "node:path";

import { defineConfig } from "@playwright/test";

const findAvailablePort = (): Promise<number> =>
  new Promise((resolve) => {
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (typeof address === "string" || address === null) {
        throw new Error("Unable to allocate a docs test port");
      }
      server.close(() => resolve(address.port));
    });
  });

const configKey = process.env.DOCS_E2E_CONFIG_KEY ?? String(process.pid);
process.env.DOCS_E2E_CONFIG_KEY = configKey;
const configPath = path.join(os.tmpdir(), `shisho-docs-e2e-${configKey}.json`);
const port = fs.existsSync(configPath)
  ? (JSON.parse(fs.readFileSync(configPath, "utf8")) as { port: number }).port
  : await findAvailablePort();
if (!fs.existsSync(configPath)) {
  fs.writeFileSync(configPath, JSON.stringify({ port }));
}

export default defineConfig({
  testDir: "./website/e2e",
  timeout: 30_000,
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    browserName: "chromium",
  },
  webServer: {
    command: `pnpm -C website build && pnpm -C website exec docusaurus serve --host 127.0.0.1 --port ${port} --no-open`,
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
