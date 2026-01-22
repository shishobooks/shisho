# Plugin System Design

This document describes the architecture for Shisho's plugin system, enabling third-party developers to extend functionality through JavaScript plugins.

## Overview

The plugin system allows users to install plugins that:
- Convert file formats to other formats (PDF → EPUB, EPUB → MOBI, etc.)
- Parse new file formats for metadata extraction
- Generate new output/download formats (MOBI, AZW3, etc.)
- Enrich metadata from external sources (Goodreads, OpenLibrary, etc.)
- Register custom identifier types

Plugins are JavaScript files executed in an embedded interpreter (goja). Plugin developers write TypeScript, bundle it to JavaScript, and distribute via GitHub.

## Architecture Decisions

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Runtime | goja (pure Go JS interpreter) | No CGO, good performance, ES5.1+ support |
| Plugin language | JavaScript (TS bundled to JS) | Large developer pool, familiar tooling |
| Manifest format | JSON with camelCase | Idiomatic for JS/TS developers |
| Repository hosting | GitHub only | Security (no arbitrary domains), reliable CDN |
| Config storage | Database | UI-editable, survives plugin updates |
| Processing order | User-defined | Deterministic, explicit control |
| Load timing | App startup + hot-reload | Initial load at startup; install/update/enable hot-reloads without restart |

## Versioning Strategy

Two version fields control plugin compatibility:

- **`manifestVersion`** - The plugin API contract version. When Shisho makes breaking changes to plugin APIs, it introduces a new manifest version. Shisho declares which manifest versions it supports; plugins with unsupported versions receive a clear incompatibility error. This is the primary compatibility mechanism.

- **`minShishoVersion`** - The minimum Shisho release required for a non-breaking feature dependency (e.g., "I need the archive API added in 1.3"). Does not imply breaking changes.

This means plugin authors don't need to predict future breaking changes (no `maxShishoVersion`). Shisho controls the compatibility boundary through manifest versions.

## Plugin Structure

A plugin is a directory containing:

```
my-plugin/
├── manifest.json      # Required: metadata, capabilities, config schema
├── main.js            # Required: bundled entry point
└── assets/            # Optional: icons, static files
```

### Manifest Format

```json
{
  "manifestVersion": 1,
  "id": "goodreads-metadata",
  "name": "Goodreads Metadata Provider",
  "version": "1.0.0",
  "description": "Fetches book metadata from Goodreads",
  "author": "Community Developer",
  "homepage": "https://github.com/example/goodreads-plugin",
  "license": "MIT",
  "minShishoVersion": "1.0.0",

  "capabilities": {
    "metadataEnricher": {
      "description": "Fetches additional metadata from Goodreads API",
      "fileTypes": ["epub", "cbz", "m4b"]
    },
    "identifierTypes": [
      {
        "id": "goodreads",
        "name": "Goodreads ID",
        "urlTemplate": "https://www.goodreads.com/book/show/{value}",
        "pattern": "^[0-9]+$"
      }
    ],
    "httpAccess": {
      "description": "Calls Goodreads API to fetch book data",
      "domains": ["goodreads.com", "api.goodreads.com"]
    }
  },

  "configSchema": {
    "apiKey": {
      "type": "string",
      "label": "Goodreads API Key",
      "description": "Your Goodreads developer API key",
      "required": true,
      "secret": true
    },
    "includeReviews": {
      "type": "boolean",
      "label": "Include Reviews",
      "default": false
    },
    "timeout": {
      "type": "number",
      "label": "Request Timeout (seconds)",
      "default": 30,
      "min": 5,
      "max": 120
    },
    "outputFormat": {
      "type": "select",
      "label": "Output Format",
      "options": [
        { "value": "epub", "label": "EPUB" },
        { "value": "mobi", "label": "MOBI" }
      ],
      "default": "epub"
    },
    "customTemplate": {
      "type": "textarea",
      "label": "Custom Template",
      "description": "Template for metadata formatting"
    }
  }
}
```

### Capability Types

**v1 (Priority):**
- `inputConverter` - Convert file formats during scan
- `fileParser` - Parse new file formats for metadata extraction
- `outputGenerator` - Generate download formats
- `metadataEnricher` - Enrich metadata during sync
- `identifierTypes` - Register custom identifier types
- `httpAccess` - Make HTTP requests (with domain restrictions)
- `fileAccess` - Access library files (read or read-write)

**Future (v2+):**
- `apiEndpoints` - Register custom API routes
- `uiComponents` - Provide React components
- `sidecarFields` - Add custom sidecar fields

### HTTP Access Rules

