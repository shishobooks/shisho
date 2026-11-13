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
let tallBookId: number;
let tallFileId: number;

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

// The pages a reader sees or scrolls to. A paginated page's neighbors wait
// off-screen, hidden from assistive tech.
const pageImages = (page: Page) => page.locator("main").getByRole("img");

// Holds the responses to the requests `matches` picks until the returned
// function is called, so a test can act at a point in the page's loading
// that it would otherwise reach only by luck.
const holdResponses = async (page: Page, matches: (url: URL) => boolean) => {
  let release!: () => void;
  const released = new Promise<void>((resolve) => (release = resolve));
  await page.route(matches, async (route) => {
    await released;
    await route.continue();
  });
  return release;
};

// A six-page comic book with pages of the given size.
const seedBook = async (
  browserName: string,
  title: string,
  width: number,
  height: number,
) => {
  const apiContext = await request.newContext({
    baseURL: getApiBaseURL(browserName),
  });
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
  const book = (await resp.json()) as { id: number; fileId: number };
  await apiContext.dispose();
  return book;
};

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
    ({ id: tallBookId, fileId: tallFileId } = await seed(
      "Tall Comic",
      200,
      1000,
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

  // A page that loads grows, and the view keeps still only if the page is
  // above it. The reader can scroll after the image loads but before the
  // page takes its new height, so that is decided when it grows.
  test("a webtoon page that loads as the reader scrolls past it keeps the view still", async ({
    page,
  }) => {
    const release = await holdResponses(
      page,
      (url) =>
        url.pathname === `/api/books/files/${webtoonFileId}/page/0` &&
        !url.searchParams.has("size"),
    );
    await login(page);
    await page.goto(readerUrl(webtoonBookId, webtoonFileId), {
      waitUntil: "domcontentloaded",
    });
    await expect(pageImages(page)).toHaveCount(6);

    // The reader handles the load first, then this scrolls to page 3.
    await page.evaluate(() => {
      const main = document.querySelector("main")!;
      const first = main.querySelector<HTMLImageElement>('img[alt="Page 1"]')!;
      first.addEventListener(
        "load",
        () => {
          const third = main.querySelector('img[alt="Page 3"]')!.parentElement!;
          main.scrollTop = third.offsetTop + 100;
        },
        { once: true },
      );
    });
    release();
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            document.querySelector<HTMLImageElement>('main img[alt="Page 1"]')
              ?.naturalWidth ?? 0,
        ),
      )
      .toBeGreaterThan(0);
    await expect.poll(() => pageTop(page, 3)).toBe(-100);
    await expect(page.getByText("Page 3 of 6")).toBeVisible();
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

  // The pages take focus in the scrolled flow, so Space and Page Down scroll
  // them. Each test reads a book of its own: scrolling by key saves a place.
  test("a webtoon opens with the keys scrolling its pages", async ({
    page,
    browser,
  }) => {
    const book = await seedBook(
      browser.browserType().name(),
      "Keyed Webtoon",
      200,
      1000,
    );
    await login(page);
    await page.goto(readerUrl(book.id, book.fileId), {
      waitUntil: "domcontentloaded",
    });
    await expect(pageImages(page)).toHaveCount(6);
    await expect(page.locator("main")).toBeFocused();
    await page.keyboard.press("PageDown");
    await expect
      .poll(() =>
        page.evaluate(() => document.querySelector("main")!.scrollTop),
      )
      .toBeGreaterThan(0);
  });

  test("closing the settings after picking Scrolled leaves the keys scrolling", async ({
    page,
    browser,
  }) => {
    const book = await seedBook(
      browser.browserType().name(),
      "Keyed Comic",
      600,
      900,
    );
    await login(page);
    await page.goto(readerUrl(book.id, book.fileId), {
      waitUntil: "domcontentloaded",
    });
    await expect(page.getByRole("img", { name: "Page 1" })).toBeVisible();
    await page.getByRole("button", { name: "Reader settings" }).click();
    await page.getByRole("button", { name: "Scrolled", exact: true }).click();
    await expect(pageImages(page)).toHaveCount(6);
    await page.keyboard.press("Escape");
    await expect(page.locator("main")).toBeFocused();
  });

  test("settings opened while the reader loads stay open as it settles on scrolling", async ({
    page,
    browser,
  }) => {
    // A book of its own: picking Paginated saves that flow for the book,
    // which would change how the shared webtoon opens in the tests above.
    const book = await seedBook(
      browser.browserType().name(),
      "Settling Webtoon",
      200,
      1000,
    );

    // Hold the thumbnails the reader probes for the pages' shape, so the
    // settings open before it knows the flow.
    const release = await holdResponses(
      page,
      (url) =>
        url.pathname.startsWith(`/api/books/files/${book.fileId}/page/`) &&
        url.searchParams.has("size"),
    );
    await login(page);
    await page.goto(readerUrl(book.id, book.fileId), {
      waitUntil: "domcontentloaded",
    });
    await page.getByRole("button", { name: "Reader settings" }).click();
    const paginated = page.getByRole("button", {
      name: "Paginated",
      exact: true,
    });
    await expect(paginated).toBeDisabled();

    release();
    // Tall strips scroll, and the settings stay open to pick another flow.
    await expect(pageImages(page)).toHaveCount(6);
    await expect(paginated).toBeEnabled();
    await paginated.click();
    await expect(pageImages(page)).toHaveCount(1);
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

    test("a next-page swipe from any third turns one page", async ({
      page,
    }) => {
      await openComic(page, 3);
      for (const [x, landsOn] of [
        [0.3, 4],
        [0.5, 5],
        [0.9, 6],
      ]) {
        await swipe(page, WIDTH * x, WIDTH * x - 100);
        await expect(page.getByText(`Page ${landsOn} of 6`)).toBeVisible();
      }
    });

    // On the first page the left tap zone is a disabled button.
    test("a next-page swipe from the left third works on the first page", async ({
      page,
    }) => {
      await openComic(page, 1);
      await swipe(page, WIDTH * 0.3, WIDTH * 0.3 - 100);
      await expect(page.getByText("Page 2 of 6")).toBeVisible();
    });

    test("a previous-page swipe from any third turns one page", async ({
      page,
    }) => {
      await openComic(page, 4);
      for (const [x, landsOn] of [
        [0.1, 3],
        [0.5, 2],
        [0.7, 1],
      ]) {
        await swipe(page, WIDTH * x, WIDTH * x + 100);
        await expect(page.getByText(`Page ${landsOn} of 6`)).toBeVisible();
      }
    });

    const touch = async (page: Page) => {
      const cdp = await page.context().newCDPSession(page);
      const y = HEIGHT / 2;
      return (type: "touchStart" | "touchMove" | "touchEnd", x?: number) =>
        cdp.send("Input.dispatchTouchEvent", {
          type,
          touchPoints: x === undefined ? [] : [{ x, y }],
        });
    };

    // Where a page's image is across the screen. Neighbors have no alt text,
    // so this goes by src, which holds the 0-indexed page.
    const imageLeft = (page: Page, zeroIndexedPage: number) =>
      page.evaluate((n) => {
        const image = Array.from(
          document.querySelectorAll<HTMLImageElement>("main img"),
        ).find((img) => new RegExp(`/pages?/${n}\\b`).test(img.src));
        return image ? Math.round(image.getBoundingClientRect().left) : null;
      }, zeroIndexedPage);

    test("the next page comes in under the finger", async ({ page }) => {
      await openComic(page, 3);
      const send = await touch(page);
      await send("touchStart", WIDTH * 0.9);
      await send("touchMove", WIDTH * 0.7);
      await send("touchMove", WIDTH * 0.5);

      const nextLeft = await imageLeft(page, 3);
      expect(nextLeft).toBeGreaterThan(0);
      expect(nextLeft).toBeLessThan(WIDTH);

      await send("touchEnd");
      await expect(page.getByText("Page 4 of 6")).toBeVisible();
      await expect.poll(() => imageLeft(page, 3)).toBe(0);
    });

    test("no spinner shows after a turn, since the page already loaded", async ({
      page,
    }) => {
      await openComic(page, 3);
      await expect
        .poll(() =>
          page.evaluate(() =>
            Array.from(document.querySelectorAll("main img")).every(
              (img) => (img as HTMLImageElement).complete,
            ),
          ),
        )
        .toBe(true);
      // Record whether the spinner ever appears during the turn.
      await page.evaluate(() => {
        const main = document.querySelector("main")!;
        (window as unknown as { spun: boolean }).spun = false;
        new MutationObserver(() => {
          if (main.querySelector('[role="status"]')) {
            (window as unknown as { spun: boolean }).spun = true;
          }
        }).observe(main, { childList: true, subtree: true });
      });

      await swipe(page, WIDTH * 0.9, WIDTH * 0.3);
      await expect(page.getByText("Page 4 of 6")).toBeVisible();
      await page.waitForTimeout(200);
      expect(
        await page.evaluate(
          () => (window as unknown as { spun: boolean }).spun,
        ),
      ).toBe(false);
    });

    test("a zoomed page pans sideways instead of turning", async ({ page }) => {
      await openComic(page, 3);
      const cdp = await page.context().newCDPSession(page);
      await cdp.send("Input.synthesizePinchGesture", {
        x: WIDTH / 2,
        y: HEIGHT / 2,
        scaleFactor: 3,
      });
      const viewport = () =>
        page.evaluate(() => ({
          scale: globalThis.visualViewport!.scale,
          left: Math.round(globalThis.visualViewport!.pageLeft),
        }));
      await expect
        .poll(async () => (await viewport()).scale)
        .toBeGreaterThan(1.5);
      // The pinch keeps settling after the scale passes that, which would also
      // move the view.
      await expect(async () => {
        const first = await viewport();
        await page.waitForTimeout(300);
        expect(await viewport()).toEqual(first);
      }).toPass();
      const before = (await viewport()).left;

      const send = await touch(page);
      await send("touchStart", WIDTH * 0.7);
      for (const x of [0.6, 0.5, 0.4, 0.3]) {
        await send("touchMove", WIDTH * x);
        await page.waitForTimeout(16);
      }
      await send("touchEnd");
      await expect.poll(async () => (await viewport()).left).not.toBe(before);
      await expect(page.getByText("Page 3 of 6")).toBeAttached();
    });

    // Touch events go to the element the finger first landed on, so a swipe
    // begun over the loading spinner must survive the spinner going away.
    test("a swipe begun while the page loads still turns it", async ({
      page,
    }) => {
      // Hold the first page's image, but not its thumbnail, which tells the
      // reader the page's shape.
      const release = await holdResponses(
        page,
        (url) =>
          url.pathname === `/api/books/files/${swipeFileId}/page/0` &&
          !url.searchParams.has("size"),
      );
      await login(page);
      await page.goto(readerUrl(swipeBookId, swipeFileId, 0), {
        waitUntil: "domcontentloaded",
      });
      // The track shows once the flow is known, which is when swipes turn on.
      await expect(page.getByTestId("page-track")).toBeAttached();
      const spinner = page.locator("main").getByRole("status");
      await expect(spinner).toBeVisible();

      const send = await touch(page);
      await send("touchStart", WIDTH * 0.5);
      await send("touchMove", WIDTH * 0.5 - 50);
      release();
      await expect(spinner).toBeHidden();
      await send("touchMove", WIDTH * 0.5 - 100);
      await send("touchEnd");
      await expect(page.getByText("Page 2 of 6")).toBeVisible();
    });

    test("a drag too short to turn springs back", async ({ page }) => {
      await openComic(page, 3);
      const send = await touch(page);
      await send("touchStart", WIDTH * 0.5);
      await send("touchMove", WIDTH * 0.5 - 30);
      await send("touchEnd");
      await expect.poll(() => imageLeft(page, 2)).toBe(0);
      await expect(page.getByText("Page 3 of 6")).toBeVisible();
    });

    test.describe("in Fit Width", () => {
      test.afterEach(async ({ page }) => {
        await page.request.put("/api/settings/user", {
          data: { fit_mode: "fit-height" },
        });
      });

      const setFitWidth = async (page: Page) => {
        await login(page);
        const put = await page.request.put("/api/settings/user", {
          data: { fit_mode: "fit-width" },
        });
        expect(put.ok()).toBe(true);
      };

      const gaps = (page: Page, name: string) =>
        page.evaluate((alt) => {
          const main = document.querySelector("main")!.getBoundingClientRect();
          const image = document
            .querySelector(`main img[alt="${alt}"]`)!
            .getBoundingClientRect();
          return {
            above: Math.round(image.top - main.top),
            below: Math.round(main.bottom - image.bottom),
          };
        }, name);

      test("a page shorter than the screen sits in the middle", async ({
        page,
      }) => {
        await setFitWidth(page);
        await page.goto(readerUrl(swipeBookId, swipeFileId, 1), {
          waitUntil: "domcontentloaded",
        });
        await expect(
          page.getByRole("img", { name: "Page 2", exact: true }),
        ).toBeVisible();
        await expect
          .poll(async () => {
            const { above, below } = await gaps(page, "Page 2");
            return above > 0 && Math.abs(above - below) <= 1;
          })
          .toBe(true);
      });

      test("a page taller than the screen starts at its top and scrolls", async ({
        page,
      }) => {
        await setFitWidth(page);
        // Flow is per book: a tall strip asked to stay paginated.
        await page.goto(readerUrl(tallBookId, tallFileId, 1), {
          waitUntil: "domcontentloaded",
        });
        await page.getByRole("button", { name: "Reader settings" }).click();
        await page
          .getByRole("button", { name: "Paginated", exact: true })
          .click();
        await page.keyboard.press("Escape");
        await expect(
          page.getByRole("img", { name: "Page 2", exact: true }),
        ).toBeVisible();
        await expect
          .poll(async () => (await gaps(page, "Page 2")).above)
          .toBe(0);
        await page.evaluate(() =>
          document.querySelector("main")!.scrollTo(0, 300),
        );
        expect(
          await page.evaluate(() => document.querySelector("main")!.scrollTop),
        ).toBe(300);
      });
    });
  });

  test.describe("tapping on a phone", () => {
    const WIDTH = 390;
    const HEIGHT = 844;
    test.use({ hasTouch: true, viewport: { width: WIDTH, height: HEIGHT } });

    test("a tap turns the page", async ({ page }) => {
      await login(page);
      await page.goto(readerUrl(swipeBookId, swipeFileId, 2), {
        waitUntil: "domcontentloaded",
      });
      await expect(
        page.getByRole("img", { name: "Page 3", exact: true }),
      ).toBeVisible();
      await page.touchscreen.tap(WIDTH * 0.9, HEIGHT / 2);
      await expect(page.getByText("Page 4 of 6")).toBeVisible();
    });
  });

  test.describe("resuming", () => {
    // A book of its own per test: the first page turn starts a Read-through.
    test("a comic reopens on the page turned to, and the end marks it read", async ({
      page,
      browser,
    }) => {
      const book = await seedBook(
        browser.browserType().name(),
        "Resumed Comic",
        600,
        900,
      );
      const savedPage = async () => {
        const resp = await page.request.get(
          `${getApiBaseURL(browser.browserType().name())}/api/reading/files/${book.fileId}/position`,
        );
        return ((await resp.json()) as { position?: { page?: number } })
          .position?.page;
      };
      await login(page);
      await page.goto(readerUrl(book.id, book.fileId), {
        waitUntil: "domcontentloaded",
      });
      await expect(page.getByRole("img", { name: "Page 1" })).toBeVisible();

      const nextPage = page.getByRole("button", { name: "Next page" });
      await nextPage.click();
      await nextPage.click();
      await expect(page.getByText("Page 3 of 6")).toBeVisible();

      // Opened without the page in the URL, so only the saved position can
      // bring it back.
      await expect.poll(savedPage).toBe(2);
      await page.goto(readerUrl(book.id, book.fileId), {
        waitUntil: "domcontentloaded",
      });
      await expect(page.getByText("Page 3 of 6")).toBeVisible();
      await expect(page.getByRole("img", { name: "Page 3" })).toBeVisible();

      // A page in the URL wins, and is saved.
      await page.goto(readerUrl(book.id, book.fileId, 1), {
        waitUntil: "domcontentloaded",
      });
      await expect(page.getByText("Page 2 of 6")).toBeVisible();
      await expect.poll(savedPage).toBe(1);
      await page.goto(readerUrl(book.id, book.fileId), {
        waitUntil: "domcontentloaded",
      });
      await expect(page.getByText("Page 2 of 6")).toBeVisible();

      await page.getByRole("slider", { name: "Reading progress" }).focus();
      await page.keyboard.press("End");
      await expect(page.getByText("Page 6 of 6")).toBeVisible();
      await expect(page.getByText("Marked as read")).toBeVisible();
    });

    test("a webtoon reopens on the page scrolled to", async ({
      page,
      browser,
    }) => {
      const book = await seedBook(
        browser.browserType().name(),
        "Resumed Webtoon",
        200,
        1000,
      );
      await login(page);
      await page.goto(readerUrl(book.id, book.fileId), {
        waitUntil: "domcontentloaded",
      });
      await expect(pageImages(page)).toHaveCount(6);

      // A real wheel, since a scroll with no input isn't reading.
      await page.locator("main").hover();
      await page.mouse.wheel(0, 200);
      await page.evaluate(() => {
        const main = document.querySelector("main")!;
        const fourth = main.querySelector('img[alt="Page 4"]')!.parentElement!;
        main.scrollTop = fourth.offsetTop + 100;
      });
      await expect(page.getByText("Page 4 of 6")).toBeVisible();

      // Leaving saves at once, without waiting for the scroll to settle.
      await page.goto(readerUrl(book.id, book.fileId), {
        waitUntil: "domcontentloaded",
      });
      await expect(page.getByText("Page 4 of 6")).toBeVisible();
      // The counter shows before the flow is known, and the pages after.
      await expect(pageImages(page)).toHaveCount(6);
      await expect.poll(() => pageTop(page, 4)).toBe(0);
    });
  });
});
