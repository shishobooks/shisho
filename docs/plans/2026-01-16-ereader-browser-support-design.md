# eReader Browser Support Design

Support stock eReader browsers (Kobo, Kindle) with a minimal web UI that works around their limitations.

## Background

OPDS works well for readers that support it (e.g., KOReader), but stock eReader browsers can't use OPDS. This feature provides a minimal HTML interface accessible via the eReader's built-in web browser.

**Inspiration:** [opds-proxy](https://github.com/evan-buss/opds-proxy)

**eReader Browser Limitations:**
- No Basic Auth support
- Cookies cleared on browser close (Kobo)
- Double requests on link clicks (Kobo)
- No flexbox/modern CSS
- Minimal JavaScript

## Architecture

### Routing

Unlike regular API endpoints that go through `/api/*` (which strips the prefix before forwarding to the backend), eReader-related routes are served directly:

- `/e/*` - Short URL resolution
- `/ereader/*` - eReader browser UI
- `/opds/*` - OPDS feeds

This allows eReaders to access these endpoints without the `/api` prefix, which simplifies URL entry on limited eReader keyboards. Both `vite.config.ts` (dev) and `Caddyfile` (prod) are configured to proxy these paths directly to the backend.

### New Components

1. **API Keys System** (`pkg/apikeys/`)
   - API key management with permissions
   - Temporary short URL generation for easy setup

2. **eReader Browser UI** (`pkg/ereader/`)
   - HTML handlers mirroring OPDS structure
   - Middleware for API key authentication
   - Auto-detect Kobo via User-Agent for KePub conversion

3. **Security Settings Page** (Frontend)
   - New route `/user/security`
   - Accessible from the User popover
   - API keys management UI
   - Change password (moved from regular settings)

### Authentication Flow

Since eReader browsers can't do Basic Auth, authentication uses API keys embedded in the URL path:

```
/ereader/key/{api_key}/libraries/...
```

**Initial Setup Flow:**
1. User creates API key with "ereader_browser" permission on Security Settings page
2. Clicks "Setup" to generate temporary short URL (30m TTL, lowercase alphanumeric)
3. Types short URL on eReader (e.g., `myserver.com/e/xk9m2p`)
4. Server redirects to full URL with API key
5. User bookmarks the page
6. Short URL expires, but bookmarked URL works indefinitely

Both Kobo and Kindle browsers support saving bookmarks, so users only type the URL once.

## Database Schema

### API Keys Table

```sql
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    last_accessed_at DATETIME
);
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_key ON api_keys(key);
```

### API Key Permissions Table

```sql
CREATE TABLE api_key_permissions (
    id TEXT PRIMARY KEY,
    api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE(api_key_id, permission)
);
CREATE INDEX idx_api_key_permissions_api_key_id ON api_key_permissions(api_key_id);
```

**Initial permission:** `ereader_browser` (more can be added later)

### Temporary Short URLs Table

```sql
CREATE TABLE api_key_short_urls (
    id TEXT PRIMARY KEY,
    api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    short_code TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_short_urls_code ON api_key_short_urls(short_code);
```

**Short code format:** 6 lowercase alphanumeric characters (easy to type on eReader keyboards)

## Backend API

### Package Structure

```
pkg/apikeys/
    model.go       -- ApiKey, ApiKeyPermission, ApiKeyShortUrl structs
    service.go     -- CRUD, permission checks, short URL generation
    handlers.go    -- REST endpoints for managing keys
    routes.go      -- Register routes under /user/api-keys

pkg/ereader/
    handlers.go    -- HTML rendering handlers mirroring OPDS structure
    middleware.go  -- Extract API key from path, validate, check permission
    routes.go      -- Register routes under /ereader and /e (short URLs)
    templates.go   -- Simple HTML templates (inline, no external files)
```

### API Key Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/user/api-keys` | List user's keys |
| POST | `/user/api-keys` | Create key |
| PATCH | `/user/api-keys/:id` | Update name |
| DELETE | `/user/api-keys/:id` | Revoke key |
| POST | `/user/api-keys/:id/permissions/:permission` | Add permission |
| DELETE | `/user/api-keys/:id/permissions/:permission` | Remove permission |
| POST | `/user/api-keys/:id/short-url` | Generate temp short URL (30m TTL) |

### API Response Format

All API key responses should include the full key.

```json
{
  "id": "abc123",
  "name": "Kobo Libra",
  "key": "ak_7kx9mp2nq...",
  "permissions": ["ereader_browser"],
  "created_at": "2025-01-16T...",
  "updated_at": "2025-01-16T...",
  "last_accessed_at": "2025-01-16T..."
}
```

### eReader Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/e/:shortCode` | Resolve short URL, redirect |
| GET | `/ereader/key/:apiKey/` | Root: list libraries |
| GET | `/ereader/key/:apiKey/libraries/:id` | Library nav |
| GET | `/ereader/key/:apiKey/libraries/:id/all` | Paginated book list |
| GET | `/ereader/key/:apiKey/libraries/:id/series` | Series list |
| GET | `/ereader/key/:apiKey/libraries/:id/series/:id` | Books in series |
| GET | `/ereader/key/:apiKey/libraries/:id/authors` | Authors list |
| GET | `/ereader/key/:apiKey/libraries/:id/authors/:name` | Books by author |
| GET | `/ereader/key/:apiKey/libraries/:id/search` | Search form + results |
| GET | `/ereader/key/:apiKey/download/:fileId` | Download file |

## eReader HTML UI

### Design Constraints

- No flexbox/modern CSS (use simple block layout)
- Minimal JavaScript (none required)
- All styling inline (no external CSS files)
- Large tap targets for e-ink
- High contrast (pure black on white)
- Sans-serif font

### Page Template

```html
<!DOCTYPE html>
<html>
<head>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Shisho</title>
  <style>
    body { font-family: sans-serif; margin: 8px; }
    a { color: #000; text-decoration: underline; }
    .item { padding: 12px 0; border-bottom: 1px solid #ccc; }
    .item-title { font-size: 1.1em; font-weight: bold; }
    .item-meta { font-size: 0.9em; color: #666; }
    .nav { margin: 16px 0; }
    .filter { margin-bottom: 12px; }
  </style>
</head>
<body>
  <div class="nav">
    <a href="...">← Back</a> | <a href="...">Home</a>
  </div>

  <h1>Library Name</h1>

  <div class="filter">
    Show: <a href="?types=all">All</a> | <a href="?types=epub">EPUB</a> |
    <a href="?types=cbz">CBZ</a> | <a href="?types=m4b">M4B</a>
    <br>
    Covers: <a href="?covers=off">Off</a> | <a href="?covers=on">On</a>
  </div>

  <div class="item">
    <div class="item-title"><a href="...">Book Title</a></div>
    <div class="item-meta">Author Name • Series #1 • EPUB</div>
  </div>

  <div class="nav">
    <a href="?page=1">← Prev</a> | Page 2 of 5 | <a href="?page=3">Next →</a>
  </div>
</body>
</html>
```

### Features

- **File type filter:** Show/hide EPUB, CBZ, M4B via query params
- **Cover toggle:** Text-only by default, optional thumbnails
- **Simple search:** Basic text input for title/author/series search
- **Pagination:** Link-based navigation

### KePub Auto-Conversion

Detect Kobo devices via User-Agent header. When downloading EPUB or CBZ files from a Kobo browser, automatically convert to KePub format using existing `pkg/kepub/` logic.

## Frontend Implementation

### Files to Create/Modify

| File | Purpose |
|------|---------|
| `app/components/pages/SecuritySettings.tsx` | New security settings page |
| `app/router.tsx` | Add `/user/security` route |
| `app/libraries/api.ts` | Add API key endpoints |
| `app/hooks/queries/apiKeys.ts` | New query hooks |
| Move change password | From existing settings to security page |

### Security Settings Page Layout

```
┌─────────────────────────────────────────────────────────┐
│ Security Settings                                       │
├─────────────────────────────────────────────────────────┤
│                                                         │
│ CHANGE PASSWORD                                         │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ [Current password field]                            │ │
│ │ [New password field]                                │ │
│ │ [Confirm password field]                            │ │
│ │                              [Update Password]      │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│ API KEYS                                                │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ Kobo Libra                        Last used: 2h ago │ │
│ │ ak_7kx9•••••••••         [👁] [📋] [Setup] [Delete] │ │
│ │ ☑ eReader Browser Access                            │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│                              [+ Create New API Key]     │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### UI Interactions

- **Eye icon:** Toggle reveal/hide full key
- **Copy icon:** Copy full key to clipboard, show toast
- **Setup button:** (Only if `ereader_browser` permission) Opens modal with:
  - Generated short URL
  - "Expires in 30 minutes" notice
  - Instructions for eReader setup
- **Delete button:** Confirmation dialog, revokes key
- **Permission checkboxes:** Toggle permissions (API calls on change)
- **Create button:** Modal to enter name, shows full key once

### Cache Invalidation

After API key mutations, invalidate `ListApiKeys` query.

## Implementation Notes

### Reuse Existing Code

- Use existing OPDS service layer for book/series/author data
- Use existing `pkg/kepub/` for KePub conversion
- Use existing `pkg/downloadcache/` for caching converted files

### Security Model

- API keys are long random strings (43 chars), infeasible to guess
- Keys can be revoked instantly
- HTTPS protects keys in transit
- `last_accessed_at` provides audit trail
- Short URLs expire after 30 minutes
- Permissions are granular and can be toggled

### HTML Templates

Store as Go string constants in `pkg/ereader/templates.go` rather than external template files. Keeps deployment simple and templates are small.
