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
let settingsBookId: number;
let longBookId: number;
let contentsBookId: number;
let sectionsBookId: number;

async function login(page: Page) {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByLabel("Username").fill(USERNAME);
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(/\/admin\/libraries|\/libraries\//);
}

// Display settings are kept in the browser. login() lands on the app, so
// this stores them there before the reader opens.
async function setDisplaySettings(
  page: Page,
  settings: { theme?: string; layout?: string; pages?: string },
) {
  await page.evaluate(
    ([key, value]) => localStorage.setItem(key, value),
    ["shisho-reader-settings", JSON.stringify(settings)],
  );
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

    const seed = async (
      title: string,
      fileType: string,
      extra: Record<string, unknown> = {},
    ) => {
      const resp = await apiContext.post("/api/test/books", {
        data: {
          libraryId,
          title,
          fileType,
          withFileOnDisk: true,
          ...extra,
        },
      });
      expect(resp.ok()).toBe(true);
      return ((await resp.json()) as { id: number }).id;
    };
    mobiBookId = await seed("Kindle Only Book", "mobi");
    epubBookId = await seed("Styled Book", "epub");
    resumeBookId = await seed("Resume Book", "epub");
    finishBookId = await seed("Finish Book", "epub");
    settingsBookId = await seed("Settings Book", "epub");
    longBookId = await seed("Long Book", "epub", { epubParagraphs: 120 });
    contentsBookId = await seed("Contents Book", "epub", { epubToc: true });
    sectionsBookId = await seed("Sections Book", "epub", {
      epubSections: true,
    });
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
      data: { viewer_hide_chrome: false },
    });
    expect(put.ok()).toBe(true);
    await setDisplaySettings(page, { theme: "light", layout: "paginated" });
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

  test("justification leaves text the book centers alone", async ({ page }) => {
    await login(page);
    await setDisplaySettings(page, { layout: "paginated" });
    await openReader(page, epubBookId);
    const frame = await bookFrame(page, "Test chapter.");

    const alignments = () =>
      frame.evaluate(() =>
        [...document.querySelectorAll("p")]
          .slice(0, 2)
          .map((p) => getComputedStyle(p).textAlign),
      );
    await expect.poll(alignments).toEqual(["justify", "center"]);

    await page.getByRole("button", { name: "Settings" }).click();
    await page.getByRole("switch", { name: "Justify text" }).click();
    await expect.poll(alignments).toEqual(["start", "center"]);
  });

  test.describe("on a phone-sized screen", () => {
    test.use({ viewport: { width: 390, height: 844 } });

    test("a footnote's popover keeps clear of the screen's edges", async ({
      page,
    }) => {
      await login(page);
      await setPaginatedReader(page);
      await openReader(page, epubBookId);
      await bookFrame(page, "Test chapter.");
      await turnPageTo(page, "Chapter Two text.");
      const frame = await bookFrame(page, "Chapter Two text.");

      // The reference sits by the page's left edge.
      const reference = (await frame
        .locator('a[href="chapter3.xhtml#note1"]')
        .boundingBox())!;
      await page.mouse.click(
        reference.x + reference.width / 2,
        reference.y + reference.height / 2,
      );

      const popover = page.getByRole("dialog", { name: "Footnote" });
      await expect(popover).toBeVisible();
      const box = (await popover.boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(16);
      expect(box.x + box.width).toBeLessThanOrEqual(390 - 16);
    });
  });

  test.describe("on a wide screen", () => {
    test.use({ viewport: { width: 1600, height: 900 } });

    test("Pages shows two pages side by side on Auto, and one on One", async ({
      page,
    }) => {
      await login(page);
      await setDisplaySettings(page, { layout: "paginated" });
      await openReader(page, longBookId);
      const frame = await bookFrame(page, "Test chapter.");

      // The columns of text on screen: paragraphs wholly on screen start at
      // one left edge per page. The next page's column starts just past the
      // screen's edge, so a partial one doesn't count.
      const columns = async () => {
        const offset = (await (await frame.frameElement()).boundingBox())!.x;
        const boxes = await frame.locator("p").evaluateAll((ps) =>
          ps.map((p) => {
            const { left, right } = p.getBoundingClientRect();
            return { left, right };
          }),
        );
        return new Set(
          boxes
            .filter(
              ({ left, right }) => offset + left >= 0 && offset + right <= 1600,
            )
            .map(({ left }) => Math.round(offset + left)),
        ).size;
      };
      await expect.poll(columns).toBe(2);

      await page.getByRole("button", { name: "Settings" }).click();
      await page.getByRole("button", { name: "One", exact: true }).click();
      await expect.poll(columns).toBe(1);

      await page.getByRole("button", { name: "Auto", exact: true }).click();
      await expect.poll(columns).toBe(2);
    });
  });

  test("chapter marks name their chapters and go to their starts", async ({
    page,
  }) => {
    await login(page);
    await setPaginatedReader(page);
    await openReader(page, contentsBookId);
    await bookFrame(page, "Test chapter.");

    const bar = page.getByRole("slider", { name: "Reading progress" });
    const marks = bar.getByTestId("chapter-mark");
    // Chapter One starts the book, at the bar's own start.
    await expect(marks).toHaveCount(3);

    for (const [i, name] of ["Two", "Three", "Four"].entries()) {
      await marks.nth(i).hover();
      // The last mark's tooltip can still be fading out.
      await expect(
        page.getByRole("tooltip").filter({ hasText: `Chapter ${name}` }),
      ).toBeVisible();
      await marks.nth(i).click();
      await bookFrame(page, `Chapter ${name} text.`);
    }
    // Back to an earlier chapter's start, too.
    await marks.nth(0).click();
    await bookFrame(page, "Chapter Two text.");
  });

  test("going to a section lands on the page its heading is on", async ({
    page,
    browserName,
  }) => {
    await login(page);
    const put = await page.request.put("/api/settings/user", {
      data: { viewer_hide_chrome: false },
    });
    expect(put.ok()).toBe(true);
    // One page at a time, so the page a jump lands on is the one on screen.
    await setDisplaySettings(page, { layout: "paginated", pages: "one" });
    await openReader(page, sectionsBookId);
    const frame = await bookFrame(page, "Section 1 begins.");
    // The reader lays the book out again as it applies the display settings
    // once the book opens; a jump before then can be undone by it.
    await expect(page.getByText("Preparing book…")).toBeHidden();
    await expect(
      page
        .getByRole("slider", { name: "Reading progress" })
        .getByTestId("chapter-mark")
        .first(),
    ).toBeAttached();

    if (browserName === "firefox") {
      // The book holds what this guards against: sections whose header moved
      // to the next page, for which Firefox reports a box on the page before.
      const pushed = await frame.evaluate(
        () =>
          [...document.querySelectorAll("section")].filter((section) => {
            const first = [...section.getClientRects()].find(
              (r) => r.width > 0 && r.height > 0,
            );
            const header = section.querySelector("header")!;
            return (
              first !== undefined &&
              header.getBoundingClientRect().left - first.left > 1
            );
          }).length,
      );
      expect(pushed).toBeGreaterThan(0);
    }

    // Jumps go straight to foliate, as the chapter menu and marks do: this
    // is about where a jump lands, and other tests cover those controls.
    // To the table of contents entry with this label, by its link as
    // foliate resolved it.
    const goTo = (label: string) =>
      page.evaluate(async (wanted) => {
        const view = document.querySelector("foliate-view") as unknown as {
          book: { toc: Array<{ label: string; href: string }> };
          goTo: (href: string) => Promise<unknown>;
        };
        const entry = view.book.toc.find((e) => e.label === wanted);
        if (!entry) throw new Error(`No contents entry ${wanted}`);
        await view.goTo(entry.href);
      }, label);
    const onScreen = (locator: ReturnType<Frame["locator"]>) => async () => {
      const box = await locator.boundingBox();
      return (
        box !== null &&
        box.x >= 0 &&
        box.x + box.width <= page.viewportSize()!.width
      );
    };

    // The sections pkg/testutils/epub.go writes (sectionsEPUBCount).
    for (let n = 2; n <= 24; n++) {
      await goTo(`Section ${n}`);
      await expect
        .poll(
          onScreen(
            frame.getByRole("heading", { name: `Section ${n}`, exact: true }),
          ),
          { message: `Section ${n}'s heading is on screen` },
        )
        .toBe(true);
    }

    // A section that opens with a page-tall picture lands on the picture, not
    // on its text a page later. Its file opens in a frame of its own.
    await goTo("Figure");
    const figureFrame = await bookFrame(page, "The figure's section begins.");
    await expect
      .poll(onScreen(figureFrame.locator("#figure svg")), {
        message: "The figure is on screen",
      })
      .toBe(true);
  });

  test("a tap on the book closes the settings without turning the page", async ({
    page,
  }) => {
    await login(page);
    await setPaginatedReader(page);
    await openReader(page, longBookId);
    const frame = await bookFrame(page, "Test chapter.");
    // On screen while the reader is on the first page; a turn moves it off.
    const onFirstPage = async () => {
      const box = await frame.locator("p").first().boundingBox();
      return box !== null && box.x >= 0 && box.x < page.viewportSize()!.width;
    };
    await expect.poll(onFirstPage).toBe(true);

    await page.getByRole("button", { name: "Settings" }).click();
    const settings = page.getByRole("dialog");
    await expect(settings).toBeVisible();

    // The right third of the page, which turns the page when nothing is
    // open, left of the popover, which sits under the gear.
    const box = (await frame.locator("p").first().boundingBox())!;
    const popover = (await settings.boundingBox())!;
    const x = page.viewportSize()!.width * 0.7;
    expect(x).toBeLessThan(popover.x);
    await page.mouse.click(x, box.y + box.height / 2);

    await expect(settings).toBeHidden();
    // A turn lands a moment after the tap, so give it time to show.
    await page.waitForTimeout(500);
    expect(await onFirstPage()).toBe(true);

    // With nothing open, the same tap turns the page.
    await page.mouse.click(x, box.y + box.height / 2);
    await expect.poll(onFirstPage).toBe(false);
  });

  test("Force font size sets text the book fixes in px", async ({ page }) => {
    await login(page);
    await setDisplaySettings(page, { layout: "paginated" });
    await openReader(page, epubBookId);
    const frame = await bookFrame(page, "Fixed size.");

    const sizes = () =>
      frame.evaluate(() =>
        [...document.querySelectorAll("p")]
          .slice(0, 3)
          .map((p) => getComputedStyle(p).fontSize),
      );
    await expect.poll(sizes).toEqual(["16px", "16px", "11px"]);

    await page.getByRole("button", { name: "Settings" }).click();
    await page.getByRole("switch", { name: "Force font size" }).click();
    await page.getByRole("button", { name: "Increase font size" }).click();
    await expect.poll(sizes).toEqual(["16.5px", "16.5px", "16.5px"]);
  });

  // Paginated with the controls showing, so the page buttons are on screen.
  async function setPaginatedReader(page: Page) {
    const put = await page.request.put("/api/settings/user", {
      data: { viewer_hide_chrome: false },
    });
    expect(put.ok()).toBe(true);
    await setDisplaySettings(page, { layout: "paginated" });
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
            ? String(Math.round(body.position.percentage * 1000) / 10)
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

  test("display settings and the place in the book survive a reload, and footnotes open in place", async ({
    page,
  }) => {
    await login(page);
    await setPaginatedReader(page);
    await openReader(page, settingsBookId);
    await bookFrame(page, "Test chapter.");

    await page.getByRole("button", { name: "Settings" }).click();
    await page.getByRole("combobox", { name: "Font" }).click();
    await page.getByRole("option", { name: "OpenDyslexic" }).click();
    await page.getByRole("button", { name: "Black", exact: true }).click();
    const size = page.getByRole("slider", { name: "Font size" });
    await size.focus();
    await size.press("ArrowRight");
    await size.press("ArrowRight");
    await expect(size).toHaveAttribute("aria-valuenow", "17");
    await page.keyboard.press("Escape");

    await turnPageTo(page, "Chapter Two text.");
    // Moves save a few seconds apart, so wait for this one to be saved.
    const fileId = page.url().match(/\/files\/(\d+)\/read$/)![1];
    await expect
      .poll(
        async () => {
          const resp = await page.request.get(
            `/api/reading/files/${fileId}/position`,
          );
          const body = (await resp.json()) as {
            position: { cfi?: string } | null;
          };
          return body.position?.cfi ?? null;
        },
        { timeout: 10_000 },
      )
      .toMatch(/^epubcfi\(\/6\/4/);

    await page.reload({ waitUntil: "domcontentloaded" });
    const frame = await bookFrame(page, "Chapter Two text.");
    await expect(page.getByText("Preparing book…")).toBeHidden();

    // The book's frame shows the settings, in the bundled font it loaded.
    const looks = () =>
      frame.evaluate(async () => {
        await document.fonts.ready;
        const html = getComputedStyle(document.documentElement);
        return {
          font: getComputedStyle(document.querySelector("p")!).fontFamily,
          fontLoaded: [...document.fonts].some(
            (f) => f.family === "OpenDyslexic" && f.status === "loaded",
          ),
          background: html.backgroundColor,
          fontSize: getComputedStyle(document.querySelector("p")!).fontSize,
        };
      });
    await expect.poll(looks).toEqual({
      font: "OpenDyslexic, sans-serif",
      fontLoaded: true,
      background: "rgb(0, 0, 0)",
      fontSize: "17px",
    });

    await page.getByRole("button", { name: "Settings" }).click();
    await expect(page.getByRole("combobox", { name: "Font" })).toHaveText(
      "OpenDyslexic",
    );
    await expect(
      page.getByRole("button", { name: "Black", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(
      page.getByRole("slider", { name: "Font size" }),
    ).toHaveAttribute("aria-valuenow", "17");
    await page.keyboard.press("Escape");

    const before = await progress(page);
    const reference = (await frame
      .locator('a[href="chapter3.xhtml#note1"]')
      .boundingBox())!;
    await page.mouse.click(
      reference.x + reference.width / 2,
      reference.y + reference.height / 2,
    );
    const popover = page.getByRole("dialog", { name: "Footnote" });
    await expect(popover).toContainText("The footnote's text.");
    // The note looks like the page: the Black theme, in the chosen font,
    // which this page loads to draw it.
    await expect(popover).toHaveCSS("background-color", "rgb(0, 0, 0)");
    await expect(popover).toHaveCSS("font-family", /Shisho Note Font/);
    await expect
      .poll(() =>
        page.evaluate(() =>
          [...document.fonts].some(
            (f) =>
              f.family.includes("Shisho Note Font") && f.status === "loaded",
          ),
        ),
      )
      .toBe(true);
    await expect(
      popover.getByRole("button", { name: "Go to note" }),
    ).toBeVisible();
    // The note is in the next chapter, and the reference opened it in place
    // rather than jumping there.
    expect(await progress(page)).toBe(before);
  });

  test("moving the pointer over a scrolled book shows hidden controls", async ({
    page,
  }) => {
    await login(page);
    const put = await page.request.put("/api/settings/user", {
      data: { viewer_hide_chrome: true },
    });
    expect(put.ok()).toBe(true);
    await setDisplaySettings(page, { theme: "light", layout: "scrolled" });
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
        data: { viewer_hide_chrome: hideChrome },
      });
      expect(put.ok()).toBe(true);
      await setDisplaySettings(page, { theme: "light", layout: "paginated" });
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

      test("a next-page swipe from any third turns the page", async ({
        page,
      }) => {
        let last = await openMidBook(page, false);
        for (const x of [0.3, 0.5, 0.9]) {
          await swipe(page, WIDTH * x, 2);
          await expect.poll(() => progress(page)).toBeGreaterThan(last);
          last = await progress(page);
          await page.waitForTimeout(150);
        }
      });

      test("a previous-page swipe from any third turns the page", async ({
        page,
      }) => {
        let last = await openMidBook(page, false);
        for (const x of [0.1, 0.5, 0.7]) {
          await swipe(page, WIDTH * x, WIDTH - 2);
          await expect.poll(() => progress(page)).toBeLessThan(last);
          last = await progress(page);
          await page.waitForTimeout(150);
        }
      });

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
            { fixedLayout: true, fixedLayoutPages: 14, ...seed },
            "Fixed page 1.",
          );

        test("a next-page swipe from any third turns the page", async ({
          page,
        }) => {
          let last = await openFixed(page);
          for (const x of [0.3, 0.5, 0.9]) {
            await swipe(page, WIDTH * x, 2);
            await expect.poll(() => progress(page)).toBeGreaterThan(last);
            last = await progress(page);
            await page.waitForTimeout(150);
          }
        });

        test("a previous-page swipe from any third turns the page", async ({
          page,
        }) => {
          let last = await openFixed(page);
          for (const x of [0.1, 0.5, 0.7]) {
            await swipe(page, WIDTH * x, WIDTH - 2);
            await expect.poll(() => progress(page)).toBeLessThan(last);
            last = await progress(page);
            await page.waitForTimeout(150);
          }
        });

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

    test.describe("reading a fixed-layout book to its end", () => {
      const footerPage = (page: Page, pageNumber: number) =>
        page.locator("footer").getByText(`Page ${pageNumber}`, { exact: true });

      // The footer's buttons turn by side, so in a book running right to left
      // the left one moves forward and is labelled "Next page".
      const turnToPage = async (page: Page, pageNumber: number) => {
        await page.getByRole("button", { name: "Next page" }).click();
        await expect(footerPage(page, pageNumber)).toBeVisible();
      };

      const open = async (page: Page, seed: Record<string, unknown>) => {
        await login(page);
        await setReaderSettings(page, false);
        await openNewBook(
          page,
          { fixedLayout: true, ...seed },
          "Fixed page 1.",
        );
      };

      // Everything the reader has told the server was the end of the book.
      const watchFinishes = (page: Page) => {
        const finishes: string[] = [];
        page.on("request", (request) => {
          if (
            request.method() === "POST" &&
            /\/reading\/files\/\d+\/position/.test(request.url()) &&
            (request.postDataJSON() as { at_end?: boolean }).at_end
          ) {
            finishes.push(request.url());
          }
        });
        return finishes;
      };

      // A finish saves at once, so a wrong one is on its way by now.
      const expectNotFinished = async (page: Page, finishes: string[]) => {
        await page.waitForTimeout(500);
        expect(finishes).toEqual([]);
      };

      test("the last page marks it read, and not the page before", async ({
        page,
      }) => {
        const finishes = watchFinishes(page);
        await open(page, {});
        for (const pageNumber of [2, 3, 4, 5, 6, 7]) {
          await turnToPage(page, pageNumber);
        }
        await expectNotFinished(page, finishes);

        await turnToPage(page, 8);
        await expect(page.getByText("Marked as read")).toBeVisible();

        const bookPage = page.url().replace(/\/files\/\d+\/read$/, "");
        await page.goto(bookPage, { waitUntil: "domcontentloaded" });
        await expect(
          page.getByRole("button", { name: "Reading status: Read" }),
        ).toBeVisible();
      });

      // Seven pages end on a two-page spread, showing one page at a time.
      for (const [direction, seed] of [
        ["left to right", { fixedLayoutPages: 7 }],
        ["right to left", { fixedLayoutPages: 7, fixedLayoutRTL: true }],
      ] as const) {
        test(`the last spread's later page marks it read, ${direction}`, async ({
          page,
        }) => {
          const finishes = watchFinishes(page);
          await open(page, seed);
          for (const pageNumber of [2, 3, 4, 5, 6]) {
            await turnToPage(page, pageNumber);
          }
          await expectNotFinished(page, finishes);

          await turnToPage(page, 7);
          await expect(page.getByText("Marked as read")).toBeVisible();
        });
      }

      test.describe("with two pages side by side", () => {
        test.use({ viewport: { width: 1280, height: 720 } });

        test("reaching the last spread marks it read", async ({ page }) => {
          const finishes = watchFinishes(page);
          await open(page, { fixedLayoutPages: 7 });
          const next = page.getByRole("button", { name: "Next page" });
          // The spreads are 1, 2-3, 4-5, and 6-7.
          for (const before of [1, 2]) {
            await next.click();
            await expect(footerPage(page, before * 2)).toBeVisible();
          }
          await expectNotFinished(page, finishes);

          await next.click();
          await expect(page.getByText("Marked as read")).toBeVisible();
        });
      });

      // A jump within the spread on screen, which a page turn doesn't take.
      test("jumping to the last page from the one before marks it read", async ({
        page,
      }) => {
        const finishes = watchFinishes(page);
        await open(page, { fixedLayoutPages: 7 });
        for (const pageNumber of [2, 3, 4, 5, 6]) {
          await turnToPage(page, pageNumber);
        }
        await expectNotFinished(page, finishes);

        await page.getByRole("slider", { name: "Reading progress" }).focus();
        await page.keyboard.press("End");
        await expect(footerPage(page, 7)).toBeVisible();
        await expect(page.getByText("Marked as read")).toBeVisible();
      });

      test("reopening resumes on the page you left", async ({ page }) => {
        await open(page, {});
        for (const pageNumber of [2, 3, 4]) {
          await turnToPage(page, pageNumber);
        }
        const fileId = page.url().match(/\/files\/(\d+)\/read$/)![1];
        await expect
          .poll(
            async () => {
              const resp = await page.request.get(
                `/api/reading/files/${fileId}/position`,
              );
              const body = (await resp.json()) as {
                position: { cfi: string } | null;
              };
              return body.position !== null;
            },
            { timeout: 10_000 },
          )
          .toBe(true);

        await page.reload({ waitUntil: "domcontentloaded" });
        await expect(footerPage(page, 4)).toBeVisible({ timeout: 20_000 });
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