The `httpAccess` capability declares allowed domains:

```json
{
  "httpAccess": {
    "description": "Calls Goodreads API to fetch book data",
    "domains": ["goodreads.com", "api.goodreads.com"]
  }
}
```

Security rules:
- **Subdomains**: Must be listed explicitly. `goodreads.com` does not allow `api.goodreads.com`.
- **Redirects**: Requests that redirect to an unlisted domain are blocked. The plugin receives an error.
- **Ports**: Only standard ports (80/443) unless explicitly specified (e.g., `example.com:8080`).

### File Access Levels

Without `fileAccess`, plugins can only access their own directory and temp dirs. The `fileAccess` capability grants access to library files:

```json
{
  "fileAccess": {
    "level": "read",
    "description": "Reads source files for metadata extraction"
  }
}
```

Levels:
- `"read"` - Read-only access to library files
- `"readwrite"` - Read and write access to library files (write implies read)

The level is displayed to the user during installation so they understand what the plugin can do.

## Plugin APIs

Plugins access Shisho functionality through a global `shisho` object:

```typescript
declare namespace shisho {
  // Logging
  function log(level: 'debug' | 'info' | 'warn' | 'error', message: string): void;

  // Configuration (plugin-specific settings)
  namespace config {
    function get<T>(key: string): T | undefined;
    function getAll(): Record<string, unknown>;
  }

  // HTTP client (respects httpAccess capability)
  namespace http {
    function fetch(url: string, options?: RequestOptions): Promise<Response>;
  }

  // File operations (respects fileAccess capability)
  namespace fs {
    function readFile(path: string): Promise<Uint8Array>;
    function readTextFile(path: string): Promise<string>;
    function writeFile(path: string, data: Uint8Array): Promise<void>;
    function writeTextFile(path: string, content: string): Promise<void>;
    function exists(path: string): Promise<boolean>;
    function mkdir(path: string): Promise<void>;
    function listDir(path: string): Promise<string[]>;
    function tempDir(): string;
  }

  // Archive utilities (ZIP-only for v1; namespace allows future format expansion)
  namespace archive {
    function extractZip(archivePath: string, destDir: string): Promise<void>;
    function createZip(srcDir: string, destPath: string): Promise<void>;
    function readZipEntry(archivePath: string, entryPath: string): Promise<Uint8Array>;
    function listZipEntries(archivePath: string): Promise<string[]>;
  }

  // XML/HTML parsing (supports namespaced selectors via namespace map)
  namespace xml {
    function parse(content: string): XMLDocument;
    function querySelector(doc: XMLDocument, selector: string, namespaces?: Record<string, string>): XMLElement | null;
    function querySelectorAll(doc: XMLDocument, selector: string, namespaces?: Record<string, string>): XMLElement[];
    // Usage: querySelector(doc, 'dc|title', { dc: 'http://purl.org/dc/elements/1.1/' })
  }
}
```

## Hook Interfaces

### Input Converter

Converts file formats to other formats during library scan. The converter writes a new file alongside the original. Both files are indexed independently based on parser availability:
- If a parser exists for the original format (built-in or plugin), it's tracked as a main file
- If no parser exists for the original format, it's tracked as a supplement file (visible in UI, downloadable, but no parsed metadata)

Converter success or failure does not affect how the original file is treated.

When multiple converters handle the same source type, all run independently on the original file. For example, a PDF→EPUB converter and a PDF→MOBI converter both produce their own output from the same PDF. Ordering controls execution sequence, not exclusivity.

```typescript
export const inputConverter: shisho.InputConverter = {
  // Types this converter handles
  sourceTypes: ['pdf', 'mobi', 'doc'],

  // What it converts to
  targetType: 'epub',

  async convert(context: ConvertContext): Promise<ConvertResult> {
    const { sourcePath, targetDir } = context;

    // Read source file
    const data = await shisho.fs.readFile(sourcePath);

    // Plugin controls the output filename (useful for adding metadata like narrator)
    const sourceBaseName = sourcePath.split('/').pop().replace(/\.[^.]+$/, '');
    const targetPath = `${targetDir}/${sourceBaseName}.epub`;

    // ... conversion logic ...

    await shisho.fs.writeFile(targetPath, convertedData);

    return {
      success: true,
      targetPath: targetPath,
    };
  }
};
```

**Manifest capability:**

```json
{
  "capabilities": {
    "inputConverter": {
      "description": "Converts PDF and MOBI files to EPUB for indexing",
      "sourceTypes": ["pdf", "mobi"],
      "targetType": "epub"
    }
  }
}
```

