# Dynamic Plugin Install Capabilities Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the plugin install dialog show only the capabilities a plugin actually declares, with structured details like approved domains and allowed commands.

**Architecture:** Add a `Capabilities` pointer to `PluginVersion` in the Go repository struct (reusing the existing manifest type). Flow it through the API to the frontend, where `CapabilitiesWarning.tsx` dynamically renders only present capabilities. Remove the hardcoded rows, "Manifest v1" badge, and "Sandboxed Execution" row.

**Tech Stack:** Go (backend struct), React/TypeScript (frontend component), Docusaurus (docs)

---

### Task 1: Add Capabilities to Go PluginVersion struct

**Files:**
- Modify: `pkg/plugins/repository.go:40-48`

- [ ] **Step 1: Write the failing test**

Add a test to `pkg/plugins/repository_test.go` that verifies capabilities are parsed from a repository manifest:

```go
func TestFetchRepository_WithCapabilities(t *testing.T) {
	manifest := RepositoryManifest{
		RepositoryVersion: 1,
		Scope:             "test",
		Name:              "Test Plugins",
		Plugins: []AvailablePlugin{
			{
				ID:   "enricher",
				Name: "Enricher",
				Versions: []PluginVersion{
					{
						Version:         "1.0.0",
						ManifestVersion: 1,
						DownloadURL:     "https://github.com/test/releases/download/v1.0.0/plugin.zip",
						SHA256:          "abc123",
						Capabilities: &Capabilities{
							MetadataEnricher: &MetadataEnricherCap{
								FileTypes: []string{"epub", "m4b"},
								Fields:    []string{"title", "authors"},
							},
							HTTPAccess: &HTTPAccessCap{
								Domains: []string{"*.example.com"},
							},
						},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	origHosts := AllowedFetchHosts
	AllowedFetchHosts = []string{server.URL}
	defer func() { AllowedFetchHosts = origHosts }()

	result, err := FetchRepository(server.URL + "/plugins.json")
	require.NoError(t, err)
	require.Len(t, result.Plugins, 1)
	require.Len(t, result.Plugins[0].Versions, 1)

	caps := result.Plugins[0].Versions[0].Capabilities
	require.NotNil(t, caps)
	require.NotNil(t, caps.MetadataEnricher)
	assert.Equal(t, []string{"epub", "m4b"}, caps.MetadataEnricher.FileTypes)
	assert.Equal(t, []string{"title", "authors"}, caps.MetadataEnricher.Fields)
	require.NotNil(t, caps.HTTPAccess)
	assert.Equal(t, []string{"*.example.com"}, caps.HTTPAccess.Domains)
	assert.Nil(t, caps.FFmpegAccess)
	assert.Nil(t, caps.ShellAccess)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-install && go test ./pkg/plugins/ -run TestFetchRepository_WithCapabilities -v`
Expected: FAIL — `Capabilities` field does not exist on `PluginVersion`

- [ ] **Step 3: Add Capabilities field to PluginVersion**

In `pkg/plugins/repository.go`, add the `Capabilities` field to the `PluginVersion` struct:

```go
// PluginVersion describes a specific version of an available plugin.
type PluginVersion struct {
	Version          string        `json:"version"`
	MinShishoVersion string        `json:"minShishoVersion"`
	ManifestVersion  int           `json:"manifestVersion"`
	ReleaseDate      string        `json:"releaseDate"`
	Changelog        string        `json:"changelog"`
	DownloadURL      string        `json:"downloadUrl"`
	SHA256           string        `json:"sha256"`
	Capabilities     *Capabilities `json:"capabilities,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-install && go test ./pkg/plugins/ -run TestFetchRepository_WithCapabilities -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-install && go test ./pkg/plugins/ -v`
Expected: All tests pass (existing tests unaffected since `Capabilities` is a pointer with `omitempty`)

- [ ] **Step 6: Commit**

```bash
git add pkg/plugins/repository.go pkg/plugins/repository_test.go
git commit -m "[Backend] Add capabilities field to repository PluginVersion struct"
```

---

### Task 2: Add capabilities types to frontend PluginVersion

**Files:**
- Modify: `app/hooks/queries/plugins.ts`

- [ ] **Step 1: Add capability type interfaces**

Add these interfaces above the existing `PluginVersion` interface in `app/hooks/queries/plugins.ts`:

