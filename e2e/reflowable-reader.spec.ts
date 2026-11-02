/**
 * E2E tests for the Reflowable reader. foliate renders the book inside closed
 * shadow roots, so the tests read the book from the frame foliate creates
 * rather than through locators on the page.
 *
 * Running:
 *   pnpm e2e:chromium e2e/reflowable-reader.spec.ts
 */

import type { Frame, Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "reflowablereader";
const PASSWORD = "password123";

let libraryId: number;
let mobiBookId: number;
let epubBookId: number;
let resumeBookId: number;
let finishBookId: number;

async function login(page: Page) {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByLabel("Username").fill(USERNAME);
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(/\/admin\/libraries|\/libraries\//);
}

async function openReader(page: Page, bookId: number) {
  await page.goto(`/libraries/${libraryId}/books/${bookId}`, {
    waitUntil: "domcontentloaded",
  });
  await page.getByRole("link", { name: "Read", exact: true }).click();
  await page.waitForURL(/\/files\/\d+\/read$/);
}

// Waits for the frame showing text. foliate's modules load on first use,
// which a cold dev server compiles on demand, so this allows more than the
// default expect timeout.
async function bookFrame(
  page: Page,
  text: string,
  timeout = 20_000,
): Promise<Frame> {
  let found: Frame | undefined;
  await expect
    .poll(
      async () => {
        for (const frame of page.frames()) {
          const body = await frame
            .locator("body")
            .textContent({ timeout: 1000 })
            .catch(() => null);
          if (body?.includes(text)) {
            found = frame;
            return true;
          }
        }
        return false;
      },
      { timeout },
    )
    .toBe(true);
  return found!;
}

// Turns the page until text shows. foliate ignores a turn for a moment after
// the last one lands, which the new chapter's text can appear before, so a
// click can be dropped. Retrying can't skip a chapter: a click during a slow
// load is dropped the same way.
async function turnPageTo(page: Page, text: string) {
  const next = page.getByRole("button", { name: "Next page" });
  await expect(async () => {
    await next.click();
    await bookFrame(page, text, 2_000);
  }).toPass({ timeout: 30_000 });
}

const progress = async (page: Page) =>
  Number(
    await page
      .getByRole("slider", { name: "Reading progress" })
      .getAttribute("aria-valuenow"),
  );

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

    const seed = async (title: string, fileType: string) => {
      const resp = await apiContext.post("/api/test/books", {
        data: {
          libraryId,
          title,
          fileType,
          withFileOnDisk: true,
        },
      });
      expect(resp.ok()).toBe(true);
      return ((await resp.json()) as { id: number }).id;
    };
    mobiBookId = await seed("Kindle Only Book", "mobi");
    epubBookId = await seed("Styled Book", "epub");
    resumeBookId = await seed("Resume Book", "epub");
    finishBookId = await seed("Finish Book", "epub");
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
    const pageErrors: string[] = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));
    await login(page);
    await openReader(page, mobiBookId);

    await bookFrame(page, "Hello world.");
    await expect(page.getByText("Preparing book…")).toBeHidden();
    await expect(page.getByText("We couldn't load this book.")).toHaveCount(0);
    await expect(
      page.getByRole("slider", { name: "Reading progress" }),
    ).toBeVisible();
    expect(pageErrors).toEqual([]);
  });

  test("the Dark theme replaces the book's own colors", async ({ page }) => {
    await login(page);
    const put = await page.request.put("/api/settings/user", {
      data: {
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "paginated",
        viewer_hide_chrome: false,
      },
    });
    expect(put.ok()).toBe(true);
    await openReader(page, epubBookId);
    const frame = await bookFrame(page, "Test chapter.");

    await page.getByRole("button", { name: "Settings" }).click();
    await page.getByRole("button", { name: "Dark", exact: true }).click();

    const colors = () =>
      frame.evaluate(() => {
        const style = (selector: string) =>
          getComputedStyle(document.querySelector(selector)!);
        return {
          bodyBackground: style("body").backgroundColor,
          text: style("p").color,
          link: style("a").color,
        };
      });
    await expect.poll(colors).toEqual({
      bodyBackground: "rgba(0, 0, 0, 0)",
      text: "rgb(232, 232, 232)",
      link: "rgb(138, 180, 248)",
    });
  });

  // Paginated with the controls showing, so the page buttons are on screen.
  async function setPaginatedReader(page: Page) {
    const put = await page.request.put("/api/settings/user", {
      data: {
        viewer_reflowable_flow: "paginated",
        viewer_hide_chrome: false,
      },
    });
    expect(put.ok()).toBe(true);
  }

  test("reopens where you left off", async ({ page }) => {
    await login(page);
    await setPaginatedReader(page);
    await openReader(page, resumeBookId);
    await bookFrame(page, "Test chapter.");

    await turnPageTo(page, "Chapter Two text.");
    await turnPageTo(page, "Chapter Three text.");

    // Moves save a few seconds apart, so wait for the last one to be saved.
    const fileId = page.url().match(/\/files\/(\d+)\/read$/)![1];
    const percent = await page
      .getByRole("slider", { name: "Reading progress" })
      .getAttribute("aria-valuenow");
    await expect
      .poll(
        async () => {
          const resp = await page.request.get(
            `/api/reading/files/${fileId}/position`,
          );
          const body = (await resp.json()) as {
            position: { percentage: number } | null;
          };
          return body.position
            ? String(Math.round(body.position.percentage * 100))
            : null;
        },
        { timeout: 10_000 },
      )
      .toBe(percent);

    await page.reload({ waitUntil: "domcontentloaded" });
    await bookFrame(page, "Chapter Three text.");
    await expect(page.getByText("Preparing book…")).toBeHidden();
  });

  test("reading to the end marks the book read", async ({ page }) => {
    await login(page);
    await setPaginatedReader(page);
    await openReader(page, finishBookId);
    await bookFrame(page, "Test chapter.");

    await turnPageTo(page, "Chapter Two text.");
    await turnPageTo(page, "Chapter Three text.");
    await turnPageTo(page, "Chapter Four text.");

    await expect(page.getByText("Marked as read")).toBeVisible();
    await page.goto(`/libraries/${libraryId}/books/${finishBookId}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("button", { name: "Reading status: Read" }),
    ).toBeVisible();
  });

  test("moving the pointer over a scrolled book shows hidden controls", async ({
    page,
  }) => {
    await login(page);
    const put = await page.request.put("/api/settings/user", {
      data: {
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "scrolled",
        viewer_hide_chrome: true,
      },
    });
    expect(put.ok()).toBe(true);
    await openReader(page, epubBookId);
    const frame = await bookFrame(page, "Test chapter.");

    const settings = page.getByRole("button", { name: "Settings" });
    await expect(settings).not.toBeInViewport();

    // Move over the text itself: events there go to the book's frame, not to
    // the reader page around it.
    const box = (await frame.locator("p").first().boundingBox())!;
    await page.mouse.move(box.x + 5, box.y + box.height / 2);
    await page.mouse.move(box.x + 25, box.y + box.height / 2);
    await expect(settings).toBeInViewport();
  });

  test.describe("on a touch screen", () => {
    const WIDTH = 390;
    const HEIGHT = 844;
    test.use({ hasTouch: true, viewport: { width: WIDTH, height: HEIGHT } });

    const setReaderSettings = async (page: Page, hideChrome: boolean) => {
      const put = await page.request.put("/api/settings/user", {
        data: {
          viewer_reflowable_theme: "light",
          viewer_reflowable_flow: "paginated",
          viewer_hide_chrome: hideChrome,
        },
      });
      expect(put.ok()).toBe(true);
    };

    // Seeds a book for one test, since each book resumes where the last test
    // left it.
    const openNewBook = async (
      page: Page,
      seed: Record<string, unknown>,
      firstPageText = "Test chapter.",
    ) => {
      const resp = await page.request.post("/api/test/books", {
        data: {
          libraryId,
          title: "Phone Book",
          fileType: "epub",
          withFileOnDisk: true,
          ...seed,
        },
      });
      expect(resp.ok()).toBe(true);
      await openReader(page, ((await resp.json()) as { id: number }).id);
      const frame = await bookFrame(page, firstPageText);
      await expect(page.getByText("Preparing book…")).toBeHidden();
      return frame;
    };

    // Opens a long book a few pages in, so turning either way shows.
    const openMidBook = async (
      page: Page,
      hideChrome: boolean,
      seed: Record<string, unknown> = { epubParagraphs: 120 },
      firstPageText?: string,
    ) => {
      await login(page);
      await setReaderSettings(page, hideChrome);
      await openNewBook(page, seed, firstPageText);
      for (let i = 0; i < 3; i++) {
        const before = await progress(page);
        // foliate drops a turn for a moment after the last one lands.
        await expect(async () => {
          await page.keyboard.press("ArrowRight");
          expect(await progress(page)).toBeGreaterThan(before);
        }).toPass();
      }
      // Outlast that moment, so the test's own turn isn't the one dropped.
      await page.waitForTimeout(150);
      return progress(page);
    };

    test.describe("swiping", () => {
      // Playwright has no touch-drag API, and only Chromium speaks CDP.
      test.skip(
        ({ browserName }) => browserName !== "chromium",
        "Sends raw touch events over CDP",
      );

      // foliate turns on drag distance plus the speed of the last touch step.
      // A few large steps keep that speed up when a loaded machine stretches
      // the time between them; a slow, short drag rightly snaps back.
      const swipe = async (
        page: Page,
        fromX: number,
        toX: number,
        y = HEIGHT / 2,
      ) => {
        const cdp = await page.context().newCDPSession(page);
        const send = (
          type: "touchStart" | "touchMove" | "touchEnd",
          x?: number,
        ) =>
          cdp.send("Input.dispatchTouchEvent", {
            type,
            touchPoints: x === undefined ? [] : [{ x, y }],
          });
        await send("touchStart", fromX);
        const steps = 3;
        for (let step = 1; step <= steps; step++) {
          await send("touchMove", fromX + ((toX - fromX) * step) / steps);
          await page.waitForTimeout(16);
        }
        await send("touchEnd");
      };

      for (const [zone, x] of [
        ["left", WIDTH * 0.3],
        ["center", WIDTH * 0.5],
        ["right", WIDTH * 0.9],
      ] as const) {
        test(`a next-page swipe from the ${zone} third turns the page`, async ({
          page,
        }) => {
          const start = await openMidBook(page, false);
          await swipe(page, x, 2);
          await expect.poll(() => progress(page)).toBeGreaterThan(start);
        });
      }

      for (const [zone, x] of [
        ["left", WIDTH * 0.1],
        ["center", WIDTH * 0.5],
        ["right", WIDTH * 0.7],
      ] as const) {
        test(`a previous-page swipe from the ${zone} third turns the page`, async ({
          page,
        }) => {
          const start = await openMidBook(page, false);
          await swipe(page, x, WIDTH - 2);
          await expect.poll(() => progress(page)).toBeLessThan(start);
        });
      }

      test("a swipe saves the position, as a button turn does", async ({
        page,
      }) => {
        await openMidBook(page, false);
        // Let the saves from reaching this page land first.
        await page.waitForTimeout(3_500);
        const saved = page.waitForRequest(
          (request) =>
            request.method() === "POST" &&
            /\/reading\/files\/\d+\/position/.test(request.url()),
        );
        await swipe(page, WIDTH * 0.9, 2);
        await saved;
      });

      test.describe("in a fixed-layout book", () => {
        const openFixed = (page: Page, seed = {}) =>
          openMidBook(
            page,
            false,
            { fixedLayout: true, ...seed },
            "Fixed page 1.",
          );

        for (const [zone, x] of [
          ["left", WIDTH * 0.3],
          ["center", WIDTH * 0.5],
          ["right", WIDTH * 0.9],
        ] as const) {
          test(`a next-page swipe from the ${zone} third turns the page`, async ({
            page,
          }) => {
            const start = await openFixed(page);
            await swipe(page, x, 2);
            await expect.poll(() => progress(page)).toBeGreaterThan(start);
          });
        }

        for (const [zone, x] of [
          ["left", WIDTH * 0.1],
          ["center", WIDTH * 0.5],
          ["right", WIDTH * 0.7],
        ] as const) {
          test(`a previous-page swipe from the ${zone} third turns the page`, async ({
            page,
          }) => {
            const start = await openFixed(page);
            await swipe(page, x, WIDTH - 2);
            await expect.poll(() => progress(page)).toBeLessThan(start);
          });
        }

        test("a swipe on the blank space around the page turns it", async ({
          page,
        }) => {
          const start = await openFixed(page);
          // The header bar ends near y 50, and the page is centered below it.
          await swipe(page, WIDTH * 0.9, 2, 70);
          await expect.poll(() => progress(page)).toBeGreaterThan(start);
        });

        test("a swipe to the right moves forward when pages run right to left", async ({
          page,
        }) => {
          await login(page);
          await setReaderSettings(page, false);
          await openNewBook(
            page,
            { fixedLayout: true, fixedLayoutRTL: true },
            "Fixed page 1.",
          );
          const start = await progress(page);
          await swipe(page, WIDTH * 0.1, WIDTH - 2);
          await expect.poll(() => progress(page)).toBeGreaterThan(start);
        });

        test("a small drag doesn't turn the page", async ({ page }) => {
          const start = await openFixed(page);
          await swipe(page, WIDTH * 0.5, WIDTH * 0.5 - 20);
          await page.waitForTimeout(300);
          expect(await progress(page)).toBe(start);
        });
      });

      test("a swipe on the margin above the text turns the page", async ({
        page,
      }) => {
        const start = await openMidBook(page, false);
        // The header bar ends near y 50 and foliate's 48px margin follows it.
        await swipe(page, WIDTH * 0.9, 2, 70);
        await expect.poll(() => progress(page)).toBeGreaterThan(start);
      });
    });

    test("tapping the sides turns the page each way, and the middle doesn't", async ({
      page,
    }) => {
      const start = await openMidBook(page, false);

      await page.touchscreen.tap(WIDTH * 0.9, HEIGHT / 2);
      await expect.poll(() => progress(page)).toBeGreaterThan(start);
      const afterNext = await progress(page);

      await page.touchscreen.tap(WIDTH / 2, HEIGHT / 2);
      await page.waitForTimeout(300);
      expect(await progress(page)).toBe(afterNext);

      await page.touchscreen.tap(WIDTH * 0.1, HEIGHT / 2);
      await expect.poll(() => progress(page)).toBeLessThan(afterNext);
    });

    test("tapping a link follows it", async ({ page }) => {
      await login(page);
      await setReaderSettings(page, false);
      const frame = await openNewBook(page, { epubParagraphs: 120 });

      // The link goes to the chapter's end, far beyond a single page turn.
      const box = (await frame.locator("a").first().boundingBox())!;
      await page.touchscreen.tap(box.x + box.width / 2, box.y + box.height / 2);
      await expect.poll(() => progress(page)).toBeGreaterThan(50);
    });

    test("tapping crosses chapters both ways and keeps turning pages", async ({
      page,
    }) => {
      await login(page);
      await setReaderSettings(page, false);
      await openNewBook(page, {});

      await expect(async () => {
        await page.touchscreen.tap(WIDTH * 0.9, HEIGHT / 2);
        await bookFrame(page, "Chapter Four text.", 1_000);
      }).toPass({ timeout: 20_000 });

      await expect(async () => {
        await page.touchscreen.tap(WIDTH * 0.1, HEIGHT / 2);
        await bookFrame(page, "Chapter Two text.", 1_000);
      }).toPass({ timeout: 20_000 });

      await turnPageTo(page, "Chapter Three text.");
    });

    test("tapping a fixed-layout book turns its pages, wherever it's scaled to", async ({
      page,
    }) => {
      const start = await openMidBook(
        page,
        false,
        { fixedLayout: true },
        "Fixed page 1.",
      );

      await page.touchscreen.tap(WIDTH * 0.9, HEIGHT / 2);
      await expect.poll(() => progress(page)).toBeGreaterThan(start);
      const afterNext = await progress(page);

      await page.waitForTimeout(150);
      await page.touchscreen.tap(WIDTH / 2, HEIGHT / 2);
      await page.waitForTimeout(300);
      expect(await progress(page)).toBe(afterNext);

      await page.touchscreen.tap(WIDTH * 0.1, HEIGHT / 2);
      await expect.poll(() => progress(page)).toBeLessThan(afterNext);
    });

    test.describe("with two fixed-layout pages side by side", () => {
      const WIDE = { width: 1280, height: 720 };
      test.use({ viewport: WIDE });

      test("clicking follows the whole screen's thirds, not each page's", async ({
        page,
      }) => {
        await login(page);
        await setReaderSettings(page, false);
        await openNewBook(page, { fixedLayout: true }, "Fixed page 1.");
        // One spread in, with spreads left on both sides.
        const first = await progress(page);
        await expect(async () => {
          await page.keyboard.press("ArrowRight");
          expect(await progress(page)).toBeGreaterThan(first);
        }).toPass();
        await page.waitForTimeout(150);
        const start = await progress(page);

        // Inside the right-hand page, on its left half: the screen's middle
        // third, where nothing turns.
        await page.mouse.click(WIDE.width * 0.55, WIDE.height / 2);
        await page.waitForTimeout(300);
        expect(await progress(page)).toBe(start);

        await page.mouse.click(WIDE.width * 0.7, WIDE.height / 2);
        await expect.poll(() => progress(page)).toBeGreaterThan(start);
        const afterNext = await progress(page);

        await page.waitForTimeout(150);
        await page.mouse.click(WIDE.width * 0.3, WIDE.height / 2);
        await expect.poll(() => progress(page)).toBeLessThan(afterNext);
      });
    });

    test("tapping a link in a fixed-layout book follows it", async ({
      page,
    }) => {
      await login(page);
      await setReaderSettings(page, false);
      const frame = await openNewBook(
        page,
        { fixedLayout: true },
        "Fixed page 1.",
      );
      const start = await progress(page);

      const box = (await frame.locator("a").first().boundingBox())!;
      await page.touchscreen.tap(box.x + box.width / 2, box.y + box.height / 2);
      await expect.poll(() => progress(page)).toBeGreaterThan(start + 25);
    });

    test("with the controls hidden, a tap where the gear was brings them back", async ({
      page,
    }) => {
      const start = await openMidBook(page, true);
      const settings = page.getByRole("button", { name: "Settings" });
      await expect(settings).not.toBeInViewport();

      await page.touchscreen.tap(WIDTH - 30, 25);
      await expect(settings).toBeInViewport();
      expect(await progress(page)).toBe(start);
    });

    test("with auto-hide on, a tap in the middle brings back the controls", async ({
      page,
    }) => {
      await openMidBook(page, true);
      const settings = page.getByRole("button", { name: "Settings" });
      await expect(settings).not.toBeInViewport();

      await page.touchscreen.tap(WIDTH / 2, HEIGHT / 2);
      await expect(settings).toBeInViewport();
    });

    test("the arrow keys still work after a tap moves focus into the book", async ({
      page,
    }) => {
      const start = await openMidBook(page, false);
      await page.touchscreen.tap(WIDTH / 2, HEIGHT / 2);
      await page.keyboard.press("ArrowRight");
      await expect.poll(() => progress(page)).toBeGreaterThan(start);
    });
  });
});
