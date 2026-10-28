/**
 * E2E test for reading tracking: marking a book read from its page, rating
 * it in quarter stars, and seeing its status on the library's book card.
 *
 * Running:
 *   pnpm e2e:chromium e2e/reading.spec.ts   # Chromium only
 *   mise test:e2e -- reading                 # Both browsers
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "readingtest";
const PASSWORD = "password123";

let libraryId: number;
let bookId: number;

test.describe("Reading", () => {
  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });
    const library = (await (
      await apiContext.post("/api/test/libraries", {
        data: { name: "Reading Test Library" },
      })
    ).json()) as { id: number };
    libraryId = library.id;
    const book = (await (
      await apiContext.post("/api/test/books", {
        data: { libraryId, title: "Tracked Book", fileType: "epub" },
      })
    ).json()) as { id: number };
    bookId = book.id;
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

  test("marks a book read, rates it, and shows the status on its card", async ({
    page,
  }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/admin\/libraries|\/libraries\//);

    await page.goto(`/libraries/${libraryId}/books/${bookId}`, {
      waitUntil: "domcontentloaded",
    });
    await page
      .getByRole("button", { name: "Reading status: Track reading" })
      .click();
    await page.getByRole("menuitem", { name: "Mark as read" }).click();
    await expect(
      page.getByRole("button", { name: "Reading status: Read" }),
    ).toBeVisible();
    await expect(page.getByText(/^Read once, last finished/)).toBeVisible();

    // The stars span the slider evenly, so three quarters of the way across
    // is 3.75 stars.
    const slider = page.getByRole("slider", { name: "Rating" });
    const rating = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/reading/books/${bookId}/rating`) &&
        response.ok(),
    );
    const box = await page.getByTestId("star-rating").boundingBox();
    if (!box) throw new Error("rating slider has no box");
    await page.mouse.click(box.x + box.width * 0.75, box.y + box.height / 2);
    await rating;
    await expect(slider).toHaveAttribute("aria-valuetext", "3.75 stars");

    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByRole("slider", { name: "Rating" })).toHaveAttribute(
      "aria-valuetext",
      "3.75 stars",
    );

    await page.goto(`/libraries/${libraryId}`, {
      waitUntil: "domcontentloaded",
    });
    const card = page.getByRole("link").filter({ hasText: "Tracked Book" });
    await expect(card.getByText("Read", { exact: true })).toBeVisible();
  });
});
