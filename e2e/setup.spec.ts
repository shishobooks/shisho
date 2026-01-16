/**
 * E2E tests for the admin account setup flow.
 *
 * These tests verify the first-time admin setup experience.
 * beforeAll clears users to ensure setup page is accessible.
 *
 * Running:
 *   yarn test:e2e                    # Run all E2E tests
 *   yarn test:e2e e2e/setup.spec.ts  # Run only setup tests
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";

test.describe("Setup Flow", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/test/users");
    await apiContext.dispose();
  });

  test("redirects to setup when no users exist", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL("/setup");
    await expect(page.getByRole("heading", { name: "Welcome!" })).toBeVisible();
    await expect(
      page.getByText("Create your admin account to get started"),
    ).toBeVisible();
  });

  test("shows validation error for password mismatch", async ({ page }) => {
    await page.goto("/setup");
    await page.getByLabel("Username").fill("testadmin");
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("different456");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(page.getByText("Passwords do not match")).toBeVisible();
    await expect(page).toHaveURL("/setup");
  });

  test("shows validation error for username too short", async ({ page }) => {
    await page.goto("/setup");
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
    await page.goto("/setup");
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
    await page.goto("/setup");
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("password123");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(page.getByText("Username is required")).toBeVisible();
    await expect(page).toHaveURL("/setup");
  });

  test("shows validation error for empty password", async ({ page }) => {
    await page.goto("/setup");
    await page.getByLabel("Username").fill("testadmin");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(page.getByText("Password is required")).toBeVisible();
    await expect(page).toHaveURL("/setup");
  });

  test("creates admin account successfully", async ({ page }) => {
    await page.goto("/setup");
    await expect(page.getByRole("heading", { name: "Welcome!" })).toBeVisible();
    await page.getByLabel("Username").fill("testadmin");
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("password123");
    await page.getByRole("button", { name: "Create Admin Account" }).click();
    await expect(page).toHaveURL("/settings/libraries");
    await expect(page.getByText("testadmin")).toBeVisible();
  });
});
