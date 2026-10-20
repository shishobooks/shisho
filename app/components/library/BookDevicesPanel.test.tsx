import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { Book, BookDevice, File } from "@/types";

import BookDevicesPanel from "./BookDevicesPanel";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const file = (id: number, fileType: string, displayName: string): File =>
  ({
    id,
    file_type: fileType,
    file_role: "main",
    display_name: displayName,
  }) as File;

const epub = file(1, "epub", "Novel (2001)");
const epub2 = file(2, "epub", "Novel (Annotated)");
const pdf = file(3, "pdf", "Novel scan");
const book = { id: 9, title: "Novel", files: [epub, epub2, pdf] } as Book;

const line = (overrides: Partial<BookDevice>): BookDevice => ({
  device_id: "d",
  device_name: "Device",
  device_type: "kobo",
  deliveries: [],
  edition_moves: [],
  ...overrides,
});

const renderPanel = (devices: BookDevice[]) => {
  vi.spyOn(API, "request").mockResolvedValue(devices);
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <BookDevicesPanel book={book} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

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
    const items = screen.getAllByRole("listitem").map((li) => li.textContent);
    expect(items).toEqual([
      "Kobo: as KePub (from EPUB)",
      "Panels: not delivered, no rule for EPUB or PDF",
      "Tablet: not delivered, outside its scope",
    ]);
    expect(screen.getByRole("link", { name: "Kobo" })).toHaveAttribute(
      "href",
      "/user/devices/kobo",
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

    expect(await screen.findByRole("listitem")).toHaveTextContent(
      "KOReader: as EPUB (from EPUB): Novel (2001)KOReader: as EPUB (from EPUB): Novel (Annotated)",
    );
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
        "Its next sync moves this book to a different edition (Novel (Annotated)), which may reopen at the start.",
      ),
    ).toBeInTheDocument();
  });

  it("shows nothing without Devices", async () => {
    const { container } = renderPanel([]);

    await vi.waitFor(() => expect(API.request).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });
});
