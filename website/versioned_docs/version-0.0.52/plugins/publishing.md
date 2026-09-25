# Publishing Plugins

Shisho discovers third-party plugins through a publisher-maintained `repository.json`. Publishing consists of building a release ZIP, hosting it on GitHub, calculating its checksum, and updating your repository index. Shisho does not provide an automated third-party submission or publishing service.

Before publishing, complete the real-runtime checks in [Testing Plugins](./testing.md).

## Package a Release

A release ZIP must contain `manifest.json` and `main.js` at its root:

```text
example-metadata-1.2.0.zip
  manifest.json
  main.js
  templates/
    result.txt
  LICENSE
```

Additional files and directories are allowed. Do not wrap the artifact in another top-level directory, because Shisho reads `manifest.json` and `main.js` directly from the extraction root.

The artifact must satisfy these contracts:

- `manifest.json` uses `manifestVersion: 1` and has the required `id`, `name`, and `version` fields.
- The manifest ID and version match the repository entry.
- Executing `main.js` creates the global `plugin` object.
- Runtime dependencies are bundled or included as files the plugin reads through the host API.
- The bundle does not rely on Node.js or browser-only APIs.
- Every hook and permission is declared accurately.

:::warning[Verify Repository and Artifact Identity]
Shisho currently verifies the ZIP host, checksum, and manifest syntax, but it does not reject every mismatch between the repository entry and the artifact's manifest ID or version. Publishers must verify those values themselves. A mismatch can install a plugin under inconsistent identity or version data.
:::

See [Manifest and Hooks Reference](./manifest-hooks-reference.md) and [Host API Reference](./host-api-reference.md).

## Host the ZIP on GitHub

Each `downloadUrl` must begin with:

```text
https://github.com/
```

A GitHub Release asset is the usual choice:

```text
https://github.com/example/shisho-example-metadata/releases/download/v1.2.0/example-metadata-1.2.0.zip
```

Other download hosts and GitHub API URLs are rejected. Make the asset stable: replacing bytes at the same URL without changing the version and checksum can leave repository users with inconsistent release information.

## Calculate SHA256

Calculate the checksum from the exact uploaded ZIP bytes:

```bash
shasum -a 256 example-metadata-1.2.0.zip
```

On systems with GNU coreutils:

```bash
sha256sum example-metadata-1.2.0.zip
```

Copy the complete lowercase hexadecimal digest into `sha256`. Shisho verifies it before extraction and rejects a mismatch.

## Create repository.json

A repository index has one scope and any number of plugins:

```json
{
  "repositoryVersion": 1,
  "scope": "example",
  "name": "Example Shisho Plugins",
  "plugins": [
    {
      "id": "example-metadata",
      "name": "Example Metadata",
      "overview": "Finds book metadata from Example Books.",
      "description": "Searches Example Books and returns edition metadata.",
      "homepage": "https://github.com/example/shisho-example-metadata",
      "imageUrl": "https://github.com/example/shisho-example-metadata/raw/main/icon.png",
      "versions": [
        {
          "version": "1.2.0",
          "minShishoVersion": "0.0.49",
          "manifestVersion": 1,
          "releaseDate": "2026-04-15",
          "changelog": "Adds language and abridged metadata.",
          "downloadUrl": "https://github.com/example/shisho-example-metadata/releases/download/v1.2.0/example-metadata-1.2.0.zip",
          "releaseUrl": "https://github.com/example/shisho-example-metadata/releases/tag/v1.2.0",
          "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
          "capabilities": {
            "metadataEnricher": {
              "description": "Searches Example Books",
              "fileTypes": ["epub", "m4b"],
              "fields": [
                "title",
                "authors",
                "description",
                "language",
                "abridged",
                "cover"
              ]
            },
            "httpAccess": {
              "description": "Calls the metadata and cover services",
              "domains": ["api.example.com", "covers.example.com"]
            }
          }
        }
      ]
    }
  ]
}
```