### File Parser

Extracts metadata from file formats. Use this when you want Shisho to natively support a format (track it, display metadata, allow downloads) without converting it.

```typescript
export const fileParser: shisho.FileParser = {
  types: ['pdf', 'djvu'],
  mimeTypes: ['application/pdf'],

  async parse(context: FileParseContext): Promise<ParsedMetadata> {
    const { filePath } = context;
    const data = await shisho.fs.readFile(filePath);

    return {
      title: '...',
      authors: [{ name: 'Author Name', role: 'author' }],
      description: '...',
      identifiers: [],
      coverData: null,
      coverMimeType: null,
    };
  }
};
```

### Output Generator

```typescript
export const outputGenerator: shisho.OutputGenerator = {
  id: 'mobi',
  name: 'MOBI (Kindle)',
  sourceTypes: ['epub'],

  async generate(context: GenerateContext): Promise<void> {
    const { sourcePath, destPath, book, file } = context;
    // Convert source to output format, write to destPath
  }
};
```

### Metadata Enricher

```typescript
export const metadataEnricher: shisho.MetadataEnricher = {
  name: 'Goodreads Metadata',
  fileTypes: ['epub', 'cbz', 'm4b'],
  priority: 50,

  async enrich(context: EnrichmentContext): Promise<EnrichmentResult> {
    const { book, file, parsedMetadata } = context;

    const isbn = parsedMetadata.identifiers.find(id => id.type === 'isbn');
    if (!isbn) return { modified: false };

    const apiKey = shisho.config.get<string>('apiKey');
    const response = await shisho.http.fetch(
      `https://api.goodreads.com/book/isbn/${isbn.value}?key=${apiKey}`
    );

    if (!response.ok) return { modified: false };
    const data = await response.json();

    return {
      modified: true,
      metadata: {
        description: data.description,
        genres: data.genres,
        identifiers: [{ type: 'goodreads', value: data.id }]
      }
    };
  }
};
```

### Identifier Types

```typescript
export const identifierTypes: shisho.IdentifierType[] = [
  {
    id: 'goodreads',
    name: 'Goodreads ID',
    urlTemplate: 'https://www.goodreads.com/book/show/{value}',
    pattern: '^[0-9]+$',
  }
];
```

### Multiple Hooks

A plugin can export multiple hooks:

```typescript
// PDF plugin with conversion, parsing, and generation
export const inputConverter: shisho.InputConverter = { /* ... */ };
export const fileParser: shisho.FileParser = { /* ... */ };
export const outputGenerator: shisho.OutputGenerator = { /* ... */ };
export const identifierTypes: shisho.IdentifierType[] = [ /* ... */ ];
```

## Repository System

### Repository Manifest Format

Repositories are GitHub-hosted JSON files listing available plugins:

```json
{
  "repositoryVersion": 1,
  "scope": "shisho",
  "name": "Official Shisho Plugins",
  "plugins": [
    {
      "id": "goodreads-metadata",
      "name": "Goodreads Metadata",
      "description": "Fetches book metadata from Goodreads",
      "author": "Shisho Team",
      "versions": [
        {
          "version": "1.2.0",
          "minShishoVersion": "1.1.0",
          "releaseDate": "2025-01-15",
          "changelog": "Added series detection",
          "downloadUrl": "https://github.com/shishobooks/plugins/releases/download/goodreads-1.2.0/goodreads-metadata.zip",
          "sha256": "abc123..."
        }
      ],
      "capabilities": ["metadataEnricher", "identifierTypes", "httpAccess"],
      "iconUrl": "https://..../icon.png"
    }
  ]
}
```

### URL Restrictions

For security, only GitHub URLs are allowed:
- Repository manifests: `https://raw.githubusercontent.com/{owner}/{repo}/...`
- Plugin downloads: `https://github.com/{owner}/{repo}/releases/download/...`

### Scope Assignment

The scope comes from the repository, not the plugin. When a user adds a repository:
1. Shisho fetches the manifest
2. Reads the suggested `scope` field
3. User confirms or edits the scope
4. Scope is stored with the repository

This prevents plugins from impersonating official sources.

### Official Repository

Pre-seeded on first run, cannot be removed:
```
URL: https://raw.githubusercontent.com/shishobooks/plugins/main/repository.json
Scope: shisho
Name: Official Shisho Plugins
```

Displayed with a star icon and "Official" tooltip in UI.

## Storage

### Directory Structure

