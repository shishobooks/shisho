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

Shisho only shows plugin versions that are compatible with your current server version.

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
      "author": "Your Name",
      "homepage": "https://github.com/my-org/my-plugin",
      "versions": [
        {
          "version": "1.0.0",
          "minShishoVersion": "0.1.0",
          "manifestVersion": 1,
          "releaseDate": "2025-06-15",
          "changelog": "Initial release.",
          "downloadUrl": "https://github.com/my-org/my-plugin/releases/download/v1.0.0/my-plugin.zip",
          "sha256": "abc123..."
        }
      ]
    }
  ]
}
```

### Key Rules

- **Repository URLs** must be on `raw.githubusercontent.com`
- **Download URLs** must be on `github.com` (typically GitHub Releases)
- **SHA256 hashes** are required and verified on install
- **Scope** must be unique across all repositories — it acts as a namespace for the plugins
- **`manifestVersion`** in each version entry must match the plugin's actual `manifest.json` version

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
