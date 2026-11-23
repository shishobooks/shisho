import { describe, expect, it } from "vitest";

import { Paginator } from "@/libraries/foliate/paginator.js";

// useLocationHistory calls settled() only where it exists, since the
// fixed-layout renderer has none, so losing the patch would fail nothing
// else: history moves made during a page turn would just be dropped again.
describe("the paginator's settled() patch", () => {
  it("is there", () => {
    expect(typeof Paginator.prototype.settled).toBe("function");
  });
});

// The iOS reader (mobile/ios/Shisho/Reader/ReaderWeb/reader.js) counts the
// pages to the next chapter with pageOf() where it exists, and falls back to
// an estimate where it doesn't, so losing the patch would fail nothing else.
describe("the paginator's pageOf() patch", () => {
  it("is there", () => {
    expect(typeof Paginator.prototype.pageOf).toBe("function");
  });
});
