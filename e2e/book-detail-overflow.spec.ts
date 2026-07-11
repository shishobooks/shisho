/**
 * E2E regression test for horizontal overflow on the book detail page.
 *
 * Issue #398: on mobile viewports the book detail page could render slightly
 * wider than the screen for some audiobooks, adding a horizontal scrollbar.
 * Audiobooks (M4B) reproduce it because they carry the widest per-file stat
 * payload (duration, bitrate, codec, filesize) plus three action buttons in a
 * single non-wrapping row.
 *
 * Layout overflow cannot be asserted in jsdom (no layout engine), so this lives
 * as an E2E test where a real browser computes scrollWidth/clientWidth.
 *
 * Running:
 *   pnpm e2e:chromium e2e/book-detail-overflow.spec.ts   # Chromium only
 *   pnpm exec playwright test book-detail-overflow        # Both browsers
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

interface TestData {
  libraryId: number;
}

let testData: TestData;

const USERNAME = "overflowtest";
const PASSWORD = "password123";

test.describe("Book detail overflow", () => {
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
      data: { name: "Overflow Test Library" },
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

  // Helper: log in through the login form.
  async function login(page: Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
  }

  test("audiobook with full stats and a long title does not force horizontal scroll on mobile", async ({
    page,
    apiContext,
  }) => {
    // Seed an M4B audiobook carrying the full per-file stat payload plus a
    // long title with unbroken tokens. Both the stats/actions row and the
    // title are unbounded-content nodes that could push the page wider than
    // the viewport.
    const bookResp = await apiContext.post("/test/books", {
      data: {
        libraryId: testData.libraryId,
        title:
          "The Complete Unabridged Chronicles of Antidisestablishmentarianism Pneumonoultramicroscopicsilicovolcanoconiosis",
        fileType: "m4b",
        name: "The Complete Unabridged Chronicles of Antidisestablishmentarianism Pneumonoultramicroscopicsilicovolcanoconiosis",
        fileSize: 268435456, // 256 MB
        audiobookDurationSeconds: 31480, // 8h 44m
        audiobookBitrateBps: 128000, // 128 kbps
        audiobookCodec: "aac",
      },
    });
    const book = (await bookResp.json()) as { id: number; fileId: number };

    await login(page);

    await page.setViewportSize({ width: 375, height: 800 });
    await page.goto(`/libraries/${testData.libraryId}/books/${book.id}`, {
      waitUntil: "domcontentloaded",
    });

    // Wait for the file row to render. The Files heading only appears once the
    // book data resolves, which guarantees the stats/actions row is present.
    await expect(
      page.getByRole("heading", { name: "Files (1)", exact: true }),
    ).toBeVisible();

    // The audiobook stat payload must actually render. Otherwise the overflow
    // check below could pass for the wrong reason (no stats means nothing wide
    // enough to overflow). The mobile stats row renders the bitrate inline from
    // audiobook_bitrate_bps, so assert the visible copy is present.
    await expect(
      page.getByText("128 kbps").filter({ visible: true }),
    ).toBeVisible();

    // The document must not be scrollable horizontally.
    const noOverflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    );
    expect(noOverflow).toBe(true);
  });
});
