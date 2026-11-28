/**
 * E2E regression test for the Advanced plugin settings dialog on a phone.
 *
 * Layout overflow needs a real browser: jsdom computes no widths, so a unit
 * test can't tell whether a row fits or a select's text is cut off.
 *
 * Running:
 *   pnpm e2e:chromium e2e/plugin-order-overflow.spec.ts
 */

import type { Locator } from "@playwright/test";

import { expect, getApiBaseURL, request, test } from "./fixtures";
import { clearPlugins, resetAndLogin, seedPlugin } from "./plugin-helpers";

// What sticks out of the tab's scroll area, or is cut off inside a select.
// Empty when everything fits.
const overflowIn = (tabPanel: Locator) =>
  tabPanel.evaluate((panel) => {
    let scroller = panel.parentElement;
    while (scroller && getComputedStyle(scroller).overflowY !== "auto") {
      scroller = scroller.parentElement;
    }
    if (!scroller) return ["no scroll area around the tab"];
    const problems: string[] = [];
    if (scroller.scrollWidth > scroller.clientWidth + 1) {
      problems.push(
        `scroll area is ${scroller.scrollWidth}px wide in ${scroller.clientWidth}px`,
      );
    }
    const bounds = scroller.getBoundingClientRect();
    for (const el of panel.querySelectorAll<HTMLElement>(
      "button, input, [role=combobox], span, p, h3",
    )) {
      const r = el.getBoundingClientRect();
      if (r.width > 0 && r.right > bounds.right + 1) {
        problems.push(`"${el.textContent?.trim()}" sticks out`);
      }
    }
    for (const trigger of panel.querySelectorAll("[role=combobox]")) {
      for (const span of trigger.querySelectorAll<HTMLElement>(
        ":scope > span",
      )) {
        if (
          span.scrollWidth > span.clientWidth + 1 ||
          span.scrollHeight > span.clientHeight + 1
        ) {
          problems.push(`"${span.textContent?.trim()}" is cut off`);
        }
      }
    }
    return problems;
  });

// Controls in each order row whose position on screen runs against the
// keyboard's Tab order: a later control drawn above, or left of, an earlier
// one on the same line. Empty when the two orders agree.
const tabOrderMismatches = (tabPanel: Locator) =>
  tabPanel.evaluate((panel) => {
    const problems: string[] = [];
    for (const row of panel.querySelectorAll(":scope .space-y-2 > div")) {
      const controls = [
        ...row.querySelectorAll<HTMLElement>("button, [role=combobox]"),
      ].map((el) => ({
        box: el.getBoundingClientRect(),
        name: el.getAttribute("aria-label") ?? el.textContent ?? "",
      }));
      for (let i = 1; i < controls.length; i++) {
        const a = controls[i - 1];
        const b = controls[i];
        const laterLine = b.box.top >= a.box.bottom - 1;
        const sameLine = Math.abs(b.box.top - a.box.top) < a.box.height;
        if (!laterLine && !(sameLine && b.box.left >= a.box.right - 1)) {
          problems.push(`"${b.name}" is drawn before "${a.name}"`);
        }
      }
    }
    return problems;
  });

test.describe("Advanced plugin settings on a phone", () => {
  test.beforeEach(async ({ browser, page }) => {
    const api = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await resetAndLogin(api, page);
    // Only the fixture id loads, so two scopes give two rows in the order.
    await seedPlugin(api, { scope: "shisho", id: "fixture" });
    await seedPlugin(api, {
      scope: "community-plugins-with-a-long-scope",
      id: "fixture",
    });
    await api.dispose();
  });

  test.afterAll(async ({ browser }) => {
    const api = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await clearPlugins(api);
    await api.dispose();
  });

  test("fits both tabs at 320px with the longest mode selected", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 640 });
    await page.goto("/admin/plugins?advanced=order", {
      waitUntil: "domcontentloaded",
    });

    const dialog = page.getByRole("dialog", {
      name: "Advanced plugin settings",
    });
    const modes = dialog.getByRole("combobox", {
      name: "When this plugin runs",
    });
    await expect(modes).toHaveCount(2);
    await modes.first().click();
    await page
      .getByRole("option", { name: /^Only when manually identifying/ })
      .click();
    await expect(modes.first()).toHaveText(/Only when manually identifying/);

    expect(await overflowIn(dialog.getByRole("tabpanel"))).toEqual([]);
    expect(await tabOrderMismatches(dialog.getByRole("tabpanel"))).toEqual([]);

    await dialog.getByRole("tab", { name: "Repositories" }).click();
    await expect(dialog.getByLabel("Repository URL")).toBeVisible();
    expect(await overflowIn(dialog.getByRole("tabpanel"))).toEqual([]);
  });
});
