// The same rule as pkg/tags/namespaces.go, which the tag list filters by:
// a tag is in a namespace when its name starts with the namespace and a
// colon, compared case-insensitively for ASCII letters only (as SQLite's LIKE
// compares), and something other than spaces follows. Change both together.

export interface SplitTag {
  /** The library namespace the tag is in, as configured, or null. */
  namespace: string | null;
  /** The name to show: without the prefix when namespaced. */
  name: string;
}

const foldASCII = (s: string) => s.replace(/[A-Z]/g, (c) => c.toLowerCase());

export const splitTagNamespace = (
  tagName: string,
  namespaces: readonly string[],
): SplitTag => {
  const folded = foldASCII(tagName);
  for (const namespace of namespaces) {
    const prefix = foldASCII(namespace) + ":";
    if (!folded.startsWith(prefix)) continue;
    const name = tagName.slice(prefix.length).replace(/^ +/, "");
    if (name !== "") return { namespace, name };
  }
  return { namespace: null, name: tagName };
};

/**
 * Which tags a tag picker lists: every tag, the plain tags outside every
 * namespace, or one namespace's tags.
 */
export type TagScope =
  | { kind: "all" }
  | { kind: "plain" }
  | { kind: "namespace"; namespace: string };

export const ALL_TAGS_SCOPE: TagScope = { kind: "all" };

/** The tag list query params that list a scope's tags. */
export const tagScopeParams = (
  scope: TagScope,
): { namespace?: string; exclude_namespaces?: boolean } => {
  switch (scope.kind) {
    case "all":
      return {};
    case "plain":
      return { exclude_namespaces: true };
    case "namespace":
      return { namespace: scope.namespace };
  }
};

/** A namespace being edited, with a row key that survives reordering. */
export interface DraftNamespace {
  key: string;
  name: string;
}

let draftKeyCounter = 0;
export const draftNamespace = (name = ""): DraftNamespace => ({
  key: `namespace-${++draftKeyCounter}`,
  name,
});
