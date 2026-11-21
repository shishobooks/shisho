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
