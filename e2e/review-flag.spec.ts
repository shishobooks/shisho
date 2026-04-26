/**
 * E2E tests for the Review Flag feature.
 *
 * Covers:
 * 1. A book missing reviewed status appears in the "Needs review" filter and
 *    drops out when marked reviewed via API.
 * 2. Manually toggling a complete book back to "needs review" keeps it in
 *    the queue even though it was previously marked reviewed.
 * 3. Bulk-mark-reviewed via multi-select cascades to all selected books.
 *
 * Running:
 *   pnpm e2e:chromium e2e/review-flag.spec.ts   # Chromium only
 *   mise test:e2e -- review-flag                 # Both browsers
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

interface TestData {
  libraryId: number;
}

let testData: TestData;

const USERNAME = "reviewtest";
const PASSWORD = "password123";

test.describe("Review flag", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });

    // Clean slate
    await apiContext.delete("/test/ereader");
    await apiContext.delete("/test/users");

    await apiContext.post("/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });

    const libraryResp = await apiContext.post("/test/libraries", {
      data: { name: "Review Flag Test Library" },
    });
    const library = (await libraryResp.json()) as { id: number };

    testData = { libraryId: library.id };
    await apiContext.dispose();
  });

  test.afterAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/test/ereader");
    await apiContext.delete("/test/users");
    await apiContext.dispose();
  });

  // Helper: log in and navigate to the test library gallery.
  async function loginAndOpenLibrary(page: Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
    await page.goto(`/libraries/${testData.libraryId}`, {
      waitUntil: "domcontentloaded",
    });
    // Wait for the gallery to resolve.
    await expect(page.getByRole("button", { name: "Select" })).toBeVisible();
  }

  test("book with unreviewed file shows in Needs review filter and drops out when marked reviewed", async ({
    page,
    apiContext,
  }) => {
    // Seed a book — new books have reviewed=null (needs review).
    const bookResp = await apiContext.post("/test/books", {
      data: {
        libraryId: testData.libraryId,
        title: "Unreviewed Book",
        fileType: "epub",
      },
    });
    const book = (await bookResp.json()) as { id: number; fileId: number };

    await loginAndOpenLibrary(page);

    // Navigate to the library with needs_review filter via URL (the filter is
    // URL-driven, so direct navigation is more reliable than UI interaction).
    await page.goto(
      `/libraries/${testData.libraryId}?reviewed_filter=needs_review`,
      { waitUntil: "domcontentloaded" },
    );
    await expect(page.getByRole("button", { name: "Select" })).toBeVisible();

    // The unreviewed book should appear in the filtered view.
    await expect(page.getByText("Unreviewed Book")).toBeVisible();

    // Mark the book's file as reviewed via the authenticated browser API.
    // page.request uses the logged-in session cookies.
    const patchResp = await page.request.patch(
      `/api/books/files/${book.fileId}/review`,
      { data: { override: "reviewed" } },
    );
    expect(patchResp.status()).toBe(200);

    // Reload the filtered view — the book should no longer appear.
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "Select" })).toBeVisible();
    await expect(page.getByText("Unreviewed Book")).not.toBeVisible();
  });

  test("manually toggling a reviewed book back to needs review keeps it in the queue", async ({
    page,
    apiContext,
  }) => {
    // Seed a book and immediately mark it reviewed.
    const bookResp = await apiContext.post("/test/books", {
      data: {
        libraryId: testData.libraryId,
        title: "Toggle Back Book",
        fileType: "epub",
      },
    });
    const book = (await bookResp.json()) as { id: number; fileId: number };

    await loginAndOpenLibrary(page);

    // Mark reviewed via authenticated session.
    let patchResp = await page.request.patch(
      `/api/books/files/${book.fileId}/review`,
      { data: { override: "reviewed" } },
    );
    expect(patchResp.status()).toBe(200);

    // Verify it does NOT appear in the needs_review filter.
    await page.goto(
      `/libraries/${testData.libraryId}?reviewed_filter=needs_review`,
      { waitUntil: "domcontentloaded" },
    );
    await expect(page.getByRole("button", { name: "Select" })).toBeVisible();
    await expect(page.getByText("Toggle Back Book")).not.toBeVisible();

    // Toggle back to unreviewed.
    patchResp = await page.request.patch(
      `/api/books/files/${book.fileId}/review`,
      { data: { override: "unreviewed" } },
    );
    expect(patchResp.status()).toBe(200);

    // Reload — the book should now appear in the needs_review filter.
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "Select" })).toBeVisible();
    await expect(page.getByText("Toggle Back Book")).toBeVisible();
  });

  test("bulk mark reviewed via multi-select removes all selected books from the needs review queue", async ({
    page,
    apiContext,
  }) => {
    // Seed two unreviewed books.
    await apiContext.post("/test/books", {
      data: {
        libraryId: testData.libraryId,
        title: "Bulk Book Alpha",
        fileType: "epub",
      },
    });
    await apiContext.post("/test/books", {
      data: {
        libraryId: testData.libraryId,
        title: "Bulk Book Beta",
        fileType: "epub",
      },
    });

    await loginAndOpenLibrary(page);

    // Navigate to the filtered view so only unreviewed books are visible.
    await page.goto(
      `/libraries/${testData.libraryId}?reviewed_filter=needs_review`,
      { waitUntil: "domcontentloaded" },
    );
    await expect(page.getByRole("button", { name: "Select" })).toBeVisible();

    // Both books should appear.
    await expect(page.getByText("Bulk Book Alpha")).toBeVisible();
    await expect(page.getByText("Bulk Book Beta")).toBeVisible();

    // Enter selection mode.
    await page.getByRole("button", { name: "Select" }).click();

    // Click both book cards to select them. In selection mode the card wrapper
    // has the click handler; pointer events on inner elements (title, link)
    // bubble up to it. Use the cover placeholder area (the first child of the
    // Link) to reliably target the card rather than the tooltip-wrapped title.
    // Using force:true tells Playwright to dispatch the event directly even
    // though a parent element intercepts it — the event bubbles to the card
    // wrapper which holds the selection handler.
    await page
      .locator(".group\\/card")
      .filter({ hasText: "Bulk Book Alpha" })
      .click({ force: true });
    await page
      .locator(".group\\/card")
      .filter({ hasText: "Bulk Book Beta" })
      .click({ force: true });

    // The selection toolbar should appear showing 2 selected.
    await expect(page.getByText(/2 selected/)).toBeVisible();

    // Open the "More" popover and click "Mark reviewed".
    await page.getByRole("button", { name: /^More$/ }).click();
    await page.getByText("Mark reviewed").click();

    // Wait for the toolbar to dismiss (selection mode exits after bulk action).
    await expect(page.getByText(/2 selected/)).not.toBeVisible();

    // The filtered view should now show neither book (they were marked reviewed).
    await expect(page.getByText("Bulk Book Alpha")).not.toBeVisible();
    await expect(page.getByText("Bulk Book Beta")).not.toBeVisible();
  });
});