```
/config/plugins/
├── installed/
│   ├── shisho/                         # Official repo scope
│   │   ├── goodreads-metadata/
│   │   │   ├── manifest.json
│   │   │   └── main.js
│   │   └── kindle-sync/
│   ├── awesome-plugins/                # Third-party repo scope
│   │   └── goodreads-metadata/         # Same ID, different scope = OK
│   └── local/                          # Manual installs
│       └── my-test-plugin/
└── cache/                              # Temp files per plugin
```

### Database Schema

```sql
-- Plugin repositories
CREATE TABLE plugin_repositories (
    url TEXT PRIMARY KEY,
    scope TEXT NOT NULL UNIQUE,
    name TEXT,
    is_official BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_fetched_at TIMESTAMP,
    fetch_error TEXT
);

-- Installed plugins
CREATE TABLE plugins (
    scope TEXT NOT NULL,
    id TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    description TEXT,
    author TEXT,
    homepage TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    installed_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP,
    load_error TEXT,
    update_available_version TEXT,
    PRIMARY KEY (scope, id)
);

-- Plugin configuration values
CREATE TABLE plugin_configs (
    scope TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (scope, plugin_id, key),
    FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
);

-- Plugin-registered identifier types
CREATE TABLE plugin_identifier_types (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    name TEXT NOT NULL,
    url_template TEXT,
    pattern TEXT,
    FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
);

-- Plugin processing order per hook type
CREATE TABLE plugin_order (
    hook_type TEXT NOT NULL,
    scope TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (hook_type, scope, plugin_id),
    FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
);
```

## Plugin Lifecycle

### Hook Execution During Scan

Plugins execute at specific points during the library scan process:

```
1. Discover files in library → initial file list
2. Run inputConverters on discovered files
   ├─ All matching converters run on each file (order = execution sequence, not exclusivity)
   └─ Newly created files added back to scan list for full processing
3. Detect file types (built-in + plugin fileParsers)
4. Parse metadata from detected files
   └─ Files with no parser tracked as supplement (visible, downloadable, no metadata)
5. Run metadataEnrichers on parsed files
6. Save to database
```

### Startup Loading

```
1. App starts
2. Load plugin records from database
3. Scan /config/plugins/installed/ for plugin directories
4. For each enabled plugin:
   a. Read manifest.json
   b. Validate manifest version and minShishoVersion
   c. Create isolated goja runtime context
   d. Execute main.js
   e. Extract exported hooks, validate against declared capabilities
   f. Register hooks with plugin manager
   g. On error: log, store in plugins.load_error, continue
5. Sort hooks by user-defined order
6. Continue normal app startup
```

### Installation Flow

```
1. User clicks "Install" in plugin browser
2. Download plugin zip from GitHub release URL
3. Verify SHA256 checksum
4. Extract to /config/plugins/installed/{scope}/{plugin-id}/
5. Read manifest.json, validate structure
6. Show capabilities and security warning to user
7. User confirms
8. Insert row into plugins table (enabled = false)
9. User enables plugin
10. Hot-reload: plugin loaded without restart
```

### Update Flow

```
1. Daily job fetches all repository manifests
2. Compares installed versions to available versions
3. Sets plugins.update_available_version for outdated plugins
4. UI shows "Update available" badge
5. User clicks update
6. Download new version, replace files
7. Hot-reload: new version loaded without restart
```

### Hot-Reload Mechanics

Plugins are loaded and unloaded without requiring an app restart:

```
1. Create new goja runtime for the plugin
2. Execute new main.js, extract and validate hooks
3. Wait for any in-progress hook calls on old runtime to complete
4. Atomically swap hook references in plugin manager
5. Tear down old runtime
6. Re-register identifier types (diff and update database)
```

This works because plugins are stateless—configuration lives in the database, not in the runtime. In-progress operations (e.g., a scan midway through) complete using the old hooks; subsequent invocations use the new ones.

## API Endpoints

```
# Repositories
GET    /api/plugins/repositories              # List configured repos
POST   /api/plugins/repositories              # Add repo {url, scope}
DELETE /api/plugins/repositories/:scope       # Remove repo (not official)
POST   /api/plugins/repositories/:scope/sync  # Refresh manifest

# Available plugins (from repos)
GET    /api/plugins/available                 # List from all repos
GET    /api/plugins/available/:scope/:id      # Plugin details + versions

# Installed plugins
GET    /api/plugins/installed                 # List installed
POST   /api/plugins/installed                 # Install {scope, id, version?}
DELETE /api/plugins/installed/:scope/:id      # Uninstall
PATCH  /api/plugins/installed/:scope/:id      # Update enabled, config
POST   /api/plugins/installed/:scope/:id/update  # Update to latest

# Plugin ordering
GET    /api/plugins/order/:hookType           # Get order
PUT    /api/plugins/order/:hookType           # Set order [{scope, id}, ...]

# Manual scanning
POST   /api/plugins/scan                      # Scan for manual installs
```

