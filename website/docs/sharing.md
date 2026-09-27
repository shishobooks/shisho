# Sharing

A Share Link lets someone open one book and download its files without a Shisho account and without access to the book's library. It is different from sharing a list, which grants a list to other users who sign in. See [Lists](./lists.md#sharing) for list sharing.

## Turning On Share Links

Share Links are off on a new server. They are controlled from **Settings > Sharing**, not from the [configuration file](./configuration.md), and changes apply as soon as you switch them. Viewing the page requires Config Read, and changing its switches requires Config Write.

:::caution[Share Links Expose Books Outside Your Server]
Anyone who holds a Share Link can open the shared book and download its files, whether or not they have an account. The link only works if the recipient can reach this server, so a server that is only available on your local network or through a VPN cannot be reached by recipients outside it. Decide which roles may create links, and whether links must expire, before you turn sharing on.
:::

- **Enable Share Links** allows users with the Shares permission to create links. Turning it off stops every existing link from working but deletes none of them, so turning it back on restores them.
- **Require expiration** makes every new link expire. When it is off, users can also create links that never expire.

## Who Can Share

Share Links are managed with the Shares permission, which only the **admin** role has by default:

- **Shares Read** lets a user see a book's Share Links and read the sharing settings.
- **Shares Write** lets a user create and manage Share Links, and see a book's links and the sharing settings as well.

Both also require access to the book's library. To let other users share books, grant Shares Read and Shares Write to their role under **Settings > Users**. See [Users and Permissions](./users-and-permissions.md).

## Creating a Link

With sharing turned on, users with Shares Read or Shares Write see a **Share** entry in a book's action menu (the **⋮** button next to the title). It appears even for users who cannot edit the book. Choosing it opens the Share dialog.

Users with Shares Write see a form at the top of the dialog:

- **Label** is optional and only visible to you and other sharers. Use it to tell links apart, for example "for Alice".
- **Expires after** offers 1 day, 7 days (the default), and 30 days. **Never** is offered last unless the admin has turned on **Require expiration**. An expiration is fixed when the link is created and cannot be extended later; create a new link instead.

**Create link** adds the link to the list below the form. The list shows every link on the book, including links other users created, with its label, who created it, whether it is active or expired, and when it expires. The copy button next to an active link puts its URL on your clipboard, ready to paste into a message. Users with only Shares Read see the list without the form.

The copied URL uses the address you are browsing Shisho on. If you reach Shisho through a local address such as `http://192.168.1.10:5173`, the link will only work for people on your network. Open Shisho through the address your recipient can reach before copying.

Expired links stay in the list, marked as expired.

## What Recipients See

A recipient opens the link in any browser, on a phone or a computer, without signing in. The page shows:

- The Shisho logo, who shared the book, and when the link expires, if it does.
- The book's cover, title, subtitle, authors, series, description, genres, and tags.
- Each of the book's files with a download button, including supplements.

Downloads are in the file's own format (EPUB, CBZ, PDF, or M4B) with the book's metadata written into it, whatever download format the library prefers. Supplements download as they are. If a file cannot have metadata written into it, for example a format only a plugin can read, the recipient gets the original file. Recipients cannot choose the original or KePub versions, read or listen in the browser, or follow links into the rest of your server. Names such as authors and series are plain text.

The page never shows file paths or other details about how your server is laid out.

If a link has expired, was mistyped, or sharing has been turned off, the recipient sees "This link is no longer available". The page does not say which, so a recipient cannot tell an expired link from one that never existed.

Share Links are not available on the [public demo](./demo.md).
