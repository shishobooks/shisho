import { describe, expect, it } from "vitest";

import { resolveIdentifiers } from "./identify-utils";

describe("resolveIdentifiers (incoming wins on type conflict)", () => {
  it("replaces an existing identifier when incoming has the same type with a different value", () => {
    const current = [{ type: "asin", value: "B01ABC1234" }];
    const incoming = [{ type: "asin", value: "B02DEF5678" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([{ type: "asin", value: "B02DEF5678" }]);
  });
});
