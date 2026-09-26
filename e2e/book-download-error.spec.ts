/**
 * E2E regression test for the book detail download error dialog.
 *
 * When a download fails, the dialog must show the message the API sent in its
 * { error: { message } } body, not a generic fallback. Seeded test books have
 * no file on disk, so the download endpoint fails with a real API error.
 *
 * Running:
 *   pnpm e2e:chromium e2e/book-download-error.spec.ts
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

interface TestData {
  libraryId: number;
}

let testData: TestData;

const USERNAME = "downloaderrortest";
const PASSWORD = "password123";

test.describe("Book detail download errors", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });

    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");

    await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });

    const libraryResp = await apiContext.post("/api/test/libraries", {
      data: { name: "Download Error Library" },
    });
    const library = (await libraryResp.json()) as { id: number };

    testData = { libraryId: library.id };
    await apiContext.dispose();
  });

  test.afterAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
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

  test("shows the API error message when a download fails", async ({
    page,
    apiContext,
  }) => {
    const bookResp = await apiContext.post("/api/test/books", {
      data: {
        libraryId: testData.libraryId,
        title: "Missing On Disk",
        fileType: "epub",
      },
    });
    const book = (await bookResp.json()) as { id: number };

    await login(page);
    await page.goto(`/libraries/${testData.libraryId}/books/${book.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("heading", { name: "Files (1)", exact: true }),
    ).toBeVisible();

    // The download button is icon-only (its label is a tooltip), and the file
    // row renders it in both the desktop and mobile layouts. Click the visible
    // one by its Lucide icon.
    await page
      .locator("button:has(svg.lucide-download)")
      .filter({ visible: true })
      .first()
      .click();

    const dialog = page.getByRole("dialog", {
      name: "Download Failed",
      exact: true,
    });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Source file not found on disk");
    await expect(dialog).not.toContainText("Failed to generate file");
  });
});
