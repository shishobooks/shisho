# Using Plugins

Plugins extend Shisho with file conversion, file parsing, metadata lookup, and additional output formats. This page covers installing and managing plugins. To create one, start with [Developing Plugins](./development.md).

## Open the Plugin Manager

Go to **Settings > Plugins**. The page has two tabs:

- **Installed** lists plugins already on this Shisho server, their versions, capabilities, status, source repository, and available updates.
- **Discover** fetches the current indexes from configured repositories. Use search, capability, and source filters to find a plugin.

Plugin management, including viewing this page, requires the **Config: Write** permission. Manual book identification requires **Books: Write** and access to the relevant library.

## Install a Plugin

1. Open **Discover**.
2. Select a plugin to review its description, version history, compatibility, homepage, and declared capabilities.
3. Select **Install**.
4. Review the capabilities in the **Install Plugin?** dialog, then select **Install Plugin**.

An incompatible plugin version cannot be installed. A repository can provide several versions, but Shisho installs the first compatible version listed in its index.

:::warning[Install only plugins you trust]
A plugin executes code on the Shisho server. The sandbox limits access, but capabilities can grant network access, broad filesystem access, FFmpeg access, or specific executable commands. The installation dialog shows capabilities supplied by the repository publisher; Shisho does not currently verify that this preview matches the downloaded plugin manifest. A SHA256 checksum confirms that the downloaded bytes match the repository index, not that the code, publisher, or preview is trustworthy. Review the publisher, homepage, source, and requested capabilities before installing.
:::

## Understand Statuses

| Status | Meaning |
|--------|---------|
| No badge | The plugin is active and loaded. |
| **Disabled** | The plugin is installed but its **Enabled** switch is off. |
| **Error** | The plugin failed to load. Open it to see the load error. |
| **Incompatible** | Its `minShishoVersion` requires a newer Shisho version. |

A plugin can be globally enabled while one of its hooks is set to **Never**. The status controls whether the plugin is loaded at all. Hook modes control when each loaded capability runs.

## Configure a Plugin

Select an installed plugin to open its detail page. The page can show:

- **Configuration** fields declared by the plugin, including masked secret fields
- **Metadata Fields** for an enricher, which globally control which declared fields it may set
- **Auto-identify confidence threshold**, which overrides the server default for that enricher
- **Hook execution order**, capabilities, manifest, version history, and update information

Select **Save** after changing configuration or global metadata field controls. The default confidence threshold is configured with `enrichment_confidence_threshold`; see [Configuration](../configuration.md).

## Set Global Order and Modes

From **Settings > Plugins**, select the gear button labeled **Advanced plugin settings**, then open **Order**.

1. Choose a **Hook Type**.
2. Use the up and down arrow buttons to set execution order.
3. Choose when each hook runs:
   - **For every new file** runs during automatic processing and is also available for manual identification.
   - **Only when manually identifying** skips automatic processing but remains available for manual identification. This choice is available only for metadata enrichers.
   - **Never** makes that hook inactive.
4. Select **Save Order**.

For metadata enrichment, order matters per field. During automatic scans, the first enricher in order that supplies an enabled field wins. File metadata fills fields that no enricher supplies. A result below the effective confidence threshold is skipped; a result without `confidence` is eligible to apply.

## Customize a Library

A library can override global hook order and modes. Open the library settings described in [Libraries](../libraries.md), find **Plugin Order**, choose a hook type, and select **Customize**. Use the mode selector and arrow buttons, then **Save**. **Reset to Default** restores the global order and modes for that hook type.

Library settings do not expose per-library metadata field controls. Enricher field switches are managed globally on the plugin detail page.

## Update or Uninstall a Plugin

When a repository reports a newer compatible version, the **Installed** tab shows an update badge and an **Update** action. The plugin detail page shows the installed version, available version, changelog, release date, and optional **View release** link. Updating replaces the installed artifact and loads the new version without a server restart.

To remove a plugin, open its detail page and select **Uninstall** in **Danger zone**. This removes the installed plugin files and saved plugin configuration. Persistent files under `plugin_data_dir` remain unless the plugin removes them during its `onUninstalling` callback. An error in that callback does not block uninstalling.

## Manage Repositories

Open **Advanced plugin settings > Repositories**.

### Add a Repository

Enter both:

- **Repository URL**: an HTTPS URL beginning with `https://raw.githubusercontent.com/`
- **Scope**: the namespace expected in that repository's `repository.json`

The entered scope must match the `scope` in the repository index. A scope identifies both the repository and its plugins, so it must not collide with another configured repository.

### Sync and Discover

**Discover** fetches enabled repository indexes live whenever its data is refreshed. **Sync** is not required to populate Discover. Sync records the latest repository name, fetch status, and last-synced time, then refreshes update indicators for installed plugins in that repository. If update refresh fails after a successful fetch, Shisho warns that update indicators may be stale.

There is no repository disable control in the current UI. The official Shisho repository cannot be removed. Removing a non-official repository leaves its installed plugins in place and working, but removes their source for repository updates and discovery.

For repository authors, see [Publishing Plugins](./publishing.md).

## Troubleshooting

- **Nothing appears in Discover:** Check the repository URL, scope, and **Sync error** in **Advanced plugin settings > Repositories**. The repository URL must use `raw.githubusercontent.com`.
- **Install fails:** Confirm the version is compatible, its ZIP URL starts with `https://github.com/`, and the repository SHA256 matches the ZIP exactly.
- **Plugin shows Error:** Open the plugin and read **Plugin failed to load**. Common causes are invalid `manifest.json`, missing `main.js`, invalid JavaScript, an undeclared hook capability, or an invalid enricher field.
- **Plugin does not run:** Check the global **Enabled** switch, the hook's mode and order, any library-specific order customization, the enricher's global **Metadata Fields**, and required plugin configuration.
- **Local changes do not appear:** Local reload is available only for active plugins with scope `local`. See [Developing Plugins](./development.md#test-in-shisho).
- **Need server evidence:** Use plugin-tagged application logs and relevant job logs. See [Troubleshooting](../troubleshooting.md).

## Developer Documentation

- [Developing Plugins](./development.md)
- [Manifest and Hooks Reference](./manifest-hooks-reference.md)
- [Host API Reference](./host-api-reference.md)
- [Testing Plugins](./testing.md)
- [Publishing Plugins](./publishing.md)
