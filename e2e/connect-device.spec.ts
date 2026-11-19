/**
 * E2E journey for Connect a device on the eReader browser path: the user
 * picks another e-reader's browser, opens the setup URL from a second
 * browser standing in for the e-reader, and the waiting screen switches to
 * "Connected!" over the live event stream, with no reload.
 *
 * Running:
 *   pnpm e2e:chromium e2e/connect-device.spec.ts
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "connectdevice";
const PASSWORD = "password123";

test.describe("Connect a device", () => {
  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
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

  test("an e-reader opening the setup URL switches the waiting screen to Connected!", async ({
    browser,
    page,
  }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => url.pathname !== "/login");

    // Getting Started's "Connect a reading device" links here.
    await page.goto("/user/devices?connect", {
      waitUntil: "domcontentloaded",
    });
    const flow = page.getByRole("dialog");
    await expect(flow.getByText("What do you read on?")).toBeVisible();
    await flow
      .getByRole("radio", { name: /Another e-reader's browser/ })
      .click();
    await expect(flow.getByLabel("Name")).toHaveValue("E-reader");
    await flow.getByRole("button", { name: "Continue" }).click();

    await flow
      .getByRole("button", { name: "Get Setup URL", exact: true })
      .click();
    const setupURL = (await flow
      .getByLabel("Setup URL", { exact: true })
      .textContent())!;
    expect(setupURL).toMatch(/\/e\/\w+$/);
    await flow.getByRole("button", { name: "Next" }).click();
    await expect(
      flow.getByRole("heading", { name: "Waiting for your device" }),
    ).toBeVisible();

    // The e-reader: a browser with no session, opening the short URL.
    const ereader = await browser.newContext();
    const ereaderPage = await ereader.newPage();
    await ereaderPage.goto(setupURL, { waitUntil: "domcontentloaded" });
    await expect(ereaderPage).toHaveURL(/\/ereader\/key\//);
    await ereader.close();

    await expect(
      flow.getByRole("heading", { name: "Connected!" }),
    ).toBeVisible();
    await expect(flow.getByRole("status")).toContainText(
      "E-reader is connected to Shisho.",
    );

    await flow.getByRole("link", { name: "Open Device" }).click();
    await expect(page.getByTestId("device-status")).toContainText(
      "Connected. Last used",
    );
  });
});
