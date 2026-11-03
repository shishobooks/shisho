/**
 * E2E tests for deleting a genre or tag that books still use: the confirmation
 * states the count, and the delete removes it from those books and keeps their
 * other entries. Unit tests mock the queries; these go through the real API.
 */

import { type Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "deleteinuse";
const PASSWORD = "password123";

let libraryId: number;
let bookIds: number[];

async function login(page: Page) {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByLabel("Username").fill(USERNAME);
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(/\/admin\/libraries|\/libraries\//, {
    waitUntil: "domcontentloaded",
  });
}

test.describe("Deleting a genre or tag still in use", () => {
  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });

    const library = await apiContext.post("/api/test/libraries", {
      data: { name: "Delete In Use Library" },
    });
    libraryId = ((await library.json()) as { id: number }).id;

    bookIds = [];
    for (const title of ["Harbor Lights", "Quiet Tides"]) {
      const book = await apiContext.post("/api/test/books", {
        data: { libraryId, title },
      });
      bookIds.push(((await book.json()) as { id: number }).id);
    }
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

  for (const { field, label, unwanted, kept } of [
    { field: "genres", label: "Genre", unwanted: "Audiobook", kept: "Fantasy" },
    { field: "tags", label: "Tag", unwanted: "Kindle", kept: "Favorite" },
  ]) {
    test(`removes a ${label.toLowerCase()} from every book and keeps the others`, async ({
      page,
    }) => {
      await login(page);

      for (const id of bookIds) {
        const saved = await page.request.post(`/api/books/${id}`, {
          data: { [field]: [unwanted, kept] },
        });
        expect(saved.ok()).toBe(true);
      }
      const list = await page.request.get(
        `/api/${field}?library_id=${libraryId}&search=${unwanted}`,
      );
      const { items } = (await list.json()) as { items: { id: number }[] };

      await page.goto(`/libraries/${libraryId}/${field}/${items[0].id}`, {
        waitUntil: "domcontentloaded",
      });
      await page.getByRole("button", { name: "Delete", exact: true }).click();

      const dialog = page.getByRole("dialog");
      await expect(dialog).toContainText(
        `"${unwanted}"? It will be removed from 2 books.`,
      );
      await dialog.getByRole("button", { name: "Delete", exact: true }).click();

      await page.waitForURL(new RegExp(`/libraries/${libraryId}/${field}$`));
      await expect(
        page.getByRole("link", { name: kept }).first(),
      ).toBeVisible();
      await expect(page.getByRole("link", { name: unwanted })).toHaveCount(0);

      await page.goto(`/libraries/${libraryId}/books/${bookIds[0]}`, {
        waitUntil: "domcontentloaded",
      });
      await expect(page.getByText(kept, { exact: true })).toBeVisible();
      await expect(page.getByText(unwanted, { exact: true })).toHaveCount(0);
    });
  }
});
