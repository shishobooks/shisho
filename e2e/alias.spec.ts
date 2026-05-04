/**
 * E2E tests for alias workflows.
 *
 * Covers:
 * 1. Adding aliases via the edit dialog — persists and is visible on reopen.
 * 2. Removing an alias via the edit dialog — no longer present after save.
 * 3. Merging two series — source name becomes alias of the target.
 * 4. Renaming a series — old name becomes alias.
 * 5. Autocomplete search matches aliases — searching by alias in the book
 *    edit dialog returns the canonical resource.
 *
 * Running:
 *   pnpm e2e:chromium e2e/alias.spec.ts
 *   mise test:e2e -- alias
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

interface TestData {
  libraryId: number;
}

let testData: TestData;

const USERNAME = "aliastest";
const PASSWORD = "password123";

test.describe("Alias workflows", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });

    await apiContext.delete("/test/ereader");
    await apiContext.delete("/test/users");

    await apiContext.post("/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });

    const libraryResp = await apiContext.post("/test/libraries", {
      data: { name: "Alias Test Library" },
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

  async function login(page: Page) {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
  }

  test("adding aliases via edit dialog persists and is visible when reopened", async ({
    page,
    apiContext,
  }) => {
    // Create a series via test API.
    const seriesResp = await apiContext.post("/test/series", {
      data: { libraryId: testData.libraryId, name: "Fantasy Saga" },
    });
    const series = (await seriesResp.json()) as { id: number };

    await login(page);
    await page.goto(`/libraries/${testData.libraryId}/series/${series.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.locator("h1").filter({ hasText: "Fantasy Saga" }),
    ).toBeVisible();

    // Open edit dialog.
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByRole("heading", { name: "Edit Series" }),
    ).toBeVisible();

    // Type an alias and press Enter to add it.
    await dialog.getByLabel("Aliases").fill("FS");
    await dialog.getByLabel("Aliases").press("Enter");

    // Badge should appear inside the dialog.
    await expect(dialog.getByText("FS")).toBeVisible();

    // Add a second alias.
    await dialog.getByLabel("Aliases").fill("The Fantasy Saga");
    await dialog.getByLabel("Aliases").press("Enter");
    await expect(dialog.getByText("The Fantasy Saga")).toBeVisible();

    // Save — wait for the PATCH response.
    await Promise.all([
      page.waitForResponse(
        (r) =>
          r.url().includes(`/api/series/${series.id}`) &&
          r.request().method() === "PATCH" &&
          r.ok(),
      ),
      dialog.getByRole("button", { name: "Save" }).click(),
    ]);

    // Reopen the edit dialog to verify aliases persisted.
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const dialog2 = page.getByRole("dialog");
    await expect(
      dialog2.getByRole("heading", { name: "Edit Series" }),
    ).toBeVisible();
    await expect(dialog2.getByText("FS")).toBeVisible();
    await expect(dialog2.getByText("The Fantasy Saga")).toBeVisible();

    // Also verify via API.
    const resp = await page.request.get(`/api/series/${series.id}`);
    const data = (await resp.json()) as { aliases: string[] };
    expect(data.aliases).toContain("FS");
    expect(data.aliases).toContain("The Fantasy Saga");
  });

  test("removing an alias via edit dialog removes it", async ({
    page,
    apiContext,
  }) => {
    // Create a series.
    const seriesResp = await apiContext.post("/test/series", {
      data: { libraryId: testData.libraryId, name: "Mystery Novels" },
    });
    const series = (await seriesResp.json()) as { id: number };

    await login(page);

    // Add aliases via API so we have something to remove.
    const patchResp = await page.request.patch(`/api/series/${series.id}`, {
      data: { name: "Mystery Novels", aliases: ["MN", "Whodunit Collection"] },
    });
    expect(patchResp.ok()).toBeTruthy();

    // Navigate to series detail and open edit dialog.
    await page.goto(`/libraries/${testData.libraryId}/series/${series.id}`, {
      waitUntil: "domcontentloaded",
    });
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByRole("heading", { name: "Edit Series" }),
    ).toBeVisible();

    // Both aliases should be present inside the dialog.
    await expect(dialog.getByText("MN")).toBeVisible();
    await expect(dialog.getByText("Whodunit Collection")).toBeVisible();

    // Remove "MN" by clicking its X button.
    await dialog.getByRole("button", { name: "Remove alias MN" }).click();
    await expect(dialog.getByText("MN")).not.toBeVisible();

    // Save.
    await Promise.all([
      page.waitForResponse(
        (r) =>
          r.url().includes(`/api/series/${series.id}`) &&
          r.request().method() === "PATCH" &&
          r.ok(),
      ),
      page.getByRole("button", { name: "Save" }).click(),
    ]);

    // Verify via API.
    const resp = await page.request.get(`/api/series/${series.id}`);
    const data = (await resp.json()) as { aliases: string[] };
    expect(data.aliases).not.toContain("MN");
    expect(data.aliases).toContain("Whodunit Collection");
  });

  test("merging series transfers source name as alias to target", async ({
    page,
    apiContext,
  }) => {
    // Create target and source series, each with a book so merge makes sense.
    const targetResp = await apiContext.post("/test/series", {
      data: { libraryId: testData.libraryId, name: "Dune Chronicles" },
    });
    const target = (await targetResp.json()) as { id: number };

    const sourceResp = await apiContext.post("/test/series", {
      data: { libraryId: testData.libraryId, name: "Dune Saga" },
    });
    const source = (await sourceResp.json()) as { id: number };

    // Create books linked to each series.
    await apiContext.post("/test/books", {
      data: {
        libraryId: testData.libraryId,
        title: "Dune",
        fileType: "epub",
        seriesId: target.id,
      },
    });
    await apiContext.post("/test/books", {
      data: {
        libraryId: testData.libraryId,
        title: "Dune Messiah",
        fileType: "epub",
        seriesId: source.id,
      },
    });

    await login(page);
    await page.goto(`/libraries/${testData.libraryId}/series/${target.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.locator("h1").filter({ hasText: "Dune Chronicles" }),
    ).toBeVisible();

    // Open merge dialog.
    await page.getByRole("button", { name: "Merge", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: /Merge into/ }),
    ).toBeVisible();

    // Open the source entity combobox and select "Dune Saga".
    await page.getByRole("combobox").click();
    await page.getByPlaceholder("Search series...").fill("Dune Saga");

    // Wait for search results to load and select the source.
    await expect(
      page.getByText("Dune Saga", { exact: false }).first(),
    ).toBeVisible();
    // Click the command item — use the one inside the command list.
    await page.locator("[cmdk-item]").filter({ hasText: "Dune Saga" }).click();

    // Confirm merge.
    await Promise.all([
      page.waitForResponse(
        (r) =>
          r.url().includes(`/api/series/${target.id}/merge`) &&
          r.request().method() === "POST" &&
          r.ok(),
      ),
      page.getByRole("button", { name: "Merge", exact: true }).last().click(),
    ]);

    // Open edit dialog to verify "Dune Saga" is now an alias.
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByRole("heading", { name: "Edit Series" }),
    ).toBeVisible();
    await expect(dialog.getByText("Dune Saga")).toBeVisible();

    // Verify via API.
    const resp = await page.request.get(`/api/series/${target.id}`);
    const data = (await resp.json()) as { aliases: string[] };
    expect(data.aliases).toContain("Dune Saga");
  });

  test("renaming a series adds old name as alias", async ({
    page,
    apiContext,
  }) => {
    const seriesResp = await apiContext.post("/test/series", {
      data: { libraryId: testData.libraryId, name: "The Lord of the Rings" },
    });
    const series = (await seriesResp.json()) as { id: number };

    await login(page);
    await page.goto(`/libraries/${testData.libraryId}/series/${series.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.locator("h1").filter({ hasText: "The Lord of the Rings" }),
    ).toBeVisible();

    // Open edit dialog and change the name.
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "Edit Series" }),
    ).toBeVisible();

    const nameInput = page.getByLabel("Name", { exact: true });
    await nameInput.clear();
    await nameInput.fill("LOTR");

    // Save.
    await Promise.all([
      page.waitForResponse(
        (r) =>
          r.url().includes(`/api/series/${series.id}`) &&
          r.request().method() === "PATCH" &&
          r.ok(),
      ),
      page.getByRole("button", { name: "Save" }).click(),
    ]);

    // Page should now show the new name.
    await expect(
      page.getByRole("heading", { name: "LOTR", exact: true }).first(),
    ).toBeVisible();

    // Reopen edit dialog — old name should be an alias.
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByRole("heading", { name: "Edit Series" }),
    ).toBeVisible();
    await expect(dialog.getByText("The Lord of the Rings")).toBeVisible();

    // Verify via API.
    const resp = await page.request.get(`/api/series/${series.id}`);
    const data = (await resp.json()) as { aliases: string[] };
    expect(data.aliases).toContain("The Lord of the Rings");
  });

  test("searching by alias on the series list page finds the canonical resource", async ({
    page,
    apiContext,
  }) => {
    // Create a series.
    const seriesResp = await apiContext.post("/test/series", {
      data: {
        libraryId: testData.libraryId,
        name: "A Song of Ice and Fire",
      },
    });
    const series = (await seriesResp.json()) as { id: number };

    await login(page);

    // Add alias via authenticated API — this triggers FTS indexing.
    const patchResp = await page.request.patch(`/api/series/${series.id}`, {
      data: {
        name: "A Song of Ice and Fire",
        aliases: ["ASOIAF", "Game of Thrones Series"],
      },
    });
    expect(patchResp.ok()).toBeTruthy();

    // Navigate to the series list page and search by alias name.
    await page.goto(`/libraries/${testData.libraryId}/series?search=ASOIAF`, {
      waitUntil: "domcontentloaded",
    });

    // The canonical series should appear in the search results.
    await expect(page.getByText("A Song of Ice and Fire")).toBeVisible();
  });
});
