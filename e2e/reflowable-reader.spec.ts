/**
 * E2E test for the Reflowable reader with a MOBI file. foliate renders the
 * book inside closed shadow roots, so the test reads the text from the frame
 * foliate creates rather than through locators on the page.
 *
 * Running:
 *   pnpm e2e:chromium e2e/reflowable-reader.spec.ts
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "reflowablereader";
const PASSWORD = "password123";
const TITLE = "Kindle Only Book";

let libraryId: number;
let bookId: number;

test.describe("Reflowable reader", () => {
  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });

    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });

    const libraryResp = await apiContext.post("/api/test/libraries", {
      data: { name: "Reflowable Library" },
    });
    libraryId = ((await libraryResp.json()) as { id: number }).id;

    const bookResp = await apiContext.post("/api/test/books", {
      data: { libraryId, title: TITLE, fileType: "mobi", withFileOnDisk: true },
    });
    expect(bookResp.ok()).toBe(true);
    bookId = ((await bookResp.json()) as { id: number }).id;
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

  test("opens a MOBI from the book page", async ({ page }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);

    await page.goto(`/libraries/${libraryId}/books/${bookId}`, {
      waitUntil: "domcontentloaded",
    });
    await page.getByRole("link", { name: "Read", exact: true }).click();
    await page.waitForURL(/\/files\/\d+\/read$/);

    // foliate's MOBI modules load on first use, which a cold dev server
    // compiles on demand, so allow more than the default expect timeout.
    await expect
      .poll(
        async () => {
          for (const frame of page.frames()) {
            const text = await frame
              .locator("body")
              .textContent({ timeout: 1000 })
              .catch(() => null);
            if (text?.includes("Hello world.")) return true;
          }
          return false;
        },
        { timeout: 20_000 },
      )
      .toBe(true);
    await expect(page.getByText("Preparing book…")).toBeHidden();
    await expect(page.getByText("We couldn't load this book.")).toHaveCount(0);
    await expect(
      page.getByRole("slider", { name: "Reading progress" }),
    ).toBeVisible();
  });
});
