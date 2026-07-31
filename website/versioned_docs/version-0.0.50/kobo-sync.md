# Kobo Sync

Kobo Sync sends books from Shisho to a Kobo eReader over Wi-Fi. Shisho handles personal-library sync requests and proxies other Kobo API requests to Kobo's store service.

## What Syncs

- Only **main EPUB and CBZ files** sync.
- Every compatible main file syncs, so a book with multiple editions can create multiple entries on the device.
- Shisho generates a KePub for each synced EPUB or CBZ.
- The first sync is full. Later syncs are incremental and send additions, removals, and detected metadata or cover changes. Scope changes take effect after the Kobo is configured with the new URL.
- M4B, PDF, and [supplement files](./supplement-files.md) do not sync.

## Setup

### 1. Add the Kobo

Open the user menu and select **Security**. Under **Kobo Wireless Sync**, click **Add Kobo**, enter a device name, and click **Add Kobo**.

### 2. Select a Sync Scope

Click **Setup**, then select one of these scopes:

| Scope | Selection |
|-------|-----------|
| **All Libraries** | Compatible books across the user's accessible libraries |
| **Library** | Compatible books in the selected library, if the user has access |
| **List** | Compatible books that belong to the selected list |

For a list scope, treat the selected list itself as the sync boundary. Do not assume library access settings will further narrow the list's contents. Choose a list containing only the books you intend to send to that device.

:::warning
After selecting **Library** or **List**, actually choose a library or list before copying the URL. The current setup dialog can display an incomplete route while the selector is empty, and that route will not work.
:::

The scope is encoded in the generated **API Endpoint URL**. Changing the selector in Shisho later does not reconfigure the Kobo. Copy the new URL and update `Kobo eReader.conf` whenever you change scope.

### 3. Configure the Kobo

:::warning
The **API Endpoint URL** contains the device's secret Shisho API key. Treat the complete URL like a password. Do not publish it, include it in screenshots or logs, or copy it to a device you do not control.
:::

1. Connect the Kobo to your computer with USB.
2. Open `.kobo/Kobo/Kobo eReader.conf` on the device.
3. Find `api_endpoint=https://storeapi.kobo.com`.
4. Replace the value with the complete **API Endpoint URL** from Shisho.
5. Safely eject the Kobo and start a sync.

If `.kobo` is hidden, enable hidden files in your file manager. On macOS, press `Cmd+Shift+.` in Finder.

## Reset or Remove Sync

Removing the Kobo device under **Security** deletes its API key, immediately revokes the URL, and cannot be undone. Restore Kobo's store endpoint on the device first when you want to stop using Shisho but keep normal Kobo service.

- **Reset:** Open the device's **Setup** dialog and click **Reset**. This clears Shisho's sync history. The next device sync is a fresh sync and resends the current scope.
- **Change scope:** Select the new scope, copy the newly generated URL, and replace `api_endpoint` on the Kobo.
- **Stop using Shisho:** Restore `api_endpoint=https://storeapi.kobo.com` on the device, then remove the Kobo device from Shisho to revoke its key.

## Network Requirements

The Kobo must be able to reach the public or local Shisho URL in `api_endpoint`. Shisho also needs outbound HTTPS access to `https://storeapi.kobo.com` because unhandled store requests are proxied upstream. This outbound connection is an operational requirement and does not change which Shisho books are in scope.

## Troubleshooting

### Sync Fails Immediately

**Symptom:** The Kobo reports a sync error before personal books appear.

**Likely cause:** `api_endpoint` is incomplete, the library or list selector was left empty, or a reverse proxy is not forwarding `/kobo/`.

**Verify:** Compare the complete configured value with the setup dialog. It must include the API key, scope, and any required library or list ID. Confirm that the Kobo can resolve and open the external Shisho hostname.

**Fix:** Select the intended scope value, copy the new URL in full, update `Kobo eReader.conf`, and forward `/kobo/` without removing route segments.

### Books or Metadata Are Stale

**Symptom:** Removed books remain, metadata is old, or the device shows the wrong scope.

**Likely cause:** The Kobo still has an older scope URL or its incremental sync history no longer matches the desired state.

**Verify:** Compare the `api_endpoint` stored on the Kobo with the currently intended scope URL.

**Fix:** Update `api_endpoint` if the scope changed. Otherwise click **Reset** in Shisho, then sync again.

### Kobo Store Features Fail

**Symptom:** Personal books sync, but Kobo store features fail or remain empty.

**Likely cause:** The Shisho server cannot reach Kobo's upstream service.

**Verify:** Check outbound DNS and HTTPS connectivity from the Shisho host to `storeapi.kobo.com`.

**Fix:** Allow that outbound connection without exposing additional inbound Shisho routes. See [Deployment and Maintenance](./deployment-and-maintenance.md) and [Troubleshooting](./troubleshooting.md).

For access questions, see [Users and Permissions](./users-and-permissions.md).