The [official Shisho plugin repository](https://github.com/shishobooks/plugins) is a useful public example of the format and release layout.

## Repository Fields

| Field | Required | Contract |
|-------|----------|----------|
| `repositoryVersion` | Yes | Integer `1`. |
| `scope` | Yes for management | Stable namespace shared by every plugin in the index. It must match the scope entered when an administrator adds the repository. |
| `name` | Recommended | Repository display name refreshed by **Sync**. |
| `plugins` | Yes | Array of plugin entries. |

An administrator supplies both the raw URL and scope. The scope must be unique among configured repositories. Changing it after users install plugins changes their identity and breaks the repository association, so choose it once and keep it stable.

## Plugin Entry Fields

| Field | Required | Contract |
|-------|----------|----------|
| `id` | Yes | Stable ID, unique within the repository scope and matching the artifact manifest. |
| `name` | Yes | Display name in Discover. |
| `overview` | No | Short one-line summary. Use this for quick scanning. |
| `description` | No | Longer description displayed on plugin pages. |
| `homepage` | No | Stable plugin landing page or source repository. It is independent of version release links. |
| `imageUrl` | No | Plugin artwork URL. A GitHub-hosted URL is recommended so installation can retain the icon. |
| `versions` | Yes | Version entries ordered newest first. |

For a multi-plugin repository, point `homepage` at the individual plugin's directory or site rather than only the index repository root.

## Version Entry Fields

| Field | Required | Contract |
|-------|----------|----------|
| `version` | Yes | Version string matching `manifest.json`. Use semantic versions for reliable update comparison. |
| `minShishoVersion` | Recommended | Minimum Shisho version for this release. Empty means no declared minimum. |
| `manifestVersion` | Yes | Manifest format used by the artifact. Currently `1`. Unsupported manifest versions are not shown. |
| `releaseDate` | No | RFC3339 timestamp or `YYYY-MM-DD`. A version with any other non-empty format is skipped. |
| `changelog` | No | Release notes shown in version history. |
| `downloadUrl` | Yes | ZIP URL beginning with `https://github.com/`. |
| `releaseUrl` | No | Explicit HTTPS page for this release, used by **View release**. It is not inferred from `homepage`. |
| `sha256` | Yes | SHA256 digest of the exact ZIP. |
| `capabilities` | Strongly recommended | Capabilities and permissions for this specific version, shown before installation. |

### Keep Versions Newest First

Order `versions` from newest to oldest. When no explicit version is requested, the current install service selects the first compatible entry. The UI also treats index order as version history order. Update detection compares versions, but correct ordering is still required for first-time installation and presentation.

Do not put an incompatible newest version after older versions and expect Shisho to infer your intended order. Instead, keep strict newest-first order and set `minShishoVersion` accurately so Shisho can choose the first compatible release.

### Publish Capabilities Per Version

Repository capabilities mirror the artifact manifest's `capabilities` object. Include them on every version, because permissions and hooks can change between releases and administrators review the selected version before installing.

Keep the index and artifact identical for:

- four hook capabilities and their extensions, formats, file types, and enricher fields
- custom identifier types
- HTTP domains
- filesystem access level
- FFmpeg access
- shell commands

An omitted repository capability does not remove the artifact's runtime declaration. It only prevents an accurate pre-install review, which is why omission is discouraged.

## Host the Repository Index

Shisho accepts repository URLs only when they begin with:

```text
https://raw.githubusercontent.com/
```

For example:

```text
https://raw.githubusercontent.com/example/shisho-plugins/main/repository.json
```

A normal `github.com` blob page, GitHub API URL, redirect service, GitLab raw URL, or custom website is rejected even if it serves the same JSON.

After committing the index, test the exact raw URL without authentication. In Shisho, add that URL and the matching scope under **Settings > Plugins > Advanced plugin settings > Repositories**. **Discover** fetches the live index. **Sync** records fetch status and refreshes installed-plugin update indicators.

## Release Checklist

### Artifact

- [ ] Build production `main.js` from the tagged source.
- [ ] Confirm `main.js` creates the global `plugin` object.
- [ ] Put `manifest.json` and `main.js` at the ZIP root.
- [ ] Confirm the artifact manifest ID and version match the repository entry.
- [ ] Include any additional runtime files with stable relative paths.
- [ ] Test the exact ZIP contents in a real Shisho server.

### Security and Compatibility

- [ ] Remove unused HTTP domains, file access, FFmpeg access, and shell commands.
- [ ] Set `minShishoVersion` from the oldest Shisho release actually tested.
- [ ] Verify each declared hook, enricher field, and output format.
- [ ] Review logs to ensure secrets and book content are not exposed.

### GitHub Release

- [ ] Upload the ZIP to a GitHub URL beginning with `https://github.com/`.
- [ ] Calculate SHA256 after creating the final ZIP.
- [ ] Publish a stable release page and use it as `releaseUrl` when available.

### Repository Index

- [ ] Add the version at the start of the newest-first `versions` array.
- [ ] Copy version, compatibility, capabilities, and permissions from the artifact manifest.
- [ ] Add release date and changelog.
- [ ] Verify the raw index URL begins with `https://raw.githubusercontent.com/`.
- [ ] Add the repository to a test Shisho server with the matching scope.
- [ ] Review Discover, the install capability dialog, installation, version history, and update detection.