```typescript
export interface PluginCapabilities {
  metadataEnricher?: { fileTypes?: string[]; fields?: string[] };
  inputConverter?: { sourceTypes?: string[]; targetType?: string };
  fileParser?: { types?: string[] };
  outputGenerator?: { sourceTypes?: string[]; name?: string };
  httpAccess?: { domains?: string[] };
  fileAccess?: { level?: string };
  ffmpegAccess?: Record<string, never>;
  shellAccess?: { commands?: string[] };
}
```

- [ ] **Step 2: Add capabilities field to PluginVersion**

Add `capabilities?: PluginCapabilities;` to the `PluginVersion` interface, after `releaseDate`:

```typescript
export interface PluginVersion {
  version: string;
  minShishoVersion: string;
  compatible: boolean;
  changelog: string;
  downloadUrl: string;
  sha256: string;
  manifestVersion: number;
  releaseDate: string;
  capabilities?: PluginCapabilities;
}
```

- [ ] **Step 3: Run type check**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-install && pnpm lint:types`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add app/hooks/queries/plugins.ts
git commit -m "[Frontend] Add capabilities types to PluginVersion interface"
```

---

### Task 3: Rewrite CapabilitiesWarning to render dynamic capabilities

**Files:**
- Modify: `app/components/plugins/CapabilitiesWarning.tsx`

- [ ] **Step 1: Replace the component with dynamic rendering**

Rewrite `app/components/plugins/CapabilitiesWarning.tsx` with the following content. This replaces the hardcoded rows with a data-driven approach:

```tsx
import {
  ArrowRightLeft,
  FileOutput,
  FileSearch,
  FolderOpen,
  Globe,
  Search,
  Terminal,
  Video,
  type LucideIcon,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type {
  AvailablePlugin,
  PluginCapabilities,
} from "@/hooks/queries/plugins";

interface CapabilitiesWarningProps {
  isPending: boolean;
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  plugin: AvailablePlugin | null;
}

interface CapabilityRowProps {
  description: string;
  detail?: string;
  icon: LucideIcon;
  label: string;
}

const CapabilityRow = ({
  description,
  detail,
  icon: Icon,
  label,
}: CapabilityRowProps) => (
  <div className="flex items-start gap-3 rounded-md border border-border p-3">
    <Icon className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
    <div>
      <p className="text-sm font-medium">{label}</p>
      <p className="text-xs text-muted-foreground">{description}</p>
      {detail && (
        <p className="mt-1 font-mono text-xs text-muted-foreground/70">
          {detail}
        </p>
      )}
    </div>
  </div>
);

interface CapabilityDef {
  key: keyof PluginCapabilities;
  icon: LucideIcon;
  label: string;
  description: string;
  detail: (cap: PluginCapabilities) => string | undefined;
}

const CAPABILITY_DEFS: CapabilityDef[] = [
  {
    key: "metadataEnricher",
    icon: Search,
    label: "Metadata Enrichment",
    description: "Searches external sources for book metadata",
    detail: (cap) =>
      cap.metadataEnricher?.fileTypes?.length
        ? cap.metadataEnricher.fileTypes.join(", ")
        : undefined,
  },
  {
    key: "inputConverter",
    icon: ArrowRightLeft,
    label: "Format Conversion",
    description: "Converts files between formats",
    detail: (cap) =>
      cap.inputConverter?.sourceTypes?.length && cap.inputConverter?.targetType
        ? `${cap.inputConverter.sourceTypes.join(", ")} \u2192 ${cap.inputConverter.targetType}`
        : undefined,
  },
  {
    key: "fileParser",
    icon: FileSearch,
    label: "File Parsing",
    description: "Extracts metadata from files",
    detail: (cap) =>
      cap.fileParser?.types?.length
        ? cap.fileParser.types.join(", ")
        : undefined,
  },
  {
    key: "outputGenerator",
    icon: FileOutput,
    label: "Output Generation",
    description: "Generates files in additional formats",
    detail: (cap) =>
      cap.outputGenerator?.sourceTypes?.length && cap.outputGenerator?.name
        ? `${cap.outputGenerator.sourceTypes.join(", ")} \u2192 ${cap.outputGenerator.name}`
        : undefined,
  },
  {
    key: "httpAccess",
    icon: Globe,
    label: "Network Access",
    description: "May make network requests to external services",
    detail: (cap) =>
      cap.httpAccess?.domains?.length
        ? cap.httpAccess.domains.join(", ")
        : undefined,
  },
  {
    key: "fileAccess",
    icon: FolderOpen,
    label: "File System Access",
    description: "Can access files beyond its sandboxed plugin directory",
    detail: (cap) =>
      cap.fileAccess?.level === "readwrite" ? "read/write" : undefined,
  },
  {
    key: "ffmpegAccess",
    icon: Video,
    label: "FFmpeg Execution",
    description: "May invoke FFmpeg for media processing",
    detail: () => undefined,
  },
  {
    key: "shellAccess",
    icon: Terminal,
    label: "Shell Command Execution",
    description: "May execute shell commands on your system",
    detail: (cap) =>
      cap.shellAccess?.commands?.length
        ? cap.shellAccess.commands.join(", ")
        : undefined,
  },
];

function getCapabilityRows(capabilities: PluginCapabilities | undefined) {
  if (!capabilities) return [];
  return CAPABILITY_DEFS.filter((def) => capabilities[def.key] != null);
}

export const CapabilitiesWarning = ({
  isPending,
  onConfirm,
  onOpenChange,
  open,
  plugin,
}: CapabilitiesWarningProps) => {
  if (!plugin) return null;

  const latestVersion =
    plugin.versions.find((v) => v.compatible) ?? plugin.versions[0];

  const rows = getCapabilityRows(latestVersion?.capabilities);

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Install {plugin.name}?</DialogTitle>
          <DialogDescription>
            {rows.length > 0
              ? "This plugin will be granted the following capabilities on your system. Review them before proceeding."
              : "This plugin does not declare any specific capabilities."}
          </DialogDescription>
        </DialogHeader>

        {rows.length > 0 && (
          <div className="space-y-2">
            {rows.map((def) => (
              <CapabilityRow
                description={def.description}
                detail={
                  latestVersion?.capabilities
                    ? def.detail(latestVersion.capabilities)
                    : undefined
                }
                icon={def.icon}
                key={def.key}
                label={def.label}
              />
            ))}
          </div>
        )}

        {latestVersion && (
          <div className="text-xs text-muted-foreground">
            Version: {latestVersion.version}
          </div>
        )}

        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button disabled={isPending} onClick={onConfirm}>
            {isPending ? "Installing..." : "Install Plugin"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
```

