import { expect, test, type Locator } from "@playwright/test";

interface RGB {
  red: number;
  green: number;
  blue: number;
}

const parseRGB = (value: string): RGB[] =>
  [...value.matchAll(/rgba?\((\d+),\s*(\d+),\s*(\d+)/g)].map((match) => ({
    red: Number(match[1]),
    green: Number(match[2]),
    blue: Number(match[3]),
  }));

const relativeLuminance = ({ red, green, blue }: RGB): number => {
  const linearize = (channel: number): number => {
    const value = channel / 255;
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  };

  return (
    0.2126 * linearize(red) +
    0.7152 * linearize(green) +
    0.0722 * linearize(blue)
  );
};

const contrastRatio = (first: RGB, second: RGB): number => {
  const lighter = Math.max(relativeLuminance(first), relativeLuminance(second));
  const darker = Math.min(relativeLuminance(first), relativeLuminance(second));
  return (lighter + 0.05) / (darker + 0.05);
};

const computedColors = async (
  locator: Locator,
): Promise<{ background: string; color: string }> =>
  locator.evaluate((element) => {
    const styles = getComputedStyle(element);
    return {
      background: styles.backgroundImage,
      color: styles.color,
    };
  });

test("keeps Aurora controls readable", async ({ page }) => {
  await page.goto("/");

  const primaryButton = page.locator(".docs-home__btn--primary").first();
  const buttonColors = await computedColors(primaryButton);
  const foreground = parseRGB(buttonColors.color)[0];
  const gradientStops = parseRGB(buttonColors.background);

  expect(foreground).toBeDefined();
  expect(gradientStops).toHaveLength(2);
  for (const stop of gradientStops) {
    expect(contrastRatio(foreground, stop)).toBeGreaterThanOrEqual(4.5);
  }

  await primaryButton.hover();
  const hoverColors = await computedColors(primaryButton);
  for (const stop of parseRGB(hoverColors.background)) {
    expect(contrastRatio(foreground, stop)).toBeGreaterThanOrEqual(4.5);
  }

  await page.goto("/docs/getting-started");
  const badgeColors = await computedColors(
    page.locator(".theme-doc-version-badge"),
  );
  expect(parseRGB(badgeColors.color)[0]).toEqual({
    red: 214,
    green: 204,
    blue: 255,
  });
  expect(badgeColors.background).not.toBe("none");
});

test("keeps the mobile layout readable", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  const featureColumns = await page
    .locator(".docs-home__features")
    .evaluate((element) => getComputedStyle(element).gridTemplateColumns);
  expect(featureColumns.trim().split(/\s+/)).toHaveLength(1);

  await page.goto("/docs/getting-started");
  await page.locator(".navbar__toggle").click();

  const background = await page
    .locator(".navbar-sidebar")
    .evaluate((element) => getComputedStyle(element).backgroundColor);
  expect(background).toMatch(/^rgb\(/);
});

test("stays dark when a theme query attribute is supplied", async ({
  page,
}) => {
  await page.goto("/?docusaurus-data-theme=light");

  const colors = await page.locator("body").evaluate((element) => {
    const styles = getComputedStyle(element);
    return {
      background: styles.backgroundColor,
      color: styles.color,
    };
  });
  const background = parseRGB(colors.background)[0];
  const foreground = parseRGB(colors.color)[0];

  expect(relativeLuminance(background)).toBeLessThan(0.05);
  expect(contrastRatio(background, foreground)).toBeGreaterThanOrEqual(7);
});
