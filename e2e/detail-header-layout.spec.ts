/**
 * E2E tests for the responsive header on the series, person (and other
 * metadata resource), and list detail pages.
 *
 * Below `sm` the title takes its own row above the actions, which show only
 * their icons; from `sm` up the title and actions share one row. Layout and
 * the phone-width accessible names depend on real CSS, which jsdom lacks.
 *
 * Running:
 *   pnpm e2e:chromium e2e/detail-header-layout.spec.ts
 */

import type { Locator, Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "headertest";
const PASSWORD = "password123";

// Several words, each short enough to fit a phone row, so the title should
// wrap only between words.
const LONG_NAME =
  "The Extraordinarily Unforgettable Chronicles of Interdimensional Wanderers";

let libraryId: number;

test.describe("Detail page headers", () => {
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
      data: { name: "Header Test Library" },
    });
    libraryId = ((await libraryResp.json()) as { id: number }).id;

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

  async function login(page: Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
  }

  const box = async (locator: Locator) => {
    const result = await locator.boundingBox();
    if (!result) throw new Error("element has no bounding box");
    return result;
  };

  // Finds words split across lines: a word broken mid-word renders as more
  // than one line box.
  const brokenWords = (heading: Locator) =>
    heading.evaluate((el) => {
      const text = el.firstChild;
      if (!text || text.nodeType !== Node.TEXT_NODE) {
        throw new Error("heading has no text node");
      }
      const broken: string[] = [];
      for (const match of (text.textContent ?? "").matchAll(/\S+/g)) {
        const range = document.createRange();
        range.setStart(text, match.index);
        range.setEnd(text, match.index + match[0].length);
        if (range.getClientRects().length > 1) broken.push(match[0]);
      }
      return broken;
    });

  async function expectPhoneHeader(page: Page, actions: string[]) {
    const heading = page.getByRole("heading", { level: 1, name: LONG_NAME });
    await expect(heading).toBeVisible();

    // The labels are hidden on a phone, so these names come from aria-label.
    const buttons = actions.map((name) =>
      page.getByRole("button", { exact: true, name }),
    );
    for (const button of buttons) {
      await expect(button).toBeVisible();
    }

    const title = await box(heading);
    for (const button of buttons) {
      const action = await box(button);
      expect(action.y).toBeGreaterThanOrEqual(title.y + title.height);
    }
    // The title gets the content width, not what is left beside the actions.
    expect(title.width).toBeGreaterThan(250);
    expect(await brokenWords(heading)).toEqual([]);
    expect(
      await page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      ),
    ).toBe(true);
  }

  async function expectDesktopHeader(page: Page, actions: string[]) {
    const heading = page.getByRole("heading", { level: 1, name: LONG_NAME });
    const title = await box(heading);
    for (const name of actions) {
      const action = await box(page.getByRole("button", { exact: true, name }));
      expect(action.y).toBeLessThan(title.y + title.height);
      expect(action.x).toBeGreaterThanOrEqual(title.x + title.width);
    }
  }

  test("series header stacks on a phone and shares a row on desktop", async ({
    page,
    apiContext,
  }) => {
    const seriesResp = await apiContext.post("/api/test/series", {
      data: { libraryId, name: LONG_NAME },
    });
    const series = (await seriesResp.json()) as { id: number };
    const actions = ["Edit", "Merge", "Delete"];

    await login(page);
    await page.setViewportSize({ width: 375, height: 800 });
    await page.goto(`/libraries/${libraryId}/series/${series.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expectPhoneHeader(page, actions);

    await page.setViewportSize({ width: 1280, height: 800 });
    await expectDesktopHeader(page, actions);
  });

  test("person header stacks on a phone and shares a row on desktop", async ({
    page,
    apiContext,
  }) => {
    const personResp = await apiContext.post("/api/test/persons", {
      data: { libraryId, name: LONG_NAME },
    });
    const person = (await personResp.json()) as { id: number };
    const actions = ["Edit", "Merge", "Delete"];

    await login(page);
    await page.setViewportSize({ width: 375, height: 800 });
    await page.goto(`/libraries/${libraryId}/people/${person.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expectPhoneHeader(page, actions);

    await page.setViewportSize({ width: 1280, height: 800 });
    await expectDesktopHeader(page, actions);
  });

  test("list header stacks on a phone and shares a row on desktop", async ({
    page,
  }) => {
    await login(page);
    const listResp = await page.request.post("/api/lists", {
      data: { name: LONG_NAME },
    });
    expect(listResp.ok()).toBe(true);
    const list = (await listResp.json()) as { id: number };
    const actions = ["Edit", "Share", "Delete"];

    await page.setViewportSize({ width: 375, height: 800 });
    await page.goto(`/lists/${list.id}`, { waitUntil: "domcontentloaded" });
    await expectPhoneHeader(page, actions);

    await page.setViewportSize({ width: 1280, height: 800 });
    await expectDesktopHeader(page, actions);
  });
});
