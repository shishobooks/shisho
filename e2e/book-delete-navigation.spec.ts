/**
 * E2E tests for where deleting a book from its page leads: Back to the page
 * the book was opened from, or the book's own library when that page is gone
 * or unknown. The unit tests use a memory router; these check the browser's
 * history.
 */

import { type Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

interface TestData {
  libraryId: number;
  keptSeriesId: number;
  loneSeriesId: number;
  directBookId: number;
  fileOnlyFileId: number;
}

let testData: TestData;

test.describe("Deleting a book", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });

    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: "deleteadmin", password: "password123" },
    });

    // The book's library is not the first one, so landing on the first
    // library's gallery would be the wrong library.
    await apiContext.post("/api/test/libraries", {
      data: { name: "Another Library" },
    });
    const library = (await (
      await apiContext.post("/api/test/libraries", {
        data: { name: "Delete Test Library" },
      })
    ).json()) as { id: number };

    const createSeries = async (name: string) =>
      (
        (await (
          await apiContext.post("/api/test/series", {
            data: { libraryId: library.id, name },
          })
        ).json()) as { id: number }
      ).id;
    const seedBook = async (title: string, seriesId?: number) =>
      (await (
        await apiContext.post("/api/test/books", {
          data: {
            libraryId: library.id,
            title,
            seriesId,
            withFileOnDisk: true,
          },
        })
      ).json()) as { id: number; fileId: number };
    const createBook = async (title: string, seriesId?: number) =>
      (await seedBook(title, seriesId)).id;

    const keptSeriesId = await createSeries("Kept Saga");
    await createBook("Kept Saga One", keptSeriesId);
    await createBook("Kept Saga Two", keptSeriesId);
    const loneSeriesId = await createSeries("Lone Saga");
    await createBook("Lone Saga Only", loneSeriesId);
    await createBook("Gallery Book");
    const directBookId = await createBook("Direct Book");
    const fileOnly = await seedBook("File Page Book");

    testData = {
      libraryId: library.id,
      keptSeriesId,
      loneSeriesId,
      directBookId,
      fileOnlyFileId: fileOnly.fileId,
    };
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
    await page.getByLabel("Username").fill("deleteadmin");
    await page.getByLabel("Password").fill("password123");
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/admin\/libraries|\/libraries\//);
  }

  async function openBook(page: Page, title: string) {
    await page.getByRole("link").filter({ hasText: title }).first().click();
    await expect(page.getByRole("heading", { name: title })).toBeVisible();
  }

  async function deleteBook(page: Page) {
    await page.getByLabel("Book actions").click();
    await page.getByRole("menuitem", { name: "Delete book" }).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Delete", exact: true })
      .click();
  }

  test("returns to the gallery page with its sort", async ({ page }) => {
    await login(page);
    const gallery = `/libraries/${testData.libraryId}?sort=title%3Aasc`;
    await page.goto(gallery, { waitUntil: "domcontentloaded" });
    await openBook(page, "Gallery Book");

    await deleteBook(page);

    await expect(page).toHaveURL(gallery);
    await expect(
      page.getByRole("link").filter({ hasText: "Gallery Book" }),
    ).toHaveCount(0);
  });

  // Back passes the deleted book's page on the way to the gallery.
  test("returns to the gallery from the book's file page", async ({ page }) => {
    await login(page);
    const gallery = `/libraries/${testData.libraryId}?sort=title%3Aasc`;
    await page.goto(gallery, { waitUntil: "domcontentloaded" });
    await openBook(page, "File Page Book");
    await page
      .locator(`a[href$="/files/${testData.fileOnlyFileId}"]`)
      .first()
      .click();
    // The menu gains "Move to another book" once the library loads, which
    // moves "Delete file" under a click made before then.
    await expect(
      page.getByRole("button", { name: "Download EPUB", exact: true }),
    ).toBeEnabled();
    await page.getByLabel("File actions").click();
    await expect(
      page.getByRole("menuitem", { name: "Move to another book" }),
    ).toBeVisible();
    await page.getByRole("menuitem", { name: "Delete file" }).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Delete", exact: true })
      .click();

    await expect(page).toHaveURL(gallery);
  });

  test("returns to a series page that still has books", async ({ page }) => {
    await login(page);
    const seriesPage = `/libraries/${testData.libraryId}/series/${testData.keptSeriesId}`;
    await page.goto(seriesPage, { waitUntil: "domcontentloaded" });
    await openBook(page, "Kept Saga One");

    await deleteBook(page);

    await expect(page).toHaveURL(seriesPage);
    await expect(page.getByText("Kept Saga Two").first()).toBeVisible();
  });

  test("goes to the library when the delete removed the series", async ({
    page,
  }) => {
    await login(page);
    await page.goto(
      `/libraries/${testData.libraryId}/series/${testData.loneSeriesId}`,
      { waitUntil: "domcontentloaded" },
    );
    await openBook(page, "Lone Saga Only");

    await deleteBook(page);

    await expect(page).toHaveURL(`/libraries/${testData.libraryId}`);
  });

  test("goes to the library when the book was opened directly", async ({
    page,
  }) => {
    await login(page);
    await page.goto(
      `/libraries/${testData.libraryId}/books/${testData.directBookId}`,
      { waitUntil: "domcontentloaded" },
    );
    await expect(
      page.getByRole("heading", { name: "Direct Book" }),
    ).toBeVisible();

    await deleteBook(page);

    await expect(page).toHaveURL(`/libraries/${testData.libraryId}`);
  });
});
