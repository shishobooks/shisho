import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const username = "cacheadmin";
const password = "password123";

const openCachePage = async (page: Page) => {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.waitForURL(/\/settings\/libraries|\/libraries\//);
  await page.goto("/settings/cache", { waitUntil: "domcontentloaded" });
};

// The limit controls of one cache, whose names include the cache's name.
const card = (page: Page, name: string) => ({
  size: page.getByRole("spinbutton", {
    name: `Maximum size (GiB) for ${name}`,
    exact: true,
  }),
  noLimit: page.getByRole("checkbox", {
    name: `No limit for ${name}`,
    exact: true,
  }),
  save: page.getByRole("button", {
    name: `Save limit for ${name}`,
    exact: true,
  }),
});

test.describe("Cache limit settings", () => {
  test.beforeAll(async ({ browser }) => {
    const api = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await api.delete("/api/test/users");
    await api.post("/api/test/users", { data: { username, password } });
    await api.dispose();
  });

  test("saves a fractional limit and keeps it after reloading", async ({
    page,
  }) => {
    await openCachePage(page);
    const covers = card(page, "Cover Thumbnails");
    await expect(covers.size).toBeVisible();
    await covers.size.fill("2.5");
    await covers.save.click();
    await expect(
      page.getByText("Cover Thumbnails cache limit saved", { exact: true }),
    ).toBeVisible();
    await expect(covers.size).toHaveValue("2.5");
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(covers.size).toHaveValue("2.5");
    await expect(covers.save).toBeDisabled();
  });

  test("sets a limit on a cache that had none and lifts it again", async ({
    page,
  }) => {
    await openCachePage(page);
    const pdf = card(page, "PDF Pages");
    await expect(pdf.noLimit).toBeChecked();
    await expect(pdf.size).toBeDisabled();
    await pdf.noLimit.click();
    await pdf.size.fill("3");
    await pdf.save.click();
    await expect(
      page.getByText("PDF Pages cache limit saved", { exact: true }),
    ).toBeVisible();
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(pdf.noLimit).not.toBeChecked();
    await expect(pdf.size).toHaveValue("3");

    await pdf.noLimit.click();
    // Reloading before the save lands could abort the request.
    const saved = page.waitForResponse(
      (res) =>
        res.url().endsWith("/api/settings/cache") &&
        res.request().method() === "PUT",
    );
    await pdf.save.click();
    expect((await saved).ok()).toBe(true);
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(pdf.noLimit).toBeChecked();
    await expect(pdf.size).toHaveValue("3");
  });

  test.afterAll(async ({ browser }) => {
    const api = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await api.post("/api/auth/login", { data: { username, password } });
    await api.put("/api/settings/cache", {
      data: {
        cover_thumbnails: { max_size_gb: 1, unlimited: false },
        pdf_pages: { max_size_gb: 5, unlimited: true },
      },
    });
    await api.delete("/api/test/users");
    await api.dispose();
  });
});
