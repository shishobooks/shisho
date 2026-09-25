import { expect, test } from "@playwright/test";

test("routes evaluators from the homepage to the Public Demo page", async ({
  page,
}) => {
  await page.goto("/");

  const hero = page.locator(".docs-home__hero");
  const demoButton = hero.getByRole("link", { name: "Try the Demo" });
  await expect(demoButton).toHaveAttribute("href", "/docs/demo");
  await expect(hero.getByRole("link", { name: "Get Started" })).toHaveAttribute(
    "href",
    "/docs/getting-started",
  );

  await demoButton.click();
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "Public Demo",
  );
});

test("publishes the Public Demo page with its credential", async ({ page }) => {
  await page.goto("/docs/unreleased/demo");

  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "Public Demo",
  );
  await expect(
    page.getByRole("link", { name: "demo.shishobooks.com" }).first(),
  ).toHaveAttribute("href", "https://demo.shishobooks.com");
  await expect(page.locator("article")).toContainText("shishodemo");
  await expect(
    page
      .locator("article")
      .getByRole("link", { name: "Getting Started" })
      .first(),
  ).toHaveAttribute("href", /\/docs\/(unreleased\/)?getting-started$/);
});
