/**
 * E2E flow for KOReader's unrecognized books: the KOReader plugin sends
 * history for a copy Shisho never produced, the server holds it, and linking
 * the copy to its Book on the Device's page brings the history in at once.
 *
 * Running:
 *   pnpm e2e:chromium e2e/koreader-unrecognized.spec.ts
 */

import { randomBytes } from "node:crypto";

import { expect, getApiBaseURL, request, test } from "./fixtures";

const USERNAME = "koreaderunrecognized";
const PASSWORD = "password123";
const TITLE = "The Left Hand of Darkness";
const LIBRARY = "KOReader Unrecognized Library";

let libraryId: number;
let bookId: number;
let deviceId: string;

test.describe("KOReader unrecognized books", () => {
  test.beforeAll(async ({ browser }) => {
    const apiContext = await request.newContext({
      baseURL: getApiBaseURL(browser.browserType().name()),
    });
    await apiContext.delete("/api/test/ereader");
    await apiContext.delete("/api/test/users");
    const user = (await (
      await apiContext.post("/api/test/users", {
        data: { username: USERNAME, password: PASSWORD },
      })
    ).json()) as { id: number };
    const library = (await (
      await apiContext.post("/api/test/libraries", { data: { name: LIBRARY } })
    ).json()) as { id: number };
    libraryId = library.id;
    const book = (await (
      await apiContext.post("/api/test/books", {
        data: { libraryId, title: TITLE, fileType: "epub" },
      })
    ).json()) as { id: number };
    bookId = book.id;
    const device = (await (
      await apiContext.post("/api/test/devices", {
        data: { userId: user.id, name: "Kobo Libra", deviceType: "koreader" },
      })
    ).json()) as { id: string; credential: string };
    deviceId = device.id;

    // What the KOReader plugin sends for a copy downloaded before Shisho
    // recorded what it sent: reading history from KOReader's statistics and
    // the day KOReader marked it finished. No server recognizes it, so the
    // server holds it. It's history from before the Device, so it goes as
    // the history sent on first sign-in. Each run uses a fresh hash, since
    // the cleanup above leaves what other runs held.
    const document = randomBytes(16).toString("hex");
    const firstDay = new Date(Date.now() - 20 * 24 * 3600 * 1000);
    const start = Math.floor(firstDay.getTime() / 1000);
    const finishedDay = new Date(Date.now() - 10 * 24 * 3600 * 1000)
      .toISOString()
      .slice(0, 10);
    const upload = await apiContext.post("/koreader/plugin/upload", {
      headers: {
        Authorization: `Basic ${Buffer.from(`${USERNAME}:${device.credential}`).toString("base64")}`,
        "X-Shisho-Plugin-Version": "0.7.0",
        "X-Shisho-Plugin-Protocol": "1",
      },
      data: {
        clock: Math.floor(Date.now() / 1000),
        backfill: true,
        pages: Array.from({ length: 10 }, (_, i) => ({
          document,
          page: i + 1,
          total_pages: 100,
          start_time: start + i * 60,
          duration: 60,
        })),
        events: [
          {
            id: "finished",
            type: "status",
            document,
            day: finishedDay,
            status: "complete",
          },
        ],
        unrecognized: [
          {
            document,
            title: TITLE,
            authors: ["Ursula K. Le Guin"],
            file_type: "epub",
            reading_seconds: 600,
          },
        ],
      },
    });
    expect(upload.ok(), await upload.text()).toBe(true);
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

  test("linking an unrecognized book brings its history to the Book", async ({
    page,
  }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Username").fill(USERNAME);
    await page.getByLabel("Password").fill(PASSWORD);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL(/\/admin\/libraries|\/libraries\//);

    await page.goto(`/user/devices/${deviceId}`, {
      waitUntil: "domcontentloaded",
    });
    const row = page.getByRole("listitem", { name: TITLE, exact: true });
    await expect(row).toContainText("10m read in KOReader");

    // The seeded Book is the search suggestion, named so it can be told
    // apart from another edition.
    await row
      .getByRole("button", {
        name: `Link to ${TITLE}, EPUB in ${LIBRARY}`,
        exact: true,
      })
      .click();
    await expect(
      row.getByText(/syncs your place as a percentage only/),
    ).toBeVisible();

    // The history arrived without a sync: the Book shows the Read-through
    // KOReader finished.
    await row.getByRole("link", { name: TITLE, exact: true }).click();
    await page.waitForURL(`/libraries/${libraryId}/books/${bookId}`);
    const history = page.getByRole("region", { name: "Reading history" });
    await expect(history.getByText("Finished", { exact: true })).toBeVisible();
    await expect(history).toContainText("1 session");
  });
});
