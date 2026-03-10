---
sidebar_position: 1
---

# Overview

Shisho has a plugin system that lets you extend its functionality with JavaScript. Plugins hook into the book processing pipeline to add support for new file formats, enrich metadata from external sources, and generate alternative output formats.

## What Plugins Can Do

There are four types of plugins, each serving a different purpose:

| Type | Purpose | Example |
|------|---------|---------|
| **Input Converter** | Convert unsupported file formats into supported ones | MOBI to EPUB |
| **File Parser** | Extract metadata from file formats Shisho doesn't natively support | PDF metadata extraction |
| **Metadata Enricher** | Look up and add metadata from external APIs | Fetching descriptions and genres from Goodreads |
| **Output Generator** | Generate alternative download formats from existing files | EPUB to MOBI conversion |

Plugins run in a sandboxed environment with controlled access to the filesystem, network, and system commands. Each plugin declares exactly what permissions it needs in its manifest.

## Installing Plugins

Plugins are installed from **[plugin repositories](./repositories)** — curated lists of available plugins hosted on GitHub. Shisho ships with an official repository enabled by default.

To install a plugin:

1. Go to **Admin > Plugins**
2. Browse the **Available** tab to see plugins from enabled repositories
3. Click **Install** on the plugin you want

Plugins can also be installed by placing them directly in the plugin directory for [development or testing](./development#local-development) purposes.

## Configuring Plugins

Many plugins have configuration options, such as API keys for external services. After installing a plugin:

1. Go to **Admin > Plugins**
2. Click the gear icon on the installed plugin
3. Fill in the required configuration fields
4. Save

### Execution Order

When multiple plugins of the same type are installed (e.g., two metadata enrichers), you can control the order they run in. Go to **Admin > Plugins** and drag plugins to reorder them within each hook type.

### Per-Library Settings

Plugins can be customized on a per-library basis. In the library settings, you can:

- **Enable or disable** specific plugins for that library
- **Reorder** plugin execution for that library
- **Toggle individual fields** for metadata enrichers (e.g., allow a plugin to set genres but not the description)

If no per-library customization is set, the global plugin settings apply.

### Field Controls for Metadata Enrichers

Metadata enrichers declare which fields they may modify (e.g., description, genres, cover). You can control this at two levels:

- **Global**: Enable or disable specific fields for a plugin across all libraries
- **Per-library**: Override the global setting for individual libraries

This gives you fine-grained control over what metadata each plugin is allowed to change.

## Updating Plugins

When a new version of a plugin is available, you'll see an update indicator in **Admin > Plugins**. Click **Update** to install the latest version. Updates are applied instantly without restarting the server.

## Plugin Security

Plugins run in a sandboxed JavaScript environment. Each plugin must declare its required permissions in its manifest:

- **HTTP access**: Which domains the plugin can make requests to
- **File access**: Whether the plugin can read or write files beyond its own directory
- **FFmpeg access**: Whether the plugin can use FFmpeg for media processing
- **Shell access**: Which specific shell commands the plugin can execute

These permissions are visible when installing a plugin so you can review what access it needs before enabling it.
