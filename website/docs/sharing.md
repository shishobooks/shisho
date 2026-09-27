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
- **Shares Write** lets a user create and manage Share Links.

Both also require access to the book's library. To let other users share books, grant Shares Read and Shares Write to their role under **Settings > Users**. See [Users and Permissions](./users-and-permissions.md).
