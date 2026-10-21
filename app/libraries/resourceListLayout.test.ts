import { describe, expect, it } from "vitest";

import {
  resourceListColumns,
  resourceListPageSize,
} from "./resourceListLayout";

describe("resourceListColumns", () => {
  it("fits as many entries side by side as the width holds", () => {
    expect(resourceListColumns(1660, false)).toBe(4);
    expect(resourceListColumns(1000, false)).toBe(2);
    expect(resourceListColumns(300, false)).toBe(1);
  });

  it("shows one column on a phone", () => {
    expect(resourceListColumns(1660, true)).toBe(1);
  });
});

describe("resourceListPageSize", () => {
  it("fills whole rows close to fifty entries", () => {
    expect(resourceListPageSize(1)).toBe(50);
    expect(resourceListPageSize(4)).toBe(48);
    expect(resourceListPageSize(3)).toBe(48);
  });
});
