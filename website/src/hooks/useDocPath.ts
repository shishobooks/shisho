import {
  useLatestVersion,
  useVersions,
} from "@docusaurus/plugin-content-docs/client";

/** The preset registers a single docs plugin instance under this ID. */
const docsPluginId = "default";

/**
 * Resolves the served path of a doc by ID, preferring the latest released
 * version and falling back to the current (unreleased) docs.
 *
 * `onBrokenLinks: "throw"` fails the build when a page links to `/docs/<id>`
 * and the latest release snapshot has no such doc. Use this for links to pages
 * that were added since the last release so they resolve to
 * `/docs/unreleased/<id>` until the next release snapshots them.
 */
export const useDocPath = (docId: string): string => {
  const latest = useLatestVersion(docsPluginId);
  const versions = useVersions(docsPluginId);

  const released = latest.docs.find((doc) => doc.id === docId);
  if (released) {
    return released.path;
  }

  const current = versions
    .find((version) => version.name === "current")
    ?.docs.find((doc) => doc.id === docId);
  if (current) {
    return current.path;
  }

  throw new Error(`Doc "${docId}" does not exist in any docs version`);
};
