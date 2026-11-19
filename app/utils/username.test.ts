import { describe, expect, it } from "vitest";

import { usernameProblem } from "./username";

describe("usernameProblem", () => {
  it.each([
    "robin",
    "Robin_Joseph-08",
    "robin.j@mail.com",
    "José",
    "Jose\u0301",
    "司書人",
    "राम",
    "สมชาย",
  ])("accepts %s", (username) => expect(usernameProblem(username)).toBeNull());

  it.each(["rob:in", "rob in", "rob\tin", "rob\u200bin", "rob/in", "<robin>"])(
    "rejects %j for its characters",
    (username) =>
      expect(usernameProblem(username)).toBe(
        "A username can only contain letters, numbers, and . _ - @",
      ),
  );

  it("asks for a username when there is none", () => {
    expect(usernameProblem("")).toBe("Username is required");
  });

  it("counts a letter and its combining accent as one character, as the server does", () => {
    // "Joé" with the accent typed separately is 3 characters once composed.
    expect(usernameProblem("Joe\u0301")).toBeNull();
    expect(usernameProblem("e\u0301e")).toBe(
      "A username needs at least 3 characters",
    );
  });

  it("counts characters, not bytes, for the length", () => {
    expect(usernameProblem("ab")).toBe(
      "A username needs at least 3 characters",
    );
    expect(usernameProblem("司書人")).toBeNull();
    expect(usernameProblem("a".repeat(51))).toBe(
      "A username can't be longer than 50 characters",
    );
  });
});
