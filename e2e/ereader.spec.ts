/**
 * E2E tests for the eReader browser UI.
 *
 * Tests the eReader routes that serve HTML pages for e-reader devices.
 * These pages are authenticated via API key in the URL path.
 *
 * Running:
 *   yarn test:e2e                        # Run all E2E tests
 *   yarn test:e2e e2e/ereader.spec.ts    # Run only eReader tests
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";

interface TestData {
  userId: number;
  libraryId: number;
  bookId: number;
  apiKey: string;
  seriesId: number;
  authorId: number;
}

// Test data populated in beforeAll
let testData: TestData;

test.describe("eReader Browser UI", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });

    // Clean up existing test data first
    await apiContext.delete("/test/ereader");

    // Clean up existing test data
    await apiContext.delete("/test/ereader");
    await apiContext.delete("/test/users");

    // Create test user
    const userResp = await apiContext.post("/test/users", {
      data: { username: "ereadertest", password: "password123" },
    });
    const user = (await userResp.json()) as { id: number };

    // Create library
    const libraryResp = await apiContext.post("/test/libraries", {
      data: { name: "Test Library" },
    });
    const library = (await libraryResp.json()) as { id: number };

    // Create author (person)
    const authorResp = await apiContext.post("/test/persons", {
      data: { libraryId: library.id, name: "Test Author" },
    });
    const author = (await authorResp.json()) as { id: number };

    // Create series
    const seriesResp = await apiContext.post("/test/series", {
      data: { libraryId: library.id, name: "Test Series" },
    });
    const series = (await seriesResp.json()) as { id: number };

    // Create books
    const book1Resp = await apiContext.post("/test/books", {
      data: {
        libraryId: library.id,
        title: "Test Book 1",
        fileType: "epub",
        authorId: author.id,
        seriesId: series.id,
      },
    });
    const book1 = (await book1Resp.json()) as { id: number };

    await apiContext.post("/test/books", {
      data: {
        libraryId: library.id,
        title: "Test Book 2",
        fileType: "cbz",
        authorId: author.id,
      },
    });

    await apiContext.post("/test/books", {
      data: {
        libraryId: library.id,
        title: "Audiobook Test",
        fileType: "m4b",
      },
    });

    // Create API key with eReader permission
    const apiKeyResp = await apiContext.post("/test/api-keys", {
      data: {
        userId: user.id,
        name: "Test eReader Key",
        permissions: ["ereader_browser"],
      },
    });
    const apiKeyData = (await apiKeyResp.json()) as { key: string };

    testData = {
      userId: user.id,
      libraryId: library.id,
      bookId: book1.id,
      apiKey: apiKeyData.key,
      seriesId: series.id,
      authorId: author.id,
    };

    await apiContext.dispose();
  });

  test.afterAll(async ({ browser }) => {
    // Clean up all eReader test data after tests complete
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/test/ereader");
    await apiContext.dispose();
  });

  test.describe("Authentication", () => {
    test("returns 401 for invalid API key", async ({ page }) => {
      const response = await page.goto("/ereader/key/invalid-key/");
      expect(response?.status()).toBe(401);
    });

    test("returns 403 for API key without permission", async ({ browser }) => {
      const apiBaseURL = getApiBaseURL(browser.browserType().name());
      const apiContext = await request.newContext({ baseURL: apiBaseURL });

      // Create API key without eReader permission
      const apiKeyResp = await apiContext.post("/test/api-keys", {
        data: {
          userId: testData.userId,
          name: "No Permission Key",
          permissions: [], // No permissions
        },
      });
      const apiKeyData = (await apiKeyResp.json()) as { key: string };
      await apiContext.dispose();

      const page = await browser.newPage();
      const response = await page.goto(`/ereader/key/${apiKeyData.key}/`);
      expect(response?.status()).toBe(403);
      await page.close();
    });

    test("allows access with valid API key and permission", async ({
      page,
    }) => {
      const response = await page.goto(`/ereader/key/${testData.apiKey}/`);
      expect(response?.status()).toBe(200);
      await expect(page.getByText("Libraries")).toBeVisible();
    });
  });

  test.describe("Libraries Page", () => {
    test("shows list of libraries", async ({ page }) => {
      await page.goto(`/ereader/key/${testData.apiKey}/`);
      await expect(
        page.getByRole("heading", { name: "Libraries" }),
      ).toBeVisible();
      await expect(page.getByText("Test Library")).toBeVisible();
    });

    test("can navigate to library", async ({ page }) => {
      await page.goto(`/ereader/key/${testData.apiKey}/`);
      await page.locator(".item-title", { hasText: "Test Library" }).click();
      await expect(
        page.getByRole("heading", { name: "Library" }),
      ).toBeVisible();
      // Check for navigation items by their title (more specific)
      await expect(
        page.locator(".item-title", { hasText: "All Books" }),
      ).toBeVisible();
      await expect(
        page.locator(".item-title", { hasText: "Series" }),
      ).toBeVisible();
      await expect(
        page.locator(".item-title", { hasText: "Authors" }),
      ).toBeVisible();
      await expect(
        page.locator(".item-title", { hasText: "Search" }),
      ).toBeVisible();
    });
  });

  test.describe("Library Navigation", () => {
    test("shows navigation options", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}`,
      );
      await expect(
        page.locator(".item-title", { hasText: "All Books" }),
      ).toBeVisible();
      await expect(
        page.locator(".item-title", { hasText: "Series" }),
      ).toBeVisible();
      await expect(
        page.locator(".item-title", { hasText: "Authors" }),
      ).toBeVisible();
      await expect(
        page.locator(".item-title", { hasText: "Search" }),
      ).toBeVisible();
    });
  });

  test.describe("All Books", () => {
    test("shows list of books", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/all`,
      );
      await expect(
        page.getByRole("heading", { name: "All Books" }),
      ).toBeVisible();
      await expect(page.getByText("Test Book 1")).toBeVisible();
      await expect(page.getByText("Test Book 2")).toBeVisible();
      await expect(page.getByText("Audiobook Test")).toBeVisible();
    });

    test("shows file type filter options", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/all`,
      );
      // Check for filter buttons specifically (not book metadata)
      await expect(
        page.locator(".filter-btn", { hasText: "EPUB" }),
      ).toBeVisible();
      await expect(
        page.locator(".filter-btn", { hasText: "CBZ" }),
      ).toBeVisible();
      await expect(
        page.locator(".filter-btn", { hasText: "M4B" }),
      ).toBeVisible();
    });

    test("can filter by file type", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/all?types=epub`,
      );
      await expect(page.getByText("Test Book 1")).toBeVisible();
      await expect(page.getByText("Test Book 2")).not.toBeVisible();
      await expect(page.getByText("Audiobook Test")).not.toBeVisible();
    });
  });

  test.describe("Series", () => {
    test("shows list of series", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/series`,
      );
      await expect(page.getByRole("heading", { name: "Series" })).toBeVisible();
      await expect(page.getByText("Test Series")).toBeVisible();
    });

    test("shows books in series", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/series/${testData.seriesId}`,
      );
      await expect(
        page.getByRole("heading", { name: "Test Series" }),
      ).toBeVisible();
      await expect(page.getByText("Test Book 1")).toBeVisible();
    });
  });

  test.describe("Authors", () => {
    test("shows list of authors", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/authors`,
      );
      await expect(
        page.getByRole("heading", { name: "Authors" }),
      ).toBeVisible();
      await expect(page.getByText("Test Author")).toBeVisible();
    });

    test("shows books by author", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/authors/${testData.authorId}`,
      );
      await expect(
        page.getByRole("heading", { name: "Test Author" }),
      ).toBeVisible();
      await expect(page.getByText("Test Book 1")).toBeVisible();
      await expect(page.getByText("Test Book 2")).toBeVisible();
    });
  });

  test.describe("Search", () => {
    test("shows search form", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/search`,
      );
      await expect(page.getByRole("heading", { name: "Search" })).toBeVisible();
      await expect(page.getByRole("textbox")).toBeVisible();
    });

    test("can search for books", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/search?q=Test+Book`,
      );
      await expect(page.getByText("Test Book 1")).toBeVisible();
      await expect(page.getByText("Test Book 2")).toBeVisible();
    });

    test("shows no results for non-matching query", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/libraries/${testData.libraryId}/search?q=nonexistent`,
      );
      await expect(page.getByText("Found 0 results")).toBeVisible();
    });
  });

  test.describe("Book Download Page", () => {
    test("shows book details", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/download/${testData.bookId}`,
      );
      await expect(
        page.getByRole("heading", { name: "Test Book 1" }),
      ).toBeVisible();
      await expect(page.getByText("By: Test Author")).toBeVisible();
    });

    test("shows download link", async ({ page }) => {
      await page.goto(
        `/ereader/key/${testData.apiKey}/download/${testData.bookId}`,
      );
      await expect(
        page.getByRole("link", { name: /Download EPUB/i }),
      ).toBeVisible();
    });
  });
});
