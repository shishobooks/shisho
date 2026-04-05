# Dynamic Plugin Install Capabilities

## Problem

The plugin install dialog (`CapabilitiesWarning.tsx`) hardcodes five capability rows (Hook Integration, File System Access, Network Access, FFmpeg Execution, Sandboxed Execution) for every plugin, regardless of what the plugin's manifest actually declares. This misleads users — e.g., the Goodreads Enricher shows "FFmpeg Execution" even though it never uses FFmpeg.

Additionally, the "Manifest v1" badge shown at the bottom of the dialog is not meaningful to end users.

## Solution

Make the install dialog show only the capabilities that the plugin actually declares, with structured details (e.g., approved domains for network access, allowed commands for shell access). Remove the "Manifest v1" badge and "Sandboxed Execution" row (sandbox is always true and belongs in docs, not the install dialog).

## Changes

### 1. Repository Manifest Format

Add a `capabilities` object to each `PluginVersion` in `repository.json`. The shape mirrors the plugin's `manifest.json` capabilities, including only the keys that are present:

```json
{
  "version": "0.1.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["epub", "m4b", "pdf"],
      "fields": ["title", "authors", "description"]
    },
    "httpAccess": {
      "domains": ["*.goodreads.com", "i.gr-assets.com", "m.media-amazon.com"]
    }
  },
  "downloadUrl": "...",
  "sha256": "..."
}
```

The `capabilities` field is optional — repositories that don't include it will result in no capability rows being shown in the dialog.

### 2. Go Backend (`pkg/plugins/repository.go`)

Add `Capabilities *Capabilities` field to `PluginVersion` struct, reusing the existing `Capabilities` type from `manifest.go`. No handler changes needed — the handler already serializes `PluginVersion` as-is.

### 3. Frontend Types (`app/hooks/queries/plugins.ts`)

Add a `capabilities` field to the `PluginVersion` interface matching the manifest capability shapes. Only fields relevant for display are needed — the full manifest types can be simplified to what the dialog uses.

### 4. Frontend Component (`app/components/plugins/CapabilitiesWarning.tsx`)

Replace hardcoded capability rows with dynamic rendering. Define a static mapping from capability keys to display config:

| Manifest Key | Icon | Label | Extra Detail |
|---|---|---|---|
| `metadataEnricher` | `Search` | Metadata Enrichment | file types list |
| `inputConverter` | `ArrowRightLeft` | Format Conversion | source types → target type |
| `fileParser` | `FileSearch` | File Parsing | file types list |
| `outputGenerator` | `FileOutput` | Output Generation | source types → format name |
| `httpAccess` | `Globe` | Network Access | domain list |
| `fileAccess` | `FolderOpen` | File System Access | read vs read/write level |
| `ffmpegAccess` | `Video` | FFmpeg Execution | — |
| `shellAccess` | `Terminal` | Shell Command Execution | command list |

Structured details (domains, commands, file types) are shown as a secondary line below the base description.

Removed from the dialog:
- "Sandboxed Execution" row (always true, documented in plugin docs instead)
- "Manifest v1" badge (not meaningful to end users)
- "Hook Integration" row (was generic, replaced by specific hook type capabilities)

If no capabilities are present (nil or empty), show a message like "No specific capabilities declared."

### 5. Documentation (`website/docs/plugins/repositories.md`)

Update the repository manifest format documentation to include the new `capabilities` field in the version schema, with an example.

## Not In Scope

- Changes to the external plugins repository (`shishobooks/plugins`) — the `repository.json` there will need the `capabilities` field added separately.
- Automated extraction of capabilities from `manifest.json` during the plugin release process — that's a plugins repo concern.
