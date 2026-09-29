# Sharing

A Share Link lets someone open one book and download its files without a Shisho account and without access to the book's library. It is different from sharing a list, which grants a list to other users who sign in. See [Lists](./lists.md#sharing) for list sharing.

## Turning On Share Links

Share Links are off on a new server. They are controlled from **Settings > Sharing**, not from the [configuration file](./configuration.md). Each switch saves as soon as you flip it and confirms the new state with a short message. Viewing the page requires Config Read, and changing its switches requires Config Write.

:::caution[Share Links Expose Books Outside Your Server]
Anyone who holds a Share Link can open the shared book and download its files, whether or not they have an account. The link only works if the recipient can reach this server, so a server that is only available on your local network or through a VPN cannot be reached by recipients outside it. Decide which roles may create links, and whether links must expire, before you turn sharing on.
:::

- **Enable Share Links** allows users with the Shares permission to create links. Turning it off stops every existing link from working but deletes none of them, so turning it back on restores them. Links can still be revoked and deleted while it is off, so you never have to turn sharing back on just to pull one link.
- **Require expiration** makes every new link expire. When it is off, users can also create links that never expire.

## Who Can Share

Share Links are managed with the Shares permission, which only the **admin** role has by default:

- **Shares Read** lets a user see a book's Share Links and read the sharing settings.
- **Shares Write** lets a user create, revoke, and delete Share Links, and see a book's links and the sharing settings as well.

Both also require access to the book's library. To let other users share books, grant Shares Read and Shares Write to their role under **Settings > Users**. See [Users and Permissions](./users-and-permissions.md).

## Creating a Link

Users with Shares Read or Shares Write see a **Share** entry in a book's action menu (the **⋮** button next to the title). It appears even for users who cannot edit the book. Choosing it opens the Share dialog.

With sharing turned on, users with Shares Write see a form at the top of the dialog:

- **Label** is optional and only visible to you and other sharers. Use it to tell links apart, for example "for Alice".
- **Expires after** offers 1 day, 7 days (the default), and 30 days. **Never** is offered last unless the admin has turned on **Require expiration**. An expiration is fixed when the link is created and cannot be extended later; create a new link instead.

**Create link** adds the link to the list below the form. The list shows every link on the book, including links other users created, with its label, who created it, whether it is active, paused, expired, or revoked, when it expires (or when it was revoked), and how it has been used (see [Managing Links](#managing-links)). The copy button next to a working link puts its URL on your clipboard, ready to paste into a message. Users with only Shares Read see the list without the form.

The copied URL uses the address you are browsing Shisho on. If you reach Shisho through a local address such as `http://192.168.1.10:5173`, the link will only work for people on your network. Open Shisho through the address your recipient can reach before copying.

## Managing Links

Each link in the Share dialog shows three usage figures:

- **Opens** counts how many times the recipient page was loaded. Reloading the page counts again.
- **Downloads** counts file downloads started through the link. Resuming an interrupted download does not count again. A download shows the recipient fetched a file; an open only shows they looked.
- **Last used** is the time of the most recent open or download, or "Never used".

Viewing the book's cover does not count as either. The dialog fetches fresh figures each time you open it.

Users with Shares Write can act on any link on the book, including links other users created:

- **Revoke** (the ⊘ button, offered on active and paused links) stops the link immediately. The link stays in the list, marked as revoked with the time it was revoked, and keeps its figures, so you can still see whether it was used before you pulled it. Revoking cannot be undone. To share the book again, create a new link.
- **Delete** (the trash button, offered on every link) removes the link and its figures from the list. Deleting an active link also stops it.

Expired links stay in the list, marked as expired, until someone deletes them. Shisho never removes them on its own.

While sharing is turned off, the dialog says so instead of showing the form (with a link to **Settings > Sharing** for users who can change it), and no link can be copied, but links can still be revoked and deleted. A link revoked or deleted then stays that way when sharing is turned back on.

## When a Link Stops Working

A link works only while all of these hold:

- Sharing is turned on in **Settings > Sharing**.
- The link has not expired and has not been revoked or deleted.
- The user who created it is active and still has access to the book's library.
- The book still exists.

The checks run each time the link is used, so some of them can be reversed:

| What happened | Effect on the link |
| --- | --- |
| The link expires | Stops working; stays listed as expired |
| Someone revokes the link | Stops working; stays listed as revoked |
| Someone deletes the link | Stops working; removed from the list |
| An admin turns sharing off | Stops working; works again when sharing is turned back on |
| The creator is deactivated | Stops working and shows as paused; works again only if the account is reactivated. See [Deactivate Users](./users-and-permissions.md#deactivate-users) |
| The creator loses access to the book's library | Stops working and shows as paused; works again if their access is restored |
| The book is deleted | Stops working; removed |

Changing the creator's role, including removing Shares Write from it, does not stop the links they already created.

The Share dialog marks a link from a deactivated creator, or from one who lost access to the library, as **paused** and says which of the two happened. A paused link cannot be copied. Revoke or delete it if it should not come back when the creator's access does.

## What Recipients See

A recipient opens the link in any browser, on a phone or a computer, without signing in. The page shows:

- The Shisho logo and a notice saying who shared the book and when the link expires, if it does.
- The book's cover, title, subtitle, authors, series, description, genres, and tags.
- Each of the book's files with a download button, including supplements. A file's details show its publisher, release date, language, and, for audiobooks, whether it is abridged.

Downloads are in the file's own format (EPUB, CBZ, PDF, or M4B) with the book's metadata written into it, whatever download format the library prefers. Supplements download as they are. If a file cannot have metadata written into it, for example a format only a plugin can read, the recipient gets the original file. Recipients cannot choose the original or KePub versions, read or listen in the browser, or follow links into the rest of your server. Names such as authors and series are plain text.

The page never shows file paths or other details about how your server is laid out. It also leaves out what only matters inside your library: the sort title, when the book was added and last updated, file identifiers, and file URLs. The downloaded file itself still contains the book's full metadata, including identifiers such as the ISBN.

If a link has stopped working for any of the reasons in [When a Link Stops Working](#when-a-link-stops-working), or was mistyped, the recipient sees "This link is no longer available". The page does not say which, so a recipient cannot tell a revoked or expired link from one that never existed.

Share Links are not available on the [public demo](./demo.md).
