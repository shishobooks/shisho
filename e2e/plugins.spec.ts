/**
 * E2E tests for the redesigned Plugins admin UI (/settings/plugins).
 *
 * Covers UI-structural behavior that does not require plugin seeding:
 *   - Unauthenticated access redirects to /login
 *   - Installed/Discover tabs reflect the URL and update it on click
 *   - Legacy routes (/browse, /order, /repositories) redirect to the new homes
 *   - Gear button opens the Advanced dialog
 *   - Empty-state copy renders with no plugins installed
 *
 * Flows that require plugin seeding (install, update, uninstall, enable-toggle,
 * local-scope reload icon) are intentionally out of scope: the test-only API
 * has no /test/plugins endpoint, and real plugin installs require network
 * fetches from GitHub. See the Notion follow-up task for the deferred suite.
 *
 * Running:
 *   pnpm test:e2e                        # Run all E2E tests
 *   pnpm test:e2e e2e/plugins.spec.ts    # Run only plugins tests
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";
import type { Page } from "@playwright/test";

async function loginAsAdmin(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Username").fill("plugintest");
  await page.getByLabel("Password").fill("password123");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL("/settings/libraries");
}

test.describe("Plugins UI (redesigned)", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/test/ereader");
    await apiContext.delete("/test/users");
    await apiContext.post("/test/users", {
      data: { username: "plugintest", password: "password123" },
    });
    await apiContext.dispose();
  });

  test("redirects unauthenticated users to /login", async ({ page }) => {
    await page.goto("/settings/plugins");
    await expect(page).toHaveURL(/\/login/);
  });

  test("Installed tab is the default view", async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto("/settings/plugins");

    await expect(page.getByRole("heading", { name: "Plugins" })).toBeVisible();

    const installedTrigger = page.getByRole("tab", { name: /Installed/ });
    const discoverTrigger = page.getByRole("tab", { name: "Discover" });
    await expect(installedTrigger).toHaveAttribute("aria-selected", "true");
    await expect(discoverTrigger).toHaveAttribute("aria-selected", "false");
  });

  test("clicking Discover updates URL and selected tab", async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto("/settings/plugins");

    await page.getByRole("tab", { name: "Discover" }).click();

    await expect(page).toHaveURL(/\/settings\/plugins\/discover$/);
    await expect(page.getByRole("tab", { name: "Discover" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  test("clicking Installed from Discover returns to clean URL", async ({
    page,
  }) => {
    await loginAsAdmin(page);
    await page.goto("/settings/plugins/discover");

    await page.getByRole("tab", { name: /Installed/ }).click();

    await expect(page).toHaveURL(/\/settings\/plugins$/);
    await expect(page.getByRole("tab", { name: /Installed/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  test("legacy /browse redirects to /discover", async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto("/settings/plugins/browse");

    await expect(page).toHaveURL(/\/settings\/plugins\/discover$/);
  });

  test("legacy /order opens Advanced dialog with Order tab active", async ({
    page,
  }) => {
    await loginAsAdmin(page);
    await page.goto("/settings/plugins/order");

    const dialog = page.getByRole("dialog", {
      name: "Advanced plugin settings",
    });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("tab", { name: "Order" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    // The ?advanced= query param is cleared by the mount effect.
    await expect(page).toHaveURL(/\/settings\/plugins$/);
  });

  test("legacy /repositories opens Advanced dialog with Repositories tab active", async ({
    page,
  }) => {
    await loginAsAdmin(page);
    await page.goto("/settings/plugins/repositories");

    const dialog = page.getByRole("dialog", {
      name: "Advanced plugin settings",
    });
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByRole("tab", { name: "Repositories" }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(page).toHaveURL(/\/settings\/plugins$/);
  });

  test("gear button opens Advanced dialog with Order and Repositories tabs", async ({
    page,
  }) => {
    await loginAsAdmin(page);
    await page.goto("/settings/plugins");

    await page
      .getByRole("button", { name: "Advanced plugin settings" })
      .click();

    const dialog = page.getByRole("dialog", {
      name: "Advanced plugin settings",
    });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("tab", { name: "Order" })).toBeVisible();
    await expect(
      dialog.getByRole("tab", { name: "Repositories" }),
    ).toBeVisible();
  });

  test("Installed tab shows empty state when no plugins installed", async ({
    page,
  }) => {
    await loginAsAdmin(page);
    await page.goto("/settings/plugins");

    await expect(
      page.getByText(
        "No plugins installed yet. Browse available plugins to get started.",
      ),
    ).toBeVisible();
  });
});
