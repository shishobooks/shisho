/**
 * E2E test for Bookmarks in the web audiobook player: adding one with a
 * note, finding it again after a reload, and jumping back to it.
 *
 * The seeded audiobook has no file on disk, so its stream never loads. The
 * player still moves its place on a seek, and with no metadata it never
 * restores the saved position on its own, so after the reload only the jump
 * can bring it back to the Bookmark.
 *
 * Running:
 *   pnpm e2e:chromium e2e/audiobook-bookmarks.spec.ts
 */

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "bookmarks";
const PASSWORD = "password123";

let libraryId: number;
let bookId: number;
let fileId: number;

test.describe("Audiobook Bookmarks", () => {
  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: USERNAME, password: PASSWORD },
    });
    const library = (await (
      await apiContext.post("/api/test/libraries", {
        data: { name: "Bookmarks Test Library" },
      })
    ).json()) as { id: number };
    libraryId = library.id;
    const book = (await (
      await apiContext.post("/api/test/books", {
        data: {
          libraryId,
          title: "Marked Audiobook",
          fileType: "m4b",
          audiobookDurationSeconds: 600,
        },
      })
    ).json()) as { id: number; fileId: number };
    bookId = book.id;
    fileId = book.fileId;
    await apiContext.dispose();
  });

  test.afterAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/users");
    await apiContext.dispose();
  });

  test("adds a Bookmark with a note, keeps it through a reload, and jumps to it", async ({
    page,
  }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/admin\/libraries|\/libraries\//);

    const playerUrl = `/libraries/${libraryId}/books/${bookId}/files/${fileId}/read`;
    await page.goto(playerUrl, { waitUntil: "domcontentloaded" });
    const elapsed = page.getByText(/^\d+:\d\d$/).first();
    await expect(elapsed).toHaveText("0:00");

    // Seek to 1:23: PageUp moves the seek bar 10 seconds, the arrows 1.
    const seek = page.getByRole("slider", { name: "Seek" });
    await seek.focus();
    for (let i = 0; i < 8; i++) await seek.press("PageUp");
    for (let i = 0; i < 3; i++) await seek.press("ArrowRight");
    await expect(elapsed).toHaveText("1:23");

    await page
      .getByRole("button", { name: "Add Bookmark", exact: true })
      .click();
    const addDialog = page.getByRole("dialog", { name: "Add Bookmark" });
    await expect(addDialog).toContainText("Mark 1:23 to come back to.");
    await addDialog.getByLabel("Note").fill("test");
    const created = page.waitForResponse(
      (resp) =>
        resp.url().endsWith(`/api/reading/files/${fileId}/bookmarks`) &&
        resp.request().method() === "POST",
    );
    await addDialog.getByRole("button", { name: "Add Bookmark" }).click();
    expect((await created).status()).toBe(201);
    await expect(addDialog).toBeHidden();

    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(elapsed).toHaveText("0:00");

    await page.getByRole("button", { name: "Bookmarks", exact: true }).click();
    const panel = page.getByRole("dialog", { name: "Bookmarks" });
    const jump = panel.getByRole("button", { name: "Jump to 1:23 test" });
    await expect(jump).toBeVisible();
    await jump.click();

    await expect(panel).toBeHidden();
    await expect(elapsed).toHaveText("1:23");
  });
});
