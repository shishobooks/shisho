/**
 * E2E tests for the login flow.
 *
 * beforeAll creates a test user (testadmin/password123) before tests run.
 *
 * Running:
 *   yarn test:e2e                    # Run all E2E tests
 *   yarn test:e2e e2e/login.spec.ts  # Run only login tests
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";

test.describe("Login Flow", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/test/users");
    await apiContext.post("/test/users", {
      data: { username: "testadmin", password: "password123" },
    });
    await apiContext.dispose();
  });

  test("shows login form", async ({ page }) => {
    await page.goto("/login");
    await expect(
      page.getByText("Sign in to access your library"),
    ).toBeVisible();
    await expect(page.getByLabel("Username")).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("shows validation error for empty fields", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(
      page.getByText("Please enter both username and password"),
    ).toBeVisible();
    await expect(page).toHaveURL("/login");
  });

  test("shows validation error for empty password", async ({ page }) => {
    await page.goto("/login");
    await page.getByLabel("Username").fill("testadmin");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(
      page.getByText("Please enter both username and password"),
    ).toBeVisible();
    await expect(page).toHaveURL("/login");
  });

  test("shows validation error for empty username", async ({ page }) => {
    await page.goto("/login");
    await page.getByLabel("Password").fill("password123");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(
      page.getByText("Please enter both username and password"),
    ).toBeVisible();
    await expect(page).toHaveURL("/login");
  });

  test("shows error for invalid credentials", async ({ page }) => {
    await page.goto("/login");
    await page.getByLabel("Username").fill("wronguser");
    await page.getByLabel("Password").fill("wrongpassword");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(
      page.getByText(/Login failed|Invalid|credentials/i),
    ).toBeVisible();
    await expect(page).toHaveURL("/login");
  });

  test("logs in successfully with valid credentials", async ({ page }) => {
    await page.goto("/login");
    await page.getByLabel("Username").fill("testadmin");
    await page.getByLabel("Password").fill("password123");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page).toHaveURL("/settings/libraries");
  });
});
