import { expect, getApiBaseURL, request, test } from "./fixtures";
import {
  clearPlugins,
  loginApi,
  loginAsPluginAdmin,
  PLUGIN_TEST_PASSWORD,
  PLUGIN_TEST_USERNAME,
  seedPlugin,
} from "./plugin-helpers";

test.describe("Plugin config save", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const api = await request.newContext({ baseURL: apiBaseURL });
    await clearPlugins(api);
    await api.delete("/test/ereader");
    await api.delete("/test/users");
    await api.post("/test/users", {
      data: {
        username: PLUGIN_TEST_USERNAME,
        password: PLUGIN_TEST_PASSWORD,
      },
    });
    await api.dispose();
  });

  test.beforeEach(async ({ apiContext }) => {
    await clearPlugins(apiContext);
  });

  test("editing a config field saves and round-trips", async ({
    apiContext,
    page,
  }) => {
    await seedPlugin(apiContext, {
      scope: "test",
      id: "fixture",
      name: "Fixture Plugin",
    });

    await loginAsPluginAdmin(page);
    // The /plugins/* routes require authentication; log the API context in
    // so cookies attach to subsequent requests.
    await loginApi(apiContext);
    await page.goto("/settings/plugins/test/fixture");

    // The fixture has a single secret config field "apiKey".
    const input = page.getByLabel("API Key");
    await input.fill("test-secret-value");

    // Wait for the save PATCH to complete before querying — otherwise the
    // GET below can race the in-flight write and read a stale (empty)
    // plugin_configs row.
    await Promise.all([
      page.waitForResponse(
        (response) =>
          response.url().includes("/plugins/installed/test/fixture") &&
          response.request().method() === "PATCH" &&
          response.ok(),
      ),
      page.getByRole("button", { name: "Save" }).click(),
    ]);

    // Assert the save succeeded: the value round-trips via the API.
    const resp = await apiContext.get("/plugins/installed/test/fixture/config");
    expect(resp.ok()).toBeTruthy();
    const body: { values: Record<string, unknown> } = await resp.json();
    // Secret values are masked on read as "***" — asserting the masked
    // value proves both that the write landed AND that secret masking is
    // applied on read.
    expect(body.values.apiKey).toBe("***");
  });
});
