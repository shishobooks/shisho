/**
 * E2E tests for the CBZ and PDF reader's flows. Scroll positions depend on
 * real layout and image loading, which unit tests can't see.
 *
 * Running:
 *   pnpm e2e:chromium e2e/page-reader.spec.ts
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "pagereader";
const PASSWORD = "password123";

let libraryId: number;
let webtoonBookId: number;
let webtoonFileId: number;
let comicBookId: number;
let comicFileId: number;
let swipeBookId: number;
let swipeFileId: number;

async function login(page: Page) {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByLabel("Username").fill(USERNAME);
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(/\/admin\/libraries|\/libraries\//);
}

const readerUrl = (bookId: number, fileId: number, page?: number) =>
  `/libraries/${libraryId}/books/${bookId}/files/${fileId}/read` +
  (page === undefined ? "" : `?page=${page}`);

// Where the top of a page sits relative to the top of the reader's view.
const pageTop = (page: Page, pageNumber: number) =>
  page.evaluate((alt) => {
    const main = document.querySelector("main")!;
    const img = main.querySelector(`img[alt="${alt}"]`)!;
    return Math.round(
      img.parentElement!.getBoundingClientRect().top -
        main.getBoundingClientRect().top,
    );
  }, `Page ${pageNumber}`);

const pageImages = (page: Page) => page.locator("main img");

test.describe("Page reader flows", () => {
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
      data: { name: "Page Reader Library" },
    });
    libraryId = ((await libraryResp.json()) as { id: number }).id;

    const seed = async (title: string, width: number, height: number) => {
      const resp = await apiContext.post("/api/test/books", {
        data: {
          libraryId,
          title,
          fileType: "cbz",
          withFileOnDisk: true,
          pageCount: 6,
          pageWidth: width,
          pageHeight: height,
        },
      });
      expect(resp.ok()).toBe(true);
      return (await resp.json()) as { id: number; fileId: number };
    };
    // Strips five times taller than wide, like a webtoon's.
    ({ id: webtoonBookId, fileId: webtoonFileId } = await seed(
      "Webtoon",
      200,
      1000,
    ));
    ({ id: comicBookId, fileId: comicFileId } = await seed(
      "Printed Comic",
      600,
      900,
    ));
    // Stays paginated: another test here saves the scrolled flow for its comic.
    ({ id: swipeBookId, fileId: swipeFileId } = await seed(
      "Swiped Comic",
      600,
      900,
    ));
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

  test("a webtoon scrolls, and the page counter follows", async ({ page }) => {
    await login(page);
    await page.goto(readerUrl(webtoonBookId, webtoonFileId), {
      waitUntil: "domcontentloaded",
    });

    await expect(pageImages(page)).toHaveCount(6);
    await expect(page.getByText("Page 1 of 6")).toBeVisible();

    // Scroll into the third page.
    await page.evaluate(() => {
      const main = document.querySelector("main")!;
      const third = main.querySelector('img[alt="Page 3"]')!.parentElement!;
      main.scrollTop = third.offsetTop + 100;
    });
    await expect(page.getByText("Page 3 of 6")).toBeVisible();
    await expect(page).toHaveURL(/\?page=2$/);

    await page.getByRole("button", { name: "Next page" }).click();
    await expect(page.getByText("Page 4 of 6")).toBeVisible();
    await expect.poll(() => pageTop(page, 4)).toBe(0);
  });

  test("a link to a webtoon page stays on it while the pages above load", async ({
    page,
  }) => {
    await login(page);
    await page.goto(readerUrl(webtoonBookId, webtoonFileId, 4), {
      waitUntil: "domcontentloaded",
    });

    // The page above has loaded and grown past its placeholder.
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            document.querySelector<HTMLImageElement>('main img[alt="Page 4"]')
              ?.naturalWidth ?? 0,
        ),
      )
      .toBeGreaterThan(0);
    await expect.poll(() => pageTop(page, 5)).toBe(0);
    await expect(page.getByText("Page 5 of 6")).toBeVisible();
  });

  test("a printed comic pages, and the flow chosen for it is saved", async ({
    page,
  }) => {
    await login(page);
    await page.goto(readerUrl(comicBookId, comicFileId), {
      waitUntil: "domcontentloaded",
    });

    await expect(page.getByRole("img", { name: "Page 1" })).toBeVisible();
    await expect(pageImages(page)).toHaveCount(1);

    await page.getByRole("button", { name: "Reader settings" }).click();
    await expect(
      page.getByText("Picked from the page shapes until you choose."),
    ).toBeVisible();
    await page.getByRole("button", { name: "Scrolled", exact: true }).click();
    await expect(pageImages(page)).toHaveCount(6);
    await expect(page.getByText("Saved for this book.")).toBeVisible();

    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(pageImages(page)).toHaveCount(6);
  });

  test.describe("swiping on a phone", () => {
    const WIDTH = 390;
    const HEIGHT = 844;
    test.use({ hasTouch: true, viewport: { width: WIDTH, height: HEIGHT } });
    // Playwright has no touch-drag API, and only Chromium speaks CDP.
    test.skip(
      ({ browserName }) => browserName !== "chromium",
      "Sends raw touch events over CDP",
    );

    const swipe = async (page: Page, fromX: number, toX: number) => {
      const cdp = await page.context().newCDPSession(page);
      const y = HEIGHT / 2;
      const send = (
        type: "touchStart" | "touchMove" | "touchEnd",
        x?: number,
      ) =>
        cdp.send("Input.dispatchTouchEvent", {
          type,
          touchPoints: x === undefined ? [] : [{ x, y }],
        });
      await send("touchStart", fromX);
      for (let step = 1; step <= 3; step++) {
        await send("touchMove", fromX + ((toX - fromX) * step) / 3);
        await page.waitForTimeout(16);
      }
      await send("touchEnd");
    };

    const openComic = async (page: Page, pageNumber: number) => {
      await login(page);
      // The URL's page is 0-indexed, unlike the one on screen.
      await page.goto(readerUrl(swipeBookId, swipeFileId, pageNumber - 1), {
        waitUntil: "domcontentloaded",
      });
      // The image shows once the flow is known, which is when swipes turn on.
      await expect(
        page.getByRole("img", { name: `Page ${pageNumber}`, exact: true }),
      ).toBeVisible();
    };

    for (const [zone, x] of [
      ["left", WIDTH * 0.3],
      ["center", WIDTH * 0.5],
      ["right", WIDTH * 0.9],
    ] as const) {
      test(`a next-page swipe from the ${zone} third turns one page`, async ({
        page,
      }) => {
        await openComic(page, 3);
        await swipe(page, x, x - 100);
        await expect(page.getByText("Page 4 of 6")).toBeVisible();
      });

      // On the first page the left tap zone is a disabled button.
      test(`a next-page swipe from the ${zone} third works on the first page`, async ({
        page,
      }) => {
        await openComic(page, 1);
        await swipe(page, x, x - 100);
        await expect(page.getByText("Page 2 of 6")).toBeVisible();
      });
    }

    for (const [zone, x] of [
      ["left", WIDTH * 0.1],
      ["center", WIDTH * 0.5],
      ["right", WIDTH * 0.7],
    ] as const) {
      test(`a previous-page swipe from the ${zone} third turns one page`, async ({
        page,
      }) => {
        await openComic(page, 3);
        await swipe(page, x, x + 100);
        await expect(page.getByText("Page 2 of 6")).toBeVisible();
      });
    }

    test("a tap still turns the page", async ({ page }) => {
      await openComic(page, 3);
      await page.touchscreen.tap(WIDTH * 0.9, HEIGHT / 2);
      await expect(page.getByText("Page 4 of 6")).toBeVisible();
    });
  });
});
