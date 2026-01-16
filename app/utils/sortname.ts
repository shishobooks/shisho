/**
 * Articles to strip from the beginning of titles.
 * Moved to end (e.g., "The Hobbit" → "Hobbit, The").
 */
const TITLE_ARTICLES = ["The", "A", "An"];

/**
 * Generates a sort title from a display title.
 * Leading articles are moved to the end.
 *
 * @example forTitle("The Hobbit") // "Hobbit, The"
 * @example forTitle("A Tale of Two Cities") // "Tale of Two Cities, A"
 */
export function forTitle(title: string): string {
  title = title.trim();
  if (!title) {
    return "";
  }

  for (const article of TITLE_ARTICLES) {
    const prefix = article + " ";
    if (
      title.length > prefix.length &&
      title.slice(0, prefix.length).toLowerCase() === prefix.toLowerCase()
    ) {
      // Extract actual article (preserving original case)
      const actualArticle = title.slice(0, article.length);
      const rest = title.slice(prefix.length).trim();
      if (rest) {
        return rest + ", " + actualArticle;
      }
    }
  }

  return title;
}

/**
 * Generational suffixes - preserved in sort name as they distinguish people.
 */
const GENERATIONAL_SUFFIXES = [
  "Jr.",
  "Jr",
  "Sr.",
  "Sr",
  "Junior",
  "Senior",
  "I",
  "II",
  "III",
  "IV",
  "V",
];

/**
 * Academic suffixes - stripped from sort name (credentials, not name).
 */
const ACADEMIC_SUFFIXES = [
  "PhD",
  "Ph.D",
  "Ph.D.",
  "PsyD",
  "Psy.D",
  "Psy.D.",
  "MD",
  "M.D",
  "M.D.",
  "DO",
  "D.O",
  "D.O.",
  "DDS",
  "D.D.S",
  "D.D.S.",
  "JD",
  "J.D",
  "J.D.",
  "EdD",
  "Ed.D",
  "Ed.D.",
  "LLD",
  "LL.D",
  "LL.D.",
  "MBA",
  "M.B.A",
  "M.B.A.",
  "MS",
  "M.S",
  "M.S.",
  "MA",
  "M.A",
  "M.A.",
  "BA",
  "B.A",
  "B.A.",
  "BS",
  "B.S",
  "B.S.",
  "RN",
  "R.N",
  "R.N.",
  "Esq",
  "Esq.",
];

/**
 * Prefixes - honorifics/titles stripped from sort name.
 */
const PREFIXES = [
  "Dr.",
  "Dr",
  "Mr.",
  "Mr",
  "Mrs.",
  "Mrs",
  "Ms.",
  "Ms",
  "Prof.",
  "Prof",
  "Rev.",
  "Rev",
  "Fr.",
  "Fr",
  "Sir",
  "Dame",
  "Lord",
  "Lady",
];

/**
 * Name particles - moved to end with given name (library style).
 * Example: "Ludwig van Beethoven" → "Beethoven, Ludwig van"
 */
const PARTICLES = [
  "van",
  "von",
  "de",
  "da",
  "di",
  "du",
  "del",
  "della",
  "la",
  "le",
  "el",
  "al",
  "bin",
  "ibn",
];

function isPrefix(word: string): boolean {
  return PREFIXES.some((p) => p.toLowerCase() === word.toLowerCase());
}

function isGenerationalSuffix(word: string): boolean {
  // Remove trailing comma if present
  const cleaned = word.replace(/,$/, "");
  return GENERATIONAL_SUFFIXES.some(
    (s) => s.toLowerCase() === cleaned.toLowerCase(),
  );
}

function isAcademicSuffix(word: string): boolean {
  // Remove trailing comma if present
  const cleaned = word.replace(/,$/, "");
  return ACADEMIC_SUFFIXES.some(
    (s) => s.toLowerCase() === cleaned.toLowerCase(),
  );
}

function isParticle(word: string): boolean {
  return PARTICLES.some((p) => p.toLowerCase() === word.toLowerCase());
}

/**
 * Generates a sort name from a person's display name.
 * Converts to "Last, First Middle" format with proper handling of:
 * - Prefixes (Dr., Mr., etc.) - stripped
 * - Academic suffixes (PhD, MD, etc.) - stripped
 * - Generational suffixes (Jr., III, etc.) - preserved
 * - Particles (van, von, de, etc.) - moved to end with given name
 *
 * @example forPerson("Stephen King") // "King, Stephen"
 * @example forPerson("Martin Luther King Jr.") // "King, Martin Luther, Jr."
 * @example forPerson("Ludwig van Beethoven") // "Beethoven, Ludwig van"
 */
export function forPerson(name: string): string {
  name = name.trim();
  if (!name) {
    return "";
  }

  let parts = name.split(/\s+/);
  if (parts.length === 0) {
    return "";
  }

  // Single word name - return as is
  if (parts.length === 1) {
    return name;
  }

  // Strip prefixes from the beginning
  while (parts.length > 1 && isPrefix(parts[0])) {
    parts = parts.slice(1);
  }

  if (parts.length === 0) {
    return name; // All parts were prefixes, return original
  }

  if (parts.length === 1) {
    return parts[0];
  }

  // Extract and strip academic suffixes from the end, preserve generational suffixes
  const generationalSuffixes: string[] = [];
  while (parts.length > 1) {
    const last = parts[parts.length - 1];
    if (isGenerationalSuffix(last)) {
      generationalSuffixes.unshift(last);
      parts = parts.slice(0, -1);
    } else if (isAcademicSuffix(last)) {
      parts = parts.slice(0, -1);
    } else {
      break;
    }
  }

  if (parts.length === 0) {
    return name; // All parts were suffixes, return original
  }

  if (parts.length === 1) {
    // Only one part left after stripping
    if (generationalSuffixes.length > 0) {
      return parts[0] + ", " + generationalSuffixes.join(", ");
    }
    return parts[0];
  }

  // Find the last word (surname)
  const surname = parts[parts.length - 1];
  let givenParts = parts.slice(0, -1);

  // Collect consecutive particles at the end of givenParts
  const particleParts: string[] = [];
  while (givenParts.length > 0) {
    const last = givenParts[givenParts.length - 1];
    if (isParticle(last)) {
      particleParts.unshift(last);
      givenParts = givenParts.slice(0, -1);
    } else {
      break;
    }
  }

  // Build the sort name
  let result = surname;

  // Add given name parts
  if (givenParts.length > 0 || particleParts.length > 0) {
    result += ", ";
    if (givenParts.length > 0) {
      result += givenParts.join(" ");
    }
    if (particleParts.length > 0) {
      if (givenParts.length > 0) {
        result += " ";
      }
      result += particleParts.join(" ");
    }
  }

  // Add generational suffixes
  if (generationalSuffixes.length > 0) {
    result += ", " + generationalSuffixes.join(", ");
  }

  return result;
}
