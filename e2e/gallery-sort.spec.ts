/**
 * E2E tests for the gallery sort feature.
 *
 * Tests the sort toolbar button, sort sheet, sorted-by chip row,
 * URL-driven sort state, and save-as-default behavior.
 *
 * Running:
 *   pnpm test:e2e                              # Run all E2E tests
 *   pnpm test:e2e e2e/gallery-sort.spec.ts     # Run only gallery sort tests
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";

interface TestData {
  libraryId: number;
}

let testData: TestData;

test.describe("Gallery sort", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });

    await apiContext.delete("/test/ereader");
    await apiContext.delete("/test/users");

    await apiContext.post("/test/users", {
      data: { username: "sortadmin", password: "password123" },
    });

    const libraryResp = await apiContext.post("/test/libraries", {
      data: { name: "Sort Test Library" },
    });
    const library = (await libraryResp.json()) as { id: number };

    const authorResp = await apiContext.post("/test/persons", {
      data: { libraryId: library.id, name: "Test Author" },
    });
    const author = (await authorResp.json()) as { id: number };

    // Seed at least 3 books with distinct titles so sort differences
    // are observable across renders.
    await apiContext.post("/test/books", {
      data: {
        libraryId: library.id,
        title: "Alpha Book",
        fileType: "epub",
        authorId: author.id,
      },
    });
    await apiContext.post("/test/books", {
      data: {
        libraryId: library.id,
        title: "Beta Book",
        fileType: "epub",
        authorId: author.id,
      },
    });
    await apiContext.post("/test/books", {
      data: {
        libraryId: library.id,
        title: "Gamma Book",
        fileType: "epub",
        authorId: author.id,
      },
    });

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

  // Helper: log in and land on the test library gallery.
  async function loginAndOpenLibrary(page: import("@playwright/test").Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill("sortadmin");
    await page.getByLabel("Password").fill("password123");
    await page.getByRole("button", { name: "Sign in" }).click();
    // Redirect goes to /settings/libraries by default; go to the test
    // library directly.
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
    await page.goto(`/libraries/${testData.libraryId}`, {
      waitUntil: "domcontentloaded",
    });
    // Wait for the gallery to resolve (Gallery renders after
    // settingsResolved is true).
    await expect(page.getByRole("button", { name: /^Sort$/ })).toBeVisible();
  }

  test("default load shows no sort chip row and no dirty dot", async ({
    page,
  }) => {
    await loginAndOpenLibrary(page);
    await expect(page.getByText(/^Sorted by:/)).not.toBeVisible();
    // Dirty dot is an element with aria-label "Sort differs from default".
    await expect(
      page.getByLabel("Sort differs from default"),
    ).not.toBeVisible();
  });

  test("selecting a sort updates URL and shows dot + chip row", async ({
    page,
  }) => {
    await loginAndOpenLibrary(page);
    await page.getByRole("button", { name: /^Sort$/ }).click();
    // The sheet opens showing the builtin default (date_added:desc) as the
    // current active level. The "Then by…" section lists unused fields.
    // Click Title to add it as a second sort level.
    await expect(page.getByRole("button", { name: /^Title$/ })).toBeVisible();
    await page.getByRole("button", { name: /^Title$/ }).click();
    // Close the sheet before asserting on the chip row (chips render behind
    // the open sheet panel and may not be reachable while it is open).
    await page.keyboard.press("Escape");
    // The sort param is URL-encoded (colon → %3A, comma → %2C).
    // Verify title:asc appears anywhere in the sort param.
    await expect(page).toHaveURL(/title%3Aasc/);
    await expect(page.getByText(/^Sorted by:/)).toBeVisible();
    await expect(page.getByLabel("Sort differs from default")).toBeVisible();
    // The chip uses the aria-label pattern "Title ascending — click to remove".
    await expect(
      page.getByRole("button", { name: /Title ascending/i }),
    ).toBeVisible();
  });

  test("clicking a Sorted-by chip removes that level", async ({ page }) => {
    await loginAndOpenLibrary(page);
    await page.goto(`/libraries/${testData.libraryId}?sort=title:asc`, {
      waitUntil: "domcontentloaded",
    });
    await expect(page.getByText(/^Sorted by:/)).toBeVisible();
    await page.getByRole("button", { name: /Title ascending/i }).click();
    await expect(page).not.toHaveURL(/[?&]sort=/);
    await expect(page.getByText(/^Sorted by:/)).not.toBeVisible();
  });

  test("save as default persists and clears URL", async ({ page }) => {
    await loginAndOpenLibrary(page);
    await page.goto(`/libraries/${testData.libraryId}?sort=title:asc`, {
      waitUntil: "domcontentloaded",
    });
    // When the sort is dirty the SortButton's accessible name includes the
    // dirty-dot span text: "Sort Sort differs from default". Use the exact
    // dirty-state accessible name so we don't accidentally match the
    // library name button ("Sort Test Library").
    await page
      .getByRole("button", { name: "Sort Sort differs from default" })
      .click();
    await page.getByRole("button", { name: /Save as default/i }).click();
    await expect(page).not.toHaveURL(/[?&]sort=/);
    await expect(page.getByText(/^Sorted by:/)).not.toBeVisible();
    await expect(
      page.getByLabel("Sort differs from default"),
    ).not.toBeVisible();
    // Reload — saved default is still applied (no ?sort= in URL, no chip row).
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /^Sort$/ })).toBeVisible();
    await expect(page).not.toHaveURL(/[?&]sort=/);
    await expect(page.getByText(/^Sorted by:/)).not.toBeVisible();
  });

  test("reset to default clears URL", async ({ page }) => {
    await loginAndOpenLibrary(page);
    await page.goto(`/libraries/${testData.libraryId}?sort=date_added:asc`, {
      waitUntil: "domcontentloaded",
    });
    await expect(page.getByText(/^Sorted by:/)).toBeVisible();
    await page.getByRole("button", { name: /reset to default/i }).click();
    await expect(page).not.toHaveURL(/[?&]sort=/);
  });
});
