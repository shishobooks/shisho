// Identifier validation utilities

export function validateISBN10(isbn: string): boolean {
  const normalized = isbn.replace(/[-\s]/g, "").toUpperCase();
  if (normalized.length !== 10) return false;

  let sum = 0;
  for (let i = 0; i < 10; i++) {
    const char = normalized[i];
    const digit = char === "X" ? 10 : parseInt(char, 10);
    if (isNaN(digit) && char !== "X") return false;
    if (char === "X" && i !== 9) return false;
    sum += digit * (10 - i);
  }
  return sum % 11 === 0;
}

export function validateISBN13(isbn: string): boolean {
  const normalized = isbn.replace(/[-\s]/g, "");
  if (normalized.length !== 13) return false;

  let sum = 0;
  for (let i = 0; i < 13; i++) {
    const digit = parseInt(normalized[i], 10);
    if (isNaN(digit)) return false;
    sum += digit * (i % 2 === 0 ? 1 : 3);
  }
  return sum % 10 === 0;
}

export function validateASIN(asin: string): boolean {
  return /^B0[A-Z0-9]{8}$/i.test(asin);
}

export function validateUUID(uuid: string): boolean {
  return /^(?:urn:uuid:)?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    uuid,
  );
}

/**
 * Returns a URL for an identifier value using the type's urlTemplate, or null if unavailable.
 * Replaces {value} in the template with the actual identifier value.
 */
export function getIdentifierUrl(
  type: string,
  value: string,
  pluginTypes?: Array<{ id: string; url_template?: string }>,
): string | null {
  const pluginType = pluginTypes?.find((pt) => pt.id === type);
  if (pluginType?.url_template) {
    return pluginType.url_template.replace(
      "{value}",
      encodeURIComponent(value),
    );
  }
  return null;
}

export function validateIdentifier(
  type: string,
  value: string,
  pattern?: string,
): { valid: boolean; error?: string } {
  switch (type) {
    case "isbn_10":
      return validateISBN10(value)
        ? { valid: true }
        : { valid: false, error: "Invalid ISBN-10 checksum" };
    case "isbn_13":
      return validateISBN13(value)
        ? { valid: true }
        : { valid: false, error: "Invalid ISBN-13 checksum" };
    case "asin":
      return validateASIN(value)
        ? { valid: true }
        : {
            valid: false,
            error: "ASIN must be 10 alphanumeric characters starting with B0",
          };
    case "uuid":
      return validateUUID(value)
        ? { valid: true }
        : { valid: false, error: "Invalid UUID format" };
    default:
      if (pattern) {
        const regex = new RegExp(pattern);
        if (!regex.test(value)) {
          return {
            valid: false,
            error: "Value does not match the required format",
          };
        }
      }
      return { valid: true };
  }
}
