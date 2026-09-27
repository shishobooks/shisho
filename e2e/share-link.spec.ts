/**
 * E2E happy path for Share Links: an admin turns sharing on, creates a link on
 * a book and copies it, and a browser context with no session opens the link,
 * sees the book, and downloads a file. Turning sharing off then shows the
 * unavailable page.
 *
 * This is the only test of the anonymous browser path end to end. It runs in
 * Chromium only because it reads the copied URL back from the clipboard, and
 * Playwright grants clipboard permissions only in Chromium.
 *
 * Running:
 *   pnpm e2e:chromium e2e/share-link.spec.ts
 */

import type { APIRequestContext, Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "sharelinkadmin";
const PASSWORD = "password123";
const TITLE = "Shared Over The Wire";

let libraryId: number;
let bookId: number;

// PUT /api/settings/sharing needs an admin session.
async function setSharing(apiBaseURL: string, enabled: boolean) {
  const apiContext: APIRequestContext = await request.newContext({
    baseURL: apiBaseURL,
  });
  const login = await apiContext.post("/api/auth/login", {
    data: { username: USERNAME, password: PASSWORD },
  });
  expect(login.ok()).toBe(true);
  const put = await apiContext.put("/api/settings/sharing", {
    data: { enabled, require_expiration: false },
  });
  expect(put.ok()).toBe(true);
  await apiContext.dispose();
}

test.describe("Share Links", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });

    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });

    const libraryResp = await apiContext.post("/api/test/libraries", {
      data: { name: "Share Link Library" },
    });
    libraryId = ((await libraryResp.json()) as { id: number }).id;

    const bookResp = await apiContext.post("/api/test/books", {
      data: { libraryId, title: TITLE, withEpubOnDisk: true },
    });
    bookId = ((await bookResp.json()) as { id: number }).id;
    await apiContext.dispose();

    // Start from the default so the test turns sharing on through the UI.
    await setSharing(apiBaseURL, false);
  });

  test.afterAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    await setSharing(apiBaseURL, false);
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.dispose();
  });

  async function login(page: Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
  }

  test("a recipient with no session opens a link and downloads the book", async ({
    browser,
    browserName,
    context,
    page,
  }) => {
    test.skip(
      browserName !== "chromium",
      "Clipboard permissions are Chromium-only in Playwright.",
    );
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);

    await login(page);

    // Turn sharing on from Settings > Sharing.
    await page.goto("/settings/sharing", { waitUntil: "domcontentloaded" });
    const enable = page.getByRole("switch", { name: "Enable Share Links" });
    await expect(enable).not.toBeChecked();
    await enable.click();
    await expect(enable).toBeChecked();

    // Create a link from the book's action menu and copy it.
    await page.goto(`/libraries/${libraryId}/books/${bookId}`, {
      waitUntil: "domcontentloaded",
    });
    await page.getByLabel("Book actions").click();
    await page.getByRole("menuitem", { name: "Share", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Share", exact: true });
    await dialog.getByLabel("Label (optional)").fill("for the e2e test");
    await dialog.getByRole("button", { name: "Create link" }).click();
    const row = dialog.getByRole("listitem").filter({
      hasText: "for the e2e test",
    });
    await expect(row).toContainText("active");
    await expect(row).toContainText(`Created by ${USERNAME}`);
    await row
      .getByRole("button", { name: "Copy link for the e2e test" })
      .click();

    const url = await page.evaluate(() => navigator.clipboard.readText());
    const origin = new URL(page.url()).origin;
    expect(url).toMatch(new RegExp(`^${origin}/share/[A-Za-z0-9_-]{43}$`));

    // A fresh context has no session cookie.
    const anonymous = await browser.newContext();
    try {
      const recipient = await anonymous.newPage();
      await recipient.goto(url, { waitUntil: "domcontentloaded" });

      await expect(
        recipient.getByRole("heading", { name: TITLE }),
      ).toBeVisible();
      await expect(recipient.getByText(`Shared by ${USERNAME}`)).toBeVisible();
      expect(recipient.url()).toBe(url);
      await expect(recipient.getByLabel("Book actions")).toHaveCount(0);
      await expect(recipient.getByRole("link")).toHaveCount(0);

      const downloadPromise = recipient.waitForEvent("download");
      await recipient
        .getByRole("button", { name: "Download", exact: true })
        .filter({ visible: true })
        .first()
        .click();
      const download = await downloadPromise;
      expect(download.suggestedFilename()).toMatch(/\.epub$/);
      expect(await download.failure()).toBeNull();

      // Turning sharing off makes the link unavailable.
      await setSharing(getApiBaseURL(browserName), false);
      await recipient.reload({ waitUntil: "domcontentloaded" });
      await expect(
        recipient.getByRole("heading", {
          name: "This link is no longer available",
        }),
      ).toBeVisible();
    } finally {
      await anonymous.close();
    }
  });

  test("the recipient page fits a phone-width screen", async ({
    browser,
    browserName,
    page,
  }) => {
    test.skip(browserName !== "chromium", "Covered by the Chromium run.");
    await login(page);
    await setSharing(getApiBaseURL(browserName), true);
    const create = await page.request.post(`/api/books/${bookId}/share-links`, {
      data: {},
    });
    expect(create.ok()).toBe(true);
    const { token } = (await create.json()) as { token: string };

    // Contexts made in a test do not inherit the config's baseURL.
    const anonymous = await browser.newContext({
      baseURL: new URL(page.url()).origin,
      viewport: { width: 390, height: 844 },
    });
    try {
      const recipient = await anonymous.newPage();
      await recipient.goto(`/share/${token}`, {
        waitUntil: "domcontentloaded",
      });
      await expect(
        recipient.getByRole("heading", { name: TITLE }),
      ).toBeVisible();
      const overflow = await recipient.evaluate(
        () =>
          document.documentElement.scrollWidth -
          document.documentElement.clientWidth,
      );
      expect(overflow).toBeLessThanOrEqual(0);
      await expect(
        recipient
          .getByRole("button", { name: "Download", exact: true })
          .filter({ visible: true }),
      ).not.toHaveCount(0);
    } finally {
      await anonymous.close();
      await setSharing(getApiBaseURL(browserName), false);
    }
  });
});
