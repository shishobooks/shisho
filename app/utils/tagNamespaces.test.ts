import { describe, expect, it } from "vitest";

import { splitTagNamespace } from "./tagNamespaces";

const namespaces = ["Circle", "Magazine"];

describe("splitTagNamespace", () => {
  it("splits a tag with or without a space after the colon", () => {
    expect(splitTagNamespace("Circle: Moon Factory", namespaces)).toEqual({
      namespace: "Circle",
      name: "Moon Factory",
    });
    expect(splitTagNamespace("Circle:Apple Lab", namespaces)).toEqual({
      namespace: "Circle",
      name: "Apple Lab",
    });
  });

  it("matches the namespace case-insensitively and reports it as configured", () => {
    expect(splitTagNamespace("circle: Neko", namespaces)).toEqual({
      namespace: "Circle",
      name: "Neko",
    });
  });

  it("leaves other tags plain", () => {
    for (const name of [
      "Full Color",
      "Circle:",
      "Circle:   ",
      "Circles: Plural",
      "Star Wars: The Clone Wars",
    ]) {
      expect(splitTagNamespace(name, namespaces)).toEqual({
        namespace: null,
        name,
      });
    }
  });

  it("folds only ASCII letters, as the server does", () => {
    expect(splitTagNamespace("ÉCOLE: Foo", ["école"]).namespace).toBeNull();
    expect(splitTagNamespace("école: Foo", ["école"]).namespace).toBe("école");
    expect(splitTagNamespace("ÉCOLE: Foo", ["École"]).namespace).toBe("École");
  });

  it("leaves every tag plain when the library has no namespaces", () => {
    expect(splitTagNamespace("Circle: Foo", [])).toEqual({
      namespace: null,
      name: "Circle: Foo",
    });
  });
});