## UI Pages

### Plugin Browser (`/settings/plugins/browse`)
- Search/filter available plugins from all repos
- Shows name, description, author, capabilities, install status
- "Install" button with capabilities confirmation dialog

### Installed Plugins (`/settings/plugins/installed`)
- List with enable/disable toggles
- Load errors displayed with red warning
- "Configure" button for plugin settings
- "Update available" badge
- "Uninstall" button

### Plugin Ordering (`/settings/plugins/order`)
- Tabs per hook type (Input Converters, Metadata Enrichers, File Parsers, Output Generators)
- Drag-and-drop reordering
- Only shows enabled plugins providing that hook

### Repositories (`/settings/plugins/repositories`)
- List with scope, URL, last sync, status
- Official repo marked with star, cannot be removed
- "Add Repository" form with scope confirmation
- Duplicate scope prevention with error message

### Security Warnings

Displayed during installation and in repository management:

```
⚠️ Security Warning

Plugins run code on your server with full access to:
• Network requests (can contact external services)
• File system (can read/write files)
• Your library data (books, metadata, settings)

Only install plugins from sources you trust.
```

## Error Handling

### Load-time Errors

| Error | Display | Action |
|-------|---------|--------|
| Invalid manifest.json | Parse error details | Disable plugin |
| Missing main.js | "Entry point not found" | Disable plugin |
| Runtime init error | Stack trace | Disable plugin |
| Incompatible version | "Requires Shisho v{x}" | Disable plugin |
| Undeclared capability | "Uses {hook} but doesn't declare it" | Disable plugin |

### Runtime Errors

| Error | Behavior |
|-------|----------|
| Converter exception | Log, skip conversion. Original still indexed normally (main if parser exists, supplement if not). |
| Enricher exception | Log, continue to next enricher |
| Parser exception | Log, skip file with error status |
| Generator exception | Log, return 500 to request |
| HTTP timeout | Plugin receives error, handles it |

Errors are logged and stored in `plugins.load_error` for display in UI.

### Isolation and Timeouts

Each plugin runs in its own goja runtime. A crash or panic in one plugin does not affect others or the host application.

Execution timeouts per hook type:

| Hook Type | Timeout |
|-----------|---------|
| inputConverter | 5 minutes |
| outputGenerator | 5 minutes |
| fileParser | 1 minute |
| metadataEnricher | 1 minute |

When a timeout is reached, the plugin receives a context cancellation and should clean up gracefully. The operation is treated as a runtime error (see table above).

## Developer Experience

### Provided by Shisho

1. **TypeScript type definitions** (`@shisho/plugin-types` on npm)
2. **Plugin template repository** (`github.com/shishobooks/plugin-template`)
3. **Documentation** with guides and API reference

### Plugin Template Structure

```
plugin-template/
├── src/
│   └── main.ts
├── manifest.json
├── package.json
├── tsconfig.json
├── build.js
└── README.md
```

### Build Configuration

```javascript
// build.js
require('esbuild').build({
  entryPoints: ['src/main.ts'],
  bundle: true,
  outfile: 'dist/main.js',
  format: 'esm',
  platform: 'neutral',
  target: 'es2020',
});
```

### Local Development

```bash
cd my-plugin/
npm install
npm run build
cp -r dist/main.js manifest.json /config/plugins/installed/local/my-plugin/
# Run POST /api/plugins/scan to detect, then enable in UI (hot-reloads automatically)
```

## Implementation Phases

### Phase 1: Core Infrastructure
- Plugin manager with goja runtime
- Manifest parsing and validation
- Database tables and migrations
- Basic API endpoints (list, install, enable/disable)
- Configuration loading from database
- Hot-reload mechanics (load/unload without restart)

### Phase 2: Hook Implementations
- `inputConverter` integration in scan worker (convert before indexing)
- `metadataEnricher` integration in scan worker
- `fileParser` integration in file detection
- `outputGenerator` integration in download system
- `identifierTypes` registration and storage

### Phase 3: Repository & UI
- Repository fetching and caching
- GitHub URL validation
- Plugin browser UI
- Plugin management UI
- Plugin ordering UI
- Security warning dialogs

### Phase 4: Polish
- Daily update check job
- Error display in UI
- TypeScript type definitions package
- Plugin template repository
- Developer documentation
