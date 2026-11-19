// The rules for a new username, matching the server's `username` validator
// (pkg/binder/validators.go) and its length limits. A colon would break
// HTTP Basic sign-in (OPDS, KOReader), and spaces or invisible characters
// would let one username pass for another.

/** The hint under a username field. */
export const USERNAME_HINT =
  "3 to 50 characters: letters, numbers, and . _ - @";

const ALLOWED = /^[\p{L}\p{N}._@-]*$/u;

/** Why `username` can't be used, or null when it can. */
export const usernameProblem = (username: string): string | null => {
  // The server counts characters, not UTF-16 code units.
  const length = [...username].length;
  if (length === 0) return "Username is required";
  if (length < 3) return "A username needs at least 3 characters";
  if (length > 50) return "A username can't be longer than 50 characters";
  if (!ALLOWED.test(username)) {
    return "A username can only contain letters, numbers, and . _ - @";
  }
  return null;
};
