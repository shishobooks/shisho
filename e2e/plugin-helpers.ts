import type { APIRequestContext, Page } from "@playwright/test";

import { expect } from "./fixtures";

export const PLUGIN_TEST_USERNAME = "plugintest";
export const PLUGIN_TEST_PASSWORD = "password123";

export interface FixtureInfo {
  scope: string;
  id: string;
  version: string;
  download_url: string;
  sha256: string;
}

export async function getFixtureInfo(
  api: APIRequestContext,
): Promise<FixtureInfo> {
  const resp = await api.get("/test/plugins/fixture-info");
  expect(resp.ok()).toBeTruthy();
  return resp.json();
}

export interface SeedPluginBody {
  scope: string;
  id: string;
  name?: string;
  version?: string;
  status?: number; // 0 active, -1 disabled, -2 malfunctioned, -3 unsupported
  update_available_version?: string;
  repository_scope?: string;
  repository_url?: string;
  skip_load?: boolean;
}

export async function seedPlugin(
  api: APIRequestContext,
  body: SeedPluginBody,
): Promise<void> {
  const resp = await api.post("/test/plugins", { data: body });
  expect(resp.status()).toBe(201);
}

export async function clearPlugins(api: APIRequestContext): Promise<void> {
  const resp = await api.delete("/test/plugins?include_official=true");
  expect(resp.ok()).toBeTruthy();
}

export async function loginApi(api: APIRequestContext): Promise<void> {
  const resp = await api.post("/auth/login", {
    data: {
      username: PLUGIN_TEST_USERNAME,
      password: PLUGIN_TEST_PASSWORD,
    },
  });
  expect(resp.ok()).toBeTruthy();
}

export async function loginAsPluginAdmin(page: Page): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Username").fill(PLUGIN_TEST_USERNAME);
  await page.getByLabel("Password").fill(PLUGIN_TEST_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL("/settings/libraries");
}

export async function resetAndLogin(
  api: APIRequestContext,
  page: Page,
): Promise<void> {
  await clearPlugins(api);
  await api.delete("/test/ereader");
  await api.delete("/test/users");
  await api.post("/test/users", {
    data: {
      username: PLUGIN_TEST_USERNAME,
      password: PLUGIN_TEST_PASSWORD,
    },
  });
  await loginAsPluginAdmin(page);
}
