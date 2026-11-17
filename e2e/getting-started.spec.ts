/**
 * E2E tests for the Getting Started panel where real focus handling and menu
 * animations decide what happens, which the unit tests' jsdom can't show.
 *
 * Running:
 *   pnpm e2e:chromium e2e/getting-started.spec.ts
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "gsadmin";
const PASSWORD = "password123";

test.describe("Getting Started", () => {
  let userId: number;

  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    const resp = await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });
    userId = (await resp.json()).id;
    await apiContext.dispose();
  });

  async function login(page: Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/admin\/libraries/);
  }

  const panel = (page: Page) =>
    page.getByRole("dialog", { name: "Getting started" });

  test("the account menu's Getting started entry leaves the panel open", async ({
    page,
  }) => {
    await login(page);

    await page.getByRole("button", { name: "Account menu" }).click();
    await page
      .getByRole("menuitem", { name: "Getting started", exact: true })
      .click();

    // The menu animates out after the click; the panel must still be open
    // once it has gone.
    await expect(page.getByRole("menu")).toHaveCount(0);
    await expect(panel(page)).toBeVisible();
  });

  test("the panel that opens on its own stays open through the sign-in redirect", async ({
    browserName,
    page,
  }) => {
    // An admin's Show Getting Started, from another session, makes the panel
    // open on the user's next visit.
    const admin = await request.newContext({
      baseURL: getApiBaseURL(browserName),
    });
    await admin.post("/api/auth/login", {
      data: { username: USERNAME, password: PASSWORD },
    });
    const show = await admin.post(`/api/users/${userId}/show-getting-started`);
    expect(show.status()).toBe(204);
    await admin.dispose();

    // Signing in lands on `/`, which shows its own nav while it loads the
    // libraries and then redirects to a page with another. Hold the
    // libraries until that first nav has opened the panel, so the redirect
    // always comes after it.
    let release!: () => void;
    const released = new Promise<void>((resolve) => (release = resolve));
    await page.route(
      (url) => url.pathname === "/api/user/libraries",
      async (route) => {
        await released;
        await route.continue();
      },
    );
    const cleared = page.waitForRequest(
      (req) =>
        req.method() === "DELETE" &&
        new URL(req.url()).pathname === "/api/getting-started/auto-open",
    );
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await cleared;
    release();

    await expect(page).toHaveURL(/\/admin\/libraries/);
    await expect(panel(page)).toBeVisible();
  });
});
