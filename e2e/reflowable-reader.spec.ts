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

async function login(page: Page) {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByLabel("Username").fill(USERNAME);
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(/\/settings\/libraries|\/libraries\//);
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
async function bookFrame(page: Page, text: string): Promise<Frame> {
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
      { timeout: 20_000 },
    )
    .toBe(true);
  return found!;
}

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
        data: { libraryId, title, fileType, withFileOnDisk: true },
      });
      expect(resp.ok()).toBe(true);
      return ((await resp.json()) as { id: number }).id;
    };
    mobiBookId = await seed("Kindle Only Book", "mobi");
    epubBookId = await seed("Styled Book", "epub");
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
});
