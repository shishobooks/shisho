import { describe, expect, it } from "vitest";

import { getReadingAction } from "./readingAction";

describe("getReadingAction", () => {
  it("returns a Listen action for m4b files", () => {
    expect(getReadingAction("m4b")).toBe("listen");
  });

  it("returns a Read action for cbz, epub, and pdf files", () => {
    expect(getReadingAction("cbz")).toBe("read");
    expect(getReadingAction("epub")).toBe("read");
    expect(getReadingAction("pdf")).toBe("read");
  });

  it("returns null for file types with no in-app reader", () => {
    expect(getReadingAction("txt")).toBeNull();
    expect(getReadingAction("")).toBeNull();
  });
});
