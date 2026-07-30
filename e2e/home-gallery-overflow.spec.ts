/**
 * E2E regression test for unnecessary vertical overflow on the home gallery.
 *
 * Layout overflow requires a real browser so the document's scroll geometry is
 * computed. The gallery should only scroll when its content exceeds the
 * viewport.
 *
 * Running:
 *   pnpm e2e:chromium e2e/home-gallery-overflow.spec.ts
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

interface TestData {
  libraryId: number;
}

let testData: TestData;

const USERNAME = "galleryoverflow";
const PASSWORD = "password123";

test.describe("Home gallery vertical overflow", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });

    await apiContext.delete("/test/ereader");
    await apiContext.delete("/test/users");

    await apiContext.post("/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });

    const libraryResp = await apiContext.post("/test/libraries", {
      data: { name: "Gallery Overflow Test Library" },
    });
    const library = (await libraryResp.json()) as { id: number };

    const authorResp = await apiContext.post("/test/persons", {
      data: { libraryId: library.id, name: "Test Author" },
    });
    const author = (await authorResp.json()) as { id: number };

    for (const title of ["Alpha Book", "Beta Book", "Gamma Book"]) {
      await apiContext.post("/test/books", {
        data: {
          libraryId: library.id,
          title,
          fileType: "epub",
          authorId: author.id,
        },
      });
    }

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

  async function login(page: Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
  }

  test("scrolls vertically only when the gallery content exceeds the viewport", async ({
    page,
  }) => {
    await login(page);

    await page.setViewportSize({ width: 1280, height: 1200 });
    await page.goto(`/libraries/${testData.libraryId}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(page.getByText("Showing 1-3 of 3 books")).toBeVisible();

    const fittingGallery = await page.evaluate(() => ({
      clientHeight: document.documentElement.clientHeight,
      scrollHeight: document.documentElement.scrollHeight,
    }));
    expect(fittingGallery.scrollHeight).toBeLessThanOrEqual(
      fittingGallery.clientHeight,
    );

    await page.setViewportSize({ width: 1280, height: 300 });

    const overflowingGallery = await page.evaluate(() => ({
      clientHeight: document.documentElement.clientHeight,
      scrollHeight: document.documentElement.scrollHeight,
    }));
    expect(overflowingGallery.scrollHeight).toBeGreaterThan(
      overflowingGallery.clientHeight,
    );

    await page.evaluate(() =>
      window.scrollTo(0, document.documentElement.scrollHeight),
    );
    expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(0);
  });
});
