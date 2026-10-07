import { expect, getApiBaseURL, request, test } from "./fixtures";

const username = "cacheadmin";
const password = "password123";

test.describe("Cover thumbnail cache settings", () => {
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
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(username);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await page.waitForURL(/\/settings\/libraries|\/libraries\//);
    await page.goto("/settings/cache", { waitUntil: "domcontentloaded" });
    const limit = page.getByRole("spinbutton", { name: "Maximum size (GiB)" });
    await expect(limit).toBeVisible();
    await limit.fill("2.5");
    await page.getByRole("button", { name: "Save limit", exact: true }).click();
    await expect(
      page.getByText("Cover thumbnail cache limit saved", { exact: true }),
    ).toBeVisible();
    await expect(limit).toHaveValue("2.5");
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(limit).toHaveValue("2.5");
    await expect(
      page.getByRole("button", { name: "Save limit", exact: true }),
    ).toBeDisabled();
  });

  test.afterAll(async ({ browser }) => {
    const api = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await api.post("/api/auth/login", { data: { username, password } });
    await api.put("/api/settings/cache", {
      data: { cover_thumbnail_max_size_gb: 1 },
    });
    await api.delete("/api/test/users");
    await api.dispose();
  });
});
