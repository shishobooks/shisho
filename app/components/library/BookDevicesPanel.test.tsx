import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { Book, BookDevice, File } from "@/types";

import BookDevicesPanel from "./BookDevicesPanel";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const file = (
  id: number,
  fileType: string,
  displayName: string,
  filepath: string,
): File =>
  ({
    id,
    file_type: fileType,
    file_role: "main",
    display_name: displayName,
    filepath,
  }) as File;

const epub = file(1, "epub", "Novel (2001)", "/lib/Novel/novel-2001.epub");
const epub2 = file(2, "epub", "Novel (Annotated)", "/lib/Novel/annotated.epub");
const pdf = file(3, "pdf", "Novel scan", "/lib/Novel/scan.pdf");

const line = (overrides: Partial<BookDevice>): BookDevice => ({
  device_id: "d",
  device_name: "Device",
  device_type: "kobo",
  deliveries: [],
  edition_moves: [],
  ...overrides,
});

const renderPanel = (
  devices: BookDevice[],
  files: File[] = [epub, epub2, pdf],
) => {
  vi.spyOn(API, "request").mockResolvedValue(devices);
  const book = { id: 9, title: "Novel", files } as Book;
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <BookDevicesPanel book={book} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const row = async (name: string) =>
  within(await screen.findByRole("listitem", { name }));

describe("BookDevicesPanel", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));
  afterEach(() => vi.restoreAllMocks());

  it("lists each Device with what it gets, and why it gets nothing", async () => {
    renderPanel([
      line({
        device_id: "kobo",
        device_name: "Kobo",
        deliveries: [
          {
            file_id: 1,
            source_file_type: "epub",
            delivered_file_type: "kepub",
          },
        ],
      }),
      line({
        device_id: "panels",
        device_name: "Panels",
        device_type: "opds",
        not_delivered_reason: "no_rule",
      }),
      line({
        device_id: "tablet",
        device_name: "Tablet",
        device_type: "browser",
        not_delivered_reason: "outside_scope",
      }),
    ]);

    expect(
      await screen.findByRole("heading", { name: "On your Devices" }),
    ).toBeInTheDocument();
    const kobo = await row("Kobo");
    expect(
      kobo.getByRole("img", { name: "EPUB as KePub" }),
    ).toBeInTheDocument();
    expect(kobo.getByRole("link", { name: "Kobo" })).toHaveAttribute(
      "href",
      "/user/devices/kobo",
    );
    expect((await row("Panels")).getByText(/Not delivered/)).toHaveTextContent(
      "Not delivered · No rule for EPUB or PDF",
    );
    expect((await row("Tablet")).getByText(/Not delivered/)).toHaveTextContent(
      "Not delivered · Outside its scope",
    );
    expect(screen.getByRole("link", { name: "Manage" })).toHaveAttribute(
      "href",
      "/user/devices",
    );
  });

  it("names each File when tied Files both go", async () => {
    renderPanel([
      line({
        device_name: "KOReader",
        device_type: "opds",
        deliveries: [
          { file_id: 1, source_file_type: "epub", delivered_file_type: "epub" },
          { file_id: 2, source_file_type: "epub", delivered_file_type: "epub" },
        ],
      }),
    ]);

    const koreader = await row("KOReader");
    expect(koreader.getAllByRole("img", { name: "EPUB as EPUB" })).toHaveLength(
      2,
    );
    expect(koreader.getByText("Novel (2001)")).toBeInTheDocument();
    expect(koreader.getByText("Novel (Annotated)")).toBeInTheDocument();
  });

  it("tells editions with the same name apart by their file names", async () => {
    const hardcover = file(1, "epub", "Novel", "/lib/Novel/hardcover.epub");
    const paperback = file(2, "epub", "Novel", "/lib/Novel/paperback.epub");
    renderPanel(
      [
        line({
          device_name: "Kobo",
          deliveries: [
            {
              file_id: 2,
              source_file_type: "epub",
              delivered_file_type: "kepub",
            },
          ],
          edition_moves: [{ from_file_id: 1, to_file_id: 2 }],
        }),
      ],
      [hardcover, paperback],
    );

    expect(
      await screen.findByText(
        "Its next sync moves this book to a different edition, Novel (paperback.epub), which may reopen at the start.",
      ),
    ).toBeInTheDocument();
  });

  it("warns when a Kobo's next sync moves the book to another edition", async () => {
    renderPanel([
      line({
        device_name: "Kobo",
        deliveries: [
          {
            file_id: 2,
            source_file_type: "epub",
            delivered_file_type: "kepub",
          },
        ],
        edition_moves: [{ from_file_id: 1, to_file_id: 2 }],
      }),
    ]);

    expect(
      await screen.findByText(
        "Its next sync moves this book to a different edition, Novel (Annotated), which may reopen at the start.",
      ),
    ).toBeInTheDocument();
  });

  it("warns when a Kobo's next sync switches the book between KePub and EPUB", async () => {
    renderPanel([
      line({
        device_name: "Kobo",
        deliveries: [
          { file_id: 1, source_file_type: "epub", delivered_file_type: "epub" },
        ],
        type_switch: { from_file_type: "kepub", to_file_type: "epub" },
      }),
    ]);

    expect(
      await (
        await row("Kobo")
      ).findByText(/Its next sync switches this book from KePub to EPUB/),
    ).toHaveTextContent(
      "The Kobo adds it again as a new book, without its reading position or highlights.",
    );
  });

  it("lists the Devices that get the book first", async () => {
    renderPanel([
      line({
        device_id: "panels",
        device_name: "Panels",
        not_delivered_reason: "no_rule",
      }),
      line({
        device_id: "kobo",
        device_name: "Kobo",
        deliveries: [
          {
            file_id: 1,
            source_file_type: "epub",
            delivered_file_type: "kepub",
          },
        ],
      }),
    ]);

    await screen.findByRole("listitem", { name: "Kobo" });
    expect(
      screen
        .getAllByRole("listitem")
        .map((li) => li.getAttribute("aria-label")),
    ).toEqual(["Kobo", "Panels"]);
  });

  it("shows nothing without Devices", async () => {
    const { container } = renderPanel([]);

    await vi.waitFor(() => expect(API.request).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });
});
