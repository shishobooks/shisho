import {
  validateASIN,
  validateIdentifier,
  validateISBN10,
  validateISBN13,
  validateUUID,
} from "./identifiers";
import { describe, expect, it } from "vitest";

describe("validateISBN10", () => {
  it("validates correct ISBN-10", () => {
    // "The Great Gatsby" - valid ISBN-10
    expect(validateISBN10("0743273567")).toBe(true);
  });

  it("validates ISBN-10 with dashes", () => {
    expect(validateISBN10("0-7432-7356-7")).toBe(true);
  });

  it("validates ISBN-10 with spaces", () => {
    expect(validateISBN10("0 7432 7356 7")).toBe(true);
  });

  it("validates ISBN-10 with X check digit", () => {
    // "The C Programming Language" - ends with X
    expect(validateISBN10("080442957X")).toBe(true);
  });

  it("validates lowercase x check digit", () => {
    expect(validateISBN10("080442957x")).toBe(true);
  });

  it("rejects invalid checksum", () => {
    expect(validateISBN10("0743273568")).toBe(false);
  });

  it("rejects wrong length", () => {
    expect(validateISBN10("074327356")).toBe(false);
    expect(validateISBN10("07432735677")).toBe(false);
  });

  it("rejects X in non-final position", () => {
    expect(validateISBN10("0X43273567")).toBe(false);
  });

  it("rejects non-numeric characters", () => {
    expect(validateISBN10("074327356A")).toBe(false);
  });
});

describe("validateISBN13", () => {
  it("validates correct ISBN-13", () => {
    // "The Great Gatsby" - valid ISBN-13
    expect(validateISBN13("9780743273565")).toBe(true);
  });

  it("validates ISBN-13 with dashes", () => {
    expect(validateISBN13("978-0-7432-7356-5")).toBe(true);
  });

  it("validates ISBN-13 with spaces", () => {
    expect(validateISBN13("978 0 7432 7356 5")).toBe(true);
  });

  it("rejects invalid checksum", () => {
    expect(validateISBN13("9780743273566")).toBe(false);
  });

  it("rejects wrong length", () => {
    expect(validateISBN13("978074327356")).toBe(false);
    expect(validateISBN13("97807432735655")).toBe(false);
  });

  it("rejects non-numeric characters", () => {
    expect(validateISBN13("978074327356X")).toBe(false);
  });
});

describe("validateASIN", () => {
  it("validates correct ASIN", () => {
    expect(validateASIN("B0CHVFQ31G")).toBe(true);
  });

  it("validates lowercase asin", () => {
    expect(validateASIN("b0chvfq31g")).toBe(true);
  });

  it("rejects ASIN not starting with B0", () => {
    expect(validateASIN("A0CHVFQ31G")).toBe(false);
    expect(validateASIN("B1CHVFQ31G")).toBe(false);
  });

  it("rejects wrong length", () => {
    expect(validateASIN("B0CHVFQ31")).toBe(false);
    expect(validateASIN("B0CHVFQ31GX")).toBe(false);
  });

  it("rejects invalid characters", () => {
    expect(validateASIN("B0CHVFQ31!")).toBe(false);
  });
});

describe("validateUUID", () => {
  it("validates correct UUID", () => {
    expect(validateUUID("550e8400-e29b-41d4-a716-446655440000")).toBe(true);
  });

  it("validates uppercase UUID", () => {
    expect(validateUUID("550E8400-E29B-41D4-A716-446655440000")).toBe(true);
  });

  it("validates UUID with urn:uuid: prefix", () => {
    expect(validateUUID("urn:uuid:550e8400-e29b-41d4-a716-446655440000")).toBe(
      true,
    );
  });

  it("rejects UUID without dashes", () => {
    expect(validateUUID("550e8400e29b41d4a716446655440000")).toBe(false);
  });

  it("rejects UUID with wrong segment lengths", () => {
    expect(validateUUID("550e840-e29b-41d4-a716-446655440000")).toBe(false);
  });

  it("rejects invalid characters", () => {
    expect(validateUUID("550e8400-e29b-41d4-a716-44665544000g")).toBe(false);
  });
});

describe("validateIdentifier", () => {
  it("validates isbn_10 type", () => {
    expect(validateIdentifier("isbn_10", "0743273567")).toEqual({
      valid: true,
    });
    expect(validateIdentifier("isbn_10", "0743273568")).toEqual({
      valid: false,
      error: "Invalid ISBN-10 checksum",
    });
  });

  it("validates isbn_13 type", () => {
    expect(validateIdentifier("isbn_13", "9780743273565")).toEqual({
      valid: true,
    });
    expect(validateIdentifier("isbn_13", "9780743273566")).toEqual({
      valid: false,
      error: "Invalid ISBN-13 checksum",
    });
  });

  it("validates asin type", () => {
    expect(validateIdentifier("asin", "B0CHVFQ31G")).toEqual({ valid: true });
    expect(validateIdentifier("asin", "invalid")).toEqual({
      valid: false,
      error: "ASIN must be 10 alphanumeric characters starting with B0",
    });
  });

  it("validates uuid type", () => {
    expect(
      validateIdentifier("uuid", "550e8400-e29b-41d4-a716-446655440000"),
    ).toEqual({ valid: true });
    expect(validateIdentifier("uuid", "invalid")).toEqual({
      valid: false,
      error: "Invalid UUID format",
    });
  });

  it("returns valid for unknown types", () => {
    expect(validateIdentifier("unknown", "anything")).toEqual({ valid: true });
  });
});
