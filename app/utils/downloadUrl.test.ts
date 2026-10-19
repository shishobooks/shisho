import { describe, expect, it } from "vitest";

import {
  bulkDownloadUrl,
  fileDownloadAsUrl,
  fileDownloadUrl,
  fileOriginalDownloadUrl,
  fileStreamUrl,
  shareFileDownloadAsUrl,
  shareFileDownloadUrl,
} from "./downloadUrl";

describe("download URLs", () => {
  it("builds the generated download URL", () => {
    expect(fileDownloadUrl(42)).toBe("/api/books/files/42/download");
  });

  it("builds the download URL for a File Type the file can become", () => {
    expect(fileDownloadAsUrl(42, "kepub")).toBe(
      "/api/books/files/42/download/kepub",
    );
    expect(fileDownloadAsUrl(42, "epub")).toBe(
      "/api/books/files/42/download/epub",
    );
  });

  it("builds the original-file download URL", () => {
    expect(fileOriginalDownloadUrl(42)).toBe(
      "/api/books/files/42/download/original",
    );
  });

  it("builds the audio stream URL", () => {
    expect(fileStreamUrl(42)).toBe("/api/books/files/42/stream");
  });

  it("builds the bulk download URL for a job", () => {
    expect(bulkDownloadUrl(9)).toBe("/api/jobs/9/download");
  });

  it("scopes a Share Link download to the encoded token", () => {
    expect(shareFileDownloadUrl("a/b", 42)).toBe(
      "/api/share/a%2Fb/files/42/download",
    );
  });

  it("scopes a Share Link download as a File Type to the encoded token", () => {
    expect(shareFileDownloadAsUrl("a/b", 42, "epub")).toBe(
      "/api/share/a%2Fb/files/42/download/epub",
    );
  });
});
