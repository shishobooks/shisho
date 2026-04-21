import { expect, getApiBaseURL, request, test } from "./fixtures";
import {
  clearPlugins,
  loginAsPluginAdmin,
  PLUGIN_TEST_PASSWORD,
  PLUGIN_TEST_USERNAME,
  seedPlugin,
} from "./plugin-helpers";

test.describe("Plugin lifecycle flows", () => {
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

  test("uninstall removes the plugin from the Installed list", async ({
    apiContext,
    page,
  }) => {
    await seedPlugin(apiContext, {
      scope: "test",
      id: "fixture",
      name: "Fixture Plugin",
    });

    await loginAsPluginAdmin(page);
    await page.goto("/settings/plugins/test/fixture");

    // Danger zone has a single Uninstall button; the confirm dialog exposes
    // a second Uninstall button.
    await page.getByRole("button", { name: "Uninstall", exact: true }).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Uninstall", exact: true })
      .click();

    // Redirected back to the Installed list; plugin no longer present.
    await expect(page).toHaveURL(/\/settings\/plugins$/);
    await expect(
      page.getByText(
        "No plugins installed yet. Browse available plugins to get started.",
      ),
    ).toBeVisible();
  });

  test("update applies the new version and clears the update pill", async ({
    apiContext,
    page,
  }) => {
    // Seed with an update pending. The /plugins/installed/:scope/:id/update
    // handler performs the real update flow, so it needs a valid download
    // URL + sha. We patch the plugin after the page renders by calling
    // the update endpoint ourselves with fixture info.
    await seedPlugin(apiContext, {
      scope: "test",
      id: "fixture",
      name: "Fixture Plugin",
      version: "0.9.0",
      update_available_version: "1.0.0",
    });

    await loginAsPluginAdmin(page);
    await page.goto("/settings/plugins");

    // Update pill is visible.
    await expect(page.getByRole("button", { name: "Update" })).toBeVisible();

    // The real updateVersion handler looks up the repository version. That
    // path requires a seeded repository; the direct-URL update path is not
    // exposed from the UI. Skip this test until repository seeding lands.
    test.skip(
      true,
      "Update-via-UI requires a seeded repository (follow-up task).",
    );
  });

  test("disabling a plugin marks it disabled and re-enabling restores active", async ({
    apiContext,
    page,
  }) => {
    await seedPlugin(apiContext, {
      scope: "test",
      id: "fixture",
      name: "Fixture Plugin",
    });

    await loginAsPluginAdmin(page);
    await page.goto("/settings/plugins/test/fixture");

    // The hero exposes Enable/Disable as a labeled Switch, not a button.
    const enabledSwitch = page.getByRole("switch", { name: "Enabled" });
    await expect(enabledSwitch).toBeChecked();

    // Disable.
    await enabledSwitch.click();
    await expect(enabledSwitch).not.toBeChecked();

    // Back to the list: the plugin row is shown with a Disabled badge.
    await page.goto("/settings/plugins");
    const row = page.getByRole("link", { name: /Fixture Plugin/ });
    await expect(row).toBeVisible();
    await expect(row.getByText("Disabled")).toBeVisible();

    // Re-enable from the detail page.
    await page.goto("/settings/plugins/test/fixture");
    const enabledSwitchAgain = page.getByRole("switch", { name: "Enabled" });
    await expect(enabledSwitchAgain).not.toBeChecked();
    await enabledSwitchAgain.click();
    await expect(enabledSwitchAgain).toBeChecked();
  });
});
