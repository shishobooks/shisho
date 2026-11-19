/**
 * E2E tests for the admin account setup flow.
 *
 * These tests verify the first-time admin setup experience.
 * beforeAll clears users to ensure setup page is accessible.
 *
 * Running:
 *   pnpm test:e2e                    # Run all E2E tests
 *   pnpm test:e2e e2e/setup.spec.ts  # Run only setup tests
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

async function gotoSetup(page: Page, path = "/setup") {
  await page.goto(path, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveURL("/setup");
  await expect(page.getByRole("heading", { name: "Welcome!" })).toBeVisible();
}

test.describe("Setup Flow", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.dispose();
  });

  test("redirects to setup when no users exist", async ({ page }) => {
    await gotoSetup(page, "/");
    await expect(
      page.getByText("Create your admin account to get started"),
    ).toBeVisible();
  });

  test("shows validation error for password mismatch", async ({ page }) => {
    await gotoSetup(page);
    await page.getByLabel("Username").fill("testadmin");
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("different456");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(page.getByText("Passwords do not match")).toBeVisible();
    await expect(page).toHaveURL("/setup");
  });

  test("shows validation error for username too short", async ({ page }) => {
    await gotoSetup(page);
    await page.getByLabel("Username").fill("ab");
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("password123");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(
      page.getByText("Username must be at least 3 characters"),
    ).toBeVisible();
    await expect(page).toHaveURL("/setup");
  });

  test("shows validation error for password too short", async ({ page }) => {
    await gotoSetup(page);
    await page.getByLabel("Username").fill("testadmin");
    await page.getByLabel("Password", { exact: true }).fill("short");
    await page.getByLabel("Confirm Password").fill("short");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(
      page.getByText("Password must be at least 8 characters"),
    ).toBeVisible();
    await expect(page).toHaveURL("/setup");
  });

  test("shows validation error for empty username", async ({ page }) => {
    await gotoSetup(page);
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("password123");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(page.getByText("Username is required")).toBeVisible();
    await expect(page).toHaveURL("/setup");
  });

  test("shows validation error for empty password", async ({ page }) => {
    await gotoSetup(page);
    await page.getByLabel("Username").fill("testadmin");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(page.getByText("Password is required")).toBeVisible();
    await expect(page).toHaveURL("/setup");
  });

  test("creating the admin account continues to the library step", async ({
    page,
  }) => {
    await gotoSetup(page);
    await page.getByLabel("Username").fill("testadmin");
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("password123");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(page).toHaveURL("/setup/library");
    await expect(
      page.getByRole("heading", { name: "Where are your books?" }),
    ).toBeVisible();
  });
});

test.describe("First run", () => {
  let mediaPath: string;

  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    const resp = await apiContext.post("/api/test/media-folder", {
      data: {
        books: [
          { title: "First Run Book", fileType: "epub" },
          { title: "First Run Comic", fileType: "cbz" },
        ],
      },
    });
    expect(resp.status()).toBe(201);
    mediaPath = (await resp.json()).path;
    await apiContext.dispose();
  });

  test.afterAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.dispose();
  });

  test("goes from the account through Setup to the scanning library", async ({
    page,
  }) => {
    // The enrichment step lists the official repository's plugins. Keep the
    // test off the network with a stand-in for it.
    await page.route(
      (url) => url.pathname === "/api/plugins/available",
      (route) =>
        route.fulfill({
          json: [
            {
              scope: "shisho",
              id: "goodreads-enricher",
              name: "Goodreads Enricher",
              overview: "Fetches book metadata and covers from Goodreads",
              description: "",
              homepage: "",
              imageUrl: "",
              is_official: true,
              compatible: true,
              versions: [
                {
                  version: "1.0.0",
                  minShishoVersion: "0.0.0",
                  manifestVersion: 1,
                  releaseDate: "2026-01-01",
                  changelog: "",
                  downloadUrl: "",
                  sha256: "",
                  compatible: true,
                  capabilities: {
                    identifierTypes: [],
                    httpAccess: {
                      description: "",
                      domains: ["*.goodreads.com"],
                    },
                  },
                },
              ],
            },
          ],
        }),
    );

    // Account
    await gotoSetup(page);
    await page.getByLabel("Username").fill("firstadmin");
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("password123");
    await page.getByRole("button", { name: "Create Admin Account" }).click();

    // Library
    await expect(page).toHaveURL("/setup/library");
    await page.getByLabel("Library name").fill("First Library");
    await page
      .getByRole("textbox", { name: "Folder 1", exact: true })
      .fill(mediaPath);
    await expect(
      page.getByRole("checkbox", {
        name: "Organize file structure during scans",
      }),
    ).toBeChecked();
    await page.getByRole("button", { name: "Continue", exact: true }).click();

    // Enrichment, skipped
    await expect(page).toHaveURL("/setup/plugins");
    await expect(
      page.getByText("We found about 1 ebook and 1 comic."),
    ).toBeVisible();
    await page.getByRole("button", { name: "Skip this step" }).click();

    // The library, with its Books arriving and Getting Started in the nav
    await expect(page).toHaveURL(/\/libraries\/\d+/);
    await expect(page.getByText("First Run Book").first()).toBeVisible({
      timeout: 30_000,
    });
    await expect(
      page.getByRole("button", { name: /^Getting Started/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("dialog", { name: "Getting Started" }),
    ).toHaveCount(0);
  });
});
