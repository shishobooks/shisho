import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";
import {
  PLUGIN_TEST_PASSWORD,
  PLUGIN_TEST_USERNAME,
  seedPlugin,
} from "./plugin-helpers";

// The shared plugin login helper expects the no-library redirect; this
// suite seeds a library, so the landing page differs.
async function login(page: Page) {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByLabel("Username").fill(PLUGIN_TEST_USERNAME);
  await page.getByLabel("Password").fill(PLUGIN_TEST_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(/\/settings\/libraries|\/libraries\//);
}

// The fixture enricher proposes a title and an explicit unabridged flag.
// Applying that proposal must store abridged = false with the plugin's
// source: an explicit false is a value, not a clear.
test.describe("Identify", () => {
  let libraryId: number;
  let bookId: number;
  let fileId: number;

  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/api/test/plugins?include_official=true");
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: PLUGIN_TEST_USERNAME, password: PLUGIN_TEST_PASSWORD },
    });
    const libraryResp = await apiContext.post("/api/test/libraries", {
      data: { name: "Identify Library" },
    });
    libraryId = ((await libraryResp.json()) as { id: number }).id;
    await seedPlugin(apiContext, { scope: "test", id: "fixture" });
    await apiContext.dispose();
  });

  // A fresh book per test: once the proposal has been applied, the Abridged
  // row is unchanged and hidden, so a reused book cannot exercise it.
  test.beforeEach(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    const bookResp = await apiContext.post("/api/test/books", {
      data: { libraryId, title: "Identify Me", fileType: "epub" },
    });
    const book = (await bookResp.json()) as { id: number; fileId: number };
    bookId = book.id;
    fileId = book.fileId;
    await apiContext.dispose();
  });

  test.afterAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/api/test/plugins?include_official=true");
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.dispose();
  });

  test("applies a proposed unabridged flag as a plugin value", async ({
    page,
  }) => {
    await login(page);
    await page.goto(`/libraries/${libraryId}/books/${bookId}`, {
      waitUntil: "domcontentloaded",
    });

    await page.getByRole("button", { name: "Book actions" }).click();
    await page.getByRole("menuitem", { name: "Identify book" }).click();

    // The dialog searches on open; pick the fixture's result.
    await page.getByRole("button", { name: /Fixture Title/ }).click();

    // The proposal is unabridged and is applied by default. Toggle away and
    // back so the applied value is a restored proposal, not the default.
    const abridged = page.getByRole("combobox", { name: "Abridged value" });
    await expect(abridged).toHaveText("Unabridged");
    await abridged.click();
    await page.getByRole("option", { name: "Abridged", exact: true }).click();
    await expect(abridged).toHaveText("Abridged");
    await abridged.click();
    await page.getByRole("option", { name: "Unabridged" }).click();
    await expect(abridged).toHaveText("Unabridged");

    await page
      .getByRole("button", { name: /^Apply (\d+ )?changes?$/i })
      .click();
    await expect(page.getByRole("dialog")).toBeHidden();

    const resp = await page.request.get(`/api/books/${bookId}`);
    expect(resp.ok()).toBeTruthy();
    const book = (await resp.json()) as {
      title: string;
      files: Array<{
        id: number;
        abridged: boolean | null;
        abridged_source: string | null;
      }>;
    };
    const file = book.files.find((f) => f.id === fileId);
    expect(file).toBeDefined();
    expect(file?.abridged).toBe(false);
    expect(file?.abridged_source).toBe("plugin:test/fixture");
    expect(book.title).toBe("Fixture Title");
  });
});
