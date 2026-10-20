/**
 * E2E flow for Devices and Delivery Rules: a reader adds a Device, sees on a
 * book's page what it gets, gives it its own rules, and the book page and the
 * Device's eReader browser downloads both change to follow them.
 *
 * Running:
 *   pnpm e2e:chromium e2e/device-delivery-rules.spec.ts
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "devicerulesreader";
const PASSWORD = "password123";
const DEVICE = "E2E Reader";

let libraryId: number;
let bookId: number;

test.describe("Device Delivery Rules", () => {
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
      data: { name: "Device Rules Library" },
    });
    libraryId = ((await libraryResp.json()) as { id: number }).id;
    // A real EPUB on disk, so the KePub Conversion can run.
    const bookResp = await apiContext.post("/api/test/books", {
      data: {
        libraryId,
        title: "Rules Follow The Device",
        withFileOnDisk: true,
      },
    });
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

  async function login(page: Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
  }

  const bookPage = (page: Page) =>
    page.goto(`/libraries/${libraryId}/books/${bookId}`, {
      waitUntil: "domcontentloaded",
    });

  // The Device's row in the book page's "On your Devices" panel.
  const panelRow = (page: Page) =>
    page.getByRole("listitem", { name: DEVICE, exact: true });

  test("a Device's own rules change what the book page and its downloads deliver", async ({
    page,
  }) => {
    await login(page);

    // Add a Browser Device, which follows the default: EPUB as EPUB.
    await page.goto("/user/devices", { waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "Add Device" }).first().click();
    const dialog = page.getByRole("dialog", { name: "Add Device" });
    await dialog.getByLabel("Name").fill(DEVICE);
    await dialog.getByRole("radio", { name: /^Browser/ }).click();
    await dialog.getByRole("button", { name: "Add Device" }).click();
    await page.waitForURL(/\/user\/devices\/[^/]+$/);
    const devicePage = page.url();
    const credential = (await page
      .getByLabel("Credential", { exact: true })
      .textContent())!;
    expect(credential).toMatch(/^[a-z]{16}$/);

    await bookPage(page);
    await expect(
      panelRow(page).getByRole("img", { name: "EPUB as EPUB" }),
    ).toBeVisible();

    const ereaderBook = `/ereader/key/${credential}/download/${bookId}`;
    await page.goto(ereaderBook);
    await expect(
      page.getByRole("link", { name: "Download EPUB" }),
    ).toBeVisible();

    // Give the Device its own rules: EPUB as KePub.
    await page.goto(devicePage, { waitUntil: "domcontentloaded" });
    await page
      .getByRole("group", { name: "Delivery Rules" })
      .getByRole("button", { name: "Custom" })
      .click();
    await page.getByRole("combobox", { name: "Deliver EPUB as" }).click();
    await page.getByRole("option", { name: "KePub", exact: true }).click();
    await expect(
      page.getByRole("status", { name: "What this Device receives" }),
    ).toContainText("Receives 1 book.");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "Save", exact: true }),
    ).toBeDisabled();

    await bookPage(page);
    await expect(
      panelRow(page).getByRole("img", { name: "EPUB as KePub" }),
    ).toBeVisible();

    await page.goto(ereaderBook);
    const download = page.getByRole("link", { name: "Download KePub" });
    await expect(download).toBeVisible();
    const response = await page.request.get(
      (await download.getAttribute("href"))!,
    );
    expect(response.status()).toBe(200);
    expect(response.headers()["content-disposition"]).toContain(".kepub.epub");
  });
});
