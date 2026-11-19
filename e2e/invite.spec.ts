/**
 * E2E journey for Invite Links: an admin creates one from Add User, and a
 * browser with no session accepts it and lands in the app with Getting
 * Started open.
 *
 * Running:
 *   pnpm e2e:chromium e2e/invite.spec.ts
 */

import type { Page } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "inviteadmin";
const PASSWORD = "password123";
const LIBRARY = "Invite Library";

test.describe("Invite Links", () => {
  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });
    await apiContext.post("/api/test/libraries", { data: { name: LIBRARY } });
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
    await page.waitForURL(/\/admin\/libraries|\/libraries\//);
  }

  test("an admin's invite lets a new person in with Getting Started open", async ({
    browser,
    page,
  }) => {
    await login(page);

    await page.goto("/admin/users/create", { waitUntil: "domcontentloaded" });
    await expect(
      page.getByRole("radio", { name: "Invite link" }),
    ).toBeChecked();
    await page.getByRole("radio", { name: /viewer/ }).click();
    await page.getByRole("button", { name: "Create invite link" }).click();

    const link = page.locator('code[aria-label="Invite link"]');
    await expect(link).toBeVisible();
    const url = (await link.textContent())!.trim();
    const origin = new URL(page.url()).origin;
    expect(url).toMatch(new RegExp(`^${origin}/invite/[A-Za-z0-9_-]{43}$`));

    // The Users page lists it as pending.
    await page.getByRole("link", { name: "Done" }).click();
    await expect(
      page
        .getByRole("list", { name: "Users" })
        .getByRole("listitem")
        .filter({ hasText: "Pending" })
        .getByText("viewer"),
    ).toBeVisible();

    // A fresh context has no session cookie.
    const anonymous = await browser.newContext();
    try {
      const invitee = await anonymous.newPage();
      await invitee.goto(url, { waitUntil: "domcontentloaded" });
      await expect(
        invitee.getByRole("heading", {
          name: `${USERNAME} invited you to their Shisho library`,
        }),
      ).toBeVisible();

      await invitee.getByLabel("Username").fill("invitee");
      await invitee.getByLabel("Password", { exact: true }).fill(PASSWORD);
      await invitee.getByLabel("Confirm Password").fill(PASSWORD);
      await invitee.getByRole("button", { name: "Create account" }).click();

      await invitee.waitForURL(/\/libraries\/\d+/);
      await expect(
        invitee.getByRole("dialog", { name: "Getting Started" }),
      ).toBeVisible();

      // The link worked once.
      await invitee.context().clearCookies();
      await invitee.goto(url, { waitUntil: "domcontentloaded" });
      await expect(
        invitee.getByRole("heading", {
          name: "This invite link is no longer valid",
        }),
      ).toBeVisible();
    } finally {
      await anonymous.close();
    }
  });
});
