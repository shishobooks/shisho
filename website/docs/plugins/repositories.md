---
sidebar_position: 2
---

# Repositories

Plugin repositories are curated lists of plugins hosted on GitHub. They let you discover, install, and update plugins directly from the Shisho admin interface.

## Managing Repositories

Shisho ships with the official plugin repository enabled by default. You can manage repositories in **Admin > Plugins > Repositories**.

### Adding a Repository

1. Go to **Admin > Plugins > Repositories**
2. Click **Add Repository**
3. Enter the repository URL (must be a `raw.githubusercontent.com` URL)
4. The repository will be fetched and its plugins will appear in the **Available** tab

### Syncing a Repository

Repositories cache their plugin list locally. To check for newly added plugins or updated versions, click **Sync** on the repository. This fetches the latest manifest from GitHub.

### Disabling a Repository

You can disable a repository without removing it. Disabled repositories won't show their plugins in the **Available** tab, but any already-installed plugins from that repository will continue to work.

## How Repositories Work

A repository is a JSON manifest file hosted on GitHub that lists available plugins with their versions and download URLs. When you browse the **Available** tab, Shisho reads from the cached repository manifests to show you what's installable.

Each plugin version in a repository includes:

- A **download URL** pointing to a ZIP file on GitHub Releases
- A **SHA256 hash** for verifying the download integrity
- A **minimum Shisho version** for compatibility filtering
- A **changelog** describing what changed
- An optional **release URL** linking to the version's release page (shown as "View release" on the version card)
- An optional **capabilities** object declaring what the plugin can do (shown during install)

Plugin versions that require a newer version of Shisho are marked as **Incompatible** in the Browse tab and cannot be installed. Compatible versions remain installable as normal.

## Creating a Plugin Repository

If you want to create your own repository of plugins, you need to host a JSON manifest file on GitHub.

### Repository Manifest Format

Create a `repository.json` file with this structure:

```json
{
  "repositoryVersion": 1,
  "scope": "my-org",
  "name": "My Plugin Repository",
  "plugins": [
    {
      "id": "my-plugin",
      "name": "My Plugin",
      "description": "A brief description of what the plugin does.",
      "homepage": "https://github.com/my-org/my-plugin",
      "imageUrl": "https://raw.githubusercontent.com/my-org/my-plugin/main/logo.png",
      "versions": [
        {
          "version": "1.0.0",
          "minShishoVersion": "0.1.0",
          "manifestVersion": 1,
          "releaseDate": "2025-06-15",
          "changelog": "Initial release.",
          "downloadUrl": "https://github.com/my-org/my-plugin/releases/download/v1.0.0/my-plugin.zip",
          "releaseUrl": "https://github.com/my-org/my-plugin/releases/tag/v1.0.0",
          "sha256": "abc123...",
          "capabilities": {
            "metadataEnricher": {
              "fileTypes": ["epub", "m4b"],
              "fields": ["title", "authors", "description", "cover"]
            },
            "httpAccess": {
              "domains": ["*.example.com"]
            }
          }
        }
      ]
    }
  ]
}
```

### Field notes

- **`homepage`** (on each plugin entry): Optional. The plugin's landing page — shown as the "homepage" link on the plugin detail page. For multi-plugin repositories, point this at the plugin's own page (e.g. `https://github.com/my-org/my-plugins/tree/main/plugins/my-plugin`) rather than the repository root. Shisho uses this field purely for display — release links come from `releaseUrl` on each version, not from `homepage`.
- **`imageUrl`** (on each plugin entry): Plugin logo URL. Recommended 256×256 PNG or SVG (SVG preferred), 1:1 aspect ratio, centered mark with ≥10% safe area; any HTTPS URL works (GitHub raw is fine). Shisho renders it at full size inside a rounded square that scales with display size — the artwork fills the tile (kept in aspect with `object-fit: contain`), so authors should bake any desired background or padding into the image itself. When `imageUrl` is missing or fails to load, Shisho falls back to hashed-color initials derived from `scope/id`.
- **`releaseDate`** (on each version entry): Optional. Accepts RFC3339 (`2026-04-14T00:00:00Z`) or date-only (`2026-04-14`). When omitted, the "Released" line is hidden on the version card. Repository manifests are validated at fetch time; versions with an invalid `releaseDate` are skipped (and a warning is logged server-side).
- **`releaseUrl`** (on each version entry): Optional. Full URL to the release page for this version — any HTTPS URL works (GitHub release, GitLab tag, Codeberg release, etc.). When present, Shisho renders a "View release" link on the version card. When omitted, no link is shown. Shisho does not validate the host or path — it renders the URL verbatim.
- **`changelog`** (on each version entry): Rendered as sanitized markdown on the plugin detail page. Supported subset: headings (`##`, `###`), paragraphs, lists, inline code, fenced code blocks, links (open in a new tab), bold, italic. Raw HTML, images, and iframes are stripped — author content accordingly. The "View release" link shown alongside the changelog is controlled by `releaseUrl` on the version, not inferred from `homepage`.

### Key Rules

- **Repository URLs** must be on `raw.githubusercontent.com`
- **Download URLs** must be on `github.com` (typically GitHub Releases)
- **SHA256 hashes** are required and verified on install
- **Scope** must be unique across all repositories — it acts as a namespace for the plugins
- **`manifestVersion`** in each version entry must match the plugin's actual `manifest.json` version

### Capabilities

The optional `capabilities` object in each version mirrors the plugin's `manifest.json` capabilities. It tells users what the plugin can do before they install it. When present, the install dialog shows only the declared capabilities instead of a generic list.

Supported capability keys:

| Key | Description | Detail Shown |
|-----|-------------|--------------|
| `metadataEnricher` | Searches external sources for book metadata | File types |
| `inputConverter` | Converts files between formats | Source → target types |
| `fileParser` | Extracts metadata from files | File types |
| `outputGenerator` | Generates files in additional formats | Source types → format name |
| `httpAccess` | May make network requests to external services | Approved domains |
| `fileAccess` | Can access files beyond its sandboxed plugin directory | Access level |
| `ffmpegAccess` | May invoke FFmpeg for media processing | — |
| `shellAccess` | May execute shell commands on your system | Allowed commands |

See the [Development](./development) page for the full capability schema.

### Hosting

1. Create a GitHub repository for your plugin collection
2. Add a `repository.json` at the root (or any path)
3. Users add the raw GitHub URL: `https://raw.githubusercontent.com/my-org/my-plugins/main/repository.json`

### Plugin ZIP Structure

Each plugin ZIP should contain the plugin files at the root level:

```
my-plugin.zip
  manifest.json
  main.js
```

See the [Development](./development) page for details on the manifest and plugin code format.
