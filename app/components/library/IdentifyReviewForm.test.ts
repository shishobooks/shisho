import { resolveIdentifiers } from "./identify-utils";
import { describe, expect, it } from "vitest";

describe("resolveIdentifiers", () => {
  it("returns unchanged when identifiers are identical", () => {
    const current = [
      { type: "goodreads", value: "56377548" },
      { type: "uuid", value: "abc-123" },
    ];
    const incoming = [
      { type: "goodreads", value: "56377548" },
      { type: "uuid", value: "abc-123" },
    ];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("unchanged");
    expect(result.value).toEqual(current);
  });

  it("returns unchanged when current is a superset of incoming", () => {
    const current = [
      { type: "goodreads", value: "56377548" },
      { type: "uuid", value: "abc-123" },
    ];
    const incoming = [{ type: "goodreads", value: "56377548" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("unchanged");
    expect(result.value).toEqual(current);
  });

  it("returns changed when incoming has a new identifier", () => {
    const current = [{ type: "uuid", value: "abc-123" }];
    const incoming = [
      { type: "uuid", value: "abc-123" },
      { type: "goodreads", value: "56377548" },
    ];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([
      { type: "uuid", value: "abc-123" },
      { type: "goodreads", value: "56377548" },
    ]);
  });

  it("returns new when current is empty", () => {
    const incoming = [{ type: "goodreads", value: "56377548" }];
    const result = resolveIdentifiers([], incoming);
    expect(result.status).toBe("new");
    expect(result.value).toEqual(incoming);
  });

  it("returns unchanged when incoming is empty", () => {
    const current = [{ type: "uuid", value: "abc-123" }];
    const result = resolveIdentifiers(current, []);
    expect(result.status).toBe("unchanged");
    expect(result.value).toEqual(current);
  });
});