- [ ] **Step 2: Run type check**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-install && pnpm lint:types`
Expected: No type errors

- [ ] **Step 3: Run lint**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-install && pnpm lint:eslint`
Expected: No lint errors

- [ ] **Step 4: Commit**

```bash
git add app/components/plugins/CapabilitiesWarning.tsx
git commit -m "[Frontend] Render dynamic capabilities in plugin install dialog"
```

---

### Task 4: Update documentation

**Files:**
- Modify: `website/docs/plugins/repositories.md`

- [ ] **Step 1: Update the repository manifest format docs**

In `website/docs/plugins/repositories.md`, update the version description list (around line 33) to include capabilities. Replace the existing list:

```markdown
Each plugin version in a repository includes:

- A **download URL** pointing to a ZIP file on GitHub Releases
- A **SHA256 hash** for verifying the download integrity
- A **minimum Shisho version** for compatibility filtering
- A **changelog** describing what changed
```

With:

```markdown
Each plugin version in a repository includes:

- A **download URL** pointing to a ZIP file on GitHub Releases
- A **SHA256 hash** for verifying the download integrity
- A **minimum Shisho version** for compatibility filtering
- A **changelog** describing what changed
- An optional **capabilities** object declaring what the plugin can do (shown during install)
```

- [ ] **Step 2: Update the JSON example**

Replace the JSON example in the "Repository Manifest Format" section (around line 49) to include capabilities:

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

- [ ] **Step 3: Add a capabilities reference section**

Add a new section after "Key Rules" (after line 83) explaining the capabilities field:

```markdown
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
```

- [ ] **Step 4: Commit**

```bash
git add website/docs/plugins/repositories.md
git commit -m "[Docs] Document capabilities field in repository manifest format"
```

---

### Task 5: Run full validation

- [ ] **Step 1: Run mise check:quiet**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-install && mise check:quiet`
Expected: All checks pass

- [ ] **Step 2: Fix any issues found**

If any checks fail, fix the issues and re-run.
