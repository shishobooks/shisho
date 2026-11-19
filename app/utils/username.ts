// The rules for a new username, matching the server's `username` validator
// (pkg/binder/validators.go) and its length limits. A colon would break
// HTTP Basic sign-in (OPDS, KOReader), and spaces or invisible characters
// would let one username pass for another.

/** The hint under a username field. */
export const USERNAME_HINT =
  "3 to 50 characters: letters, numbers, and . _ - @";

// Letters, each with any marks it's written with (the vowel signs and
// accents of scripts such as Devanagari and Thai), numbers, and a few
// symbols. A mark must follow a letter.
const ALLOWED = /^(?:\p{L}\p{M}*|[\p{N}._@-])*$/u;

// Characters that render as nothing though Unicode files some under letters
// or marks (variation selectors, the combining grapheme joiner, Hangul
// fillers): they would let one username pass for another.
const INVISIBLE = /\p{Default_Ignorable_Code_Point}/u;

/** Why `username` can't be used, or null when it can. */
export const usernameProblem = (username: string): string | null => {
  // The server stores the composed (NFC) form and counts its characters, not
  // UTF-16 code units, so an accent typed separately doesn't count twice.
  const composed = username.normalize("NFC");
  const length = [...composed].length;
  if (length === 0) return "Username is required";
  if (length < 3) return "A username needs at least 3 characters";
  if (length > 50) return "A username can't be longer than 50 characters";
  if (!ALLOWED.test(composed) || INVISIBLE.test(composed)) {
    return "A username can only contain letters, numbers, and . _ - @";
  }
  return null;
};
