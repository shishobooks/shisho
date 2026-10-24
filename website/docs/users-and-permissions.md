# Users and Permissions

Shisho combines role permissions with a per-user library access list. Both checks must allow an action.

Open **Administration > Users** to manage accounts and roles. Access to this page and its actions depends on the current user's Users permissions.

## Built-In Roles

Shisho creates three system roles with these default permissions:

| Role | Default Permissions |
|------|---------------------|
| **admin** | Read and Write for Libraries, Books, Series, People, Users, Jobs, Config, and Shares |
| **editor** | Read and Write for Libraries, Books, Series, and People |
| **viewer** | Read for Libraries, Books, Series, and People |

System roles cannot be renamed or deleted, but an administrator can edit their permission selections.

:::caution[Role Changes Affect Every Assigned User]
Changing a role can grant or remove access for every user assigned to it. Review the role's user assignments and the complete permission matrix before saving, especially when changing a built-in role.
:::

Permissions are available as Read and Write operations for these resources:

- Libraries
- Books
- Series
- People
- Users
- Jobs
- Config
- Shares

Write permissions permit the create, edit, or delete operations associated with that resource. Read access does not imply Write access.

Some library data is edited under a broader resource than its name suggests:

- Genres, tags, and publishers are edited with Books Write, along with book and file metadata, covers, chapters, review state, Identify, rescans, merges, file moves, and deletion.
- Series are edited with Series Write.
- People (authors and narrators) are edited with People Write.
- Library rescans create jobs, so they require Jobs Read and Jobs Write. Viewing jobs and their logs requires Jobs Read.
- Bulk downloads also create jobs, but they need Books Read and access to the files' libraries instead of Jobs permissions. See [Bulk Download Permissions](./browsing-search-bulk-actions.md#bulk-download-permissions).

Some reads follow the data they return rather than the page that shows it:

- A list's books need Books Read, like other book data. See [Lists](./lists.md#permissions) for what lists allow without role permissions.
- Global search results follow the permission for each result type. See [Search](./browsing-search-bulk-actions.md#search).
- The review criteria can be read with Books Read, for the **Needs review** badge on book pages, or with Config Read, for **Administration > Review Criteria**. Changing them requires Config Write.
- The plugin order under **Administration > Plugins** needs Books Read as well as Config Read. See [Plugins](./plugins/overview.md#open-the-plugin-manager) for the plugin manager's other permissions.

Share Links are managed with Shares permissions. Granting Shares Write lets a user expose books to people outside your server, so review [Sharing](./sharing.md) before adding it to a role. Shares permissions and library access are all the server checks; they do not depend on Books Read. The **Share** button lives on the book page, though, and opening a book page needs Books Read.

The server log under **Administration > Logs** requires Config Read. Live log updates follow the same rule, so users without Config Read never receive server log lines.

## What a Read-Only Role Sees

A user whose role has Read but not Write for a resource does not see the controls that would change it. The server still rejects any unauthorized request; hiding the controls only keeps the interface honest about what the user can do.

With the built-in **viewer** role, or any role without Books Write, Series Write, or People Write:

- Book tiles in the gallery show no actions menu (rescan, identify, delete). **Add to list** stays available.
- The book page shows no **Edit**, **Identify**, rescan, merge, or delete actions, and a file's actions menu offers only **File details**. The **Needs review** badge still says what is missing but opens no editor. Reading, listening, downloading, and **Add to list** remain.
- The file page shows no **Edit** button or actions menu (rescan, move, delete), and offers no **Edit chapters** or **Fetch from Audible**.
- Series, person, genre, tag, and publisher pages show no **Edit**, **Merge**, or **Delete** buttons.
- Selection mode still works for adding books to lists, and for downloads when the role has the [bulk download permissions](./browsing-search-bulk-actions.md#bulk-download-permissions). **Merge**, **Delete**, and the review actions are hidden.

Each control follows the resource its request needs, not the page it appears on. A role with Books Write but not Series Write sees the edit controls on book, genre, tag, and publisher pages and not on series pages.

Lists use their own sharing roles, so **Add to list** does not need Books Write. See [Lists](./lists.md#permissions).

## What a Role Sees Without Read Permissions

Shisho hides pages and links that a role cannot read, rather than showing an error. Opening such a page by its address shows **Access Denied**.

- **Books Read.** Library pages, book and file pages, and search are unavailable, and the library picker is hidden. The [OPDS Catalog](./opds.md), [Kobo Sync](./kobo-sync.md), and [eReader Browser](./ereader-browser.md) refuse the user too. The home page opens **Lists**, where a list's books are hidden. See [Lists](./lists.md#permissions).
- **Series Read.** The library navigation has no **Series** entry, series names on book pages are plain text, and search shows no series. The edit dialogs suggest no existing series, but you can still type a name.
- **People Read.** The library navigation has no **People** entry, author and narrator names are plain text, and search shows no people. The edit dialogs suggest no existing authors or narrators, but you can still type a name.
- **Libraries Read.** Library settings and **Administration > Libraries** are unavailable. Every signed-in user still gets the name and display settings (cover aspect ratio and whether files are organized) of each library in their [library access](#library-access) list, never its folders. The library picker, breadcrumbs, cover shapes, and the merge and move dialogs use these. A role with Users Write can list libraries without Libraries Read, so it can assign library access when creating or editing a user.
- **Users Read.** **Administration > Users** is unavailable. [List sharing](./lists.md#sharing) still works.
- **Jobs Read.** **Administration > Jobs** is unavailable, and so are the library rescan button and **Recompute now** on **Administration > Review Criteria**, which also need Jobs Write.
- **Config Read.** The server, review criteria, sharing, plugin, cache, and log settings pages are unavailable. With Config Read but not Config Write, the review criteria are shown without a **Save** button.

**Administration** in the user menu opens the first administration page the role can view, and is hidden when there is none. When the role has Books Read, the home page opens the user's default library, which each user can choose under **Library** in **Preferences** once they can access two or more libraries. Without a default, or after losing access to it, the home page opens the first library the user can access. With no libraries yet, it opens **Administration > Libraries** for a role with Libraries Read and **Lists** otherwise.

## Custom Roles

On **Administration > Users**, select **Add Role** to create a named role with a custom permission matrix. Select an existing role in the **Roles** section to edit it. Non-system roles can also be renamed or deleted, but you must reassign every user before deleting an assigned role.

A user has one role. Assign the narrowest permissions needed for that person's work.

## Library Access

Role permissions and library access intersect:

1. The assigned role must grant the required resource operation.
2. The user must also have access to the library containing the requested data.

For example, a user with Books Write but access to only one library can edit books only in that library. Library access does not add permissions that the role lacks.

Removing a user's access to a library also stops every [Share Link](./sharing.md#when-a-link-stops-working) they created for a book in it. The links work again if the access is restored.

When creating or editing a user, choose one of these options under **Library Access**:

- **Access to all libraries** grants access to every current library and automatically includes libraries created in the future.
- Clear **Access to all libraries**, then use **Select Libraries** to grant only the selected current libraries. Future libraries are not added automatically.

## Account Requirements

User accounts follow these requirements:

- Username is required and must contain 3 to 50 characters.
- Password is required and must contain at least 8 characters.
- Email is optional.
- Usernames and email addresses must be unique without regard to letter case.

The initial setup screen creates the first user with the built-in admin role.

## Create and Edit Users

To create an account:

1. Open **Administration > Users**.
2. Select **Add User**.
3. Enter the account information.
4. Optionally select **Require password reset on first login**.
5. Select a role.
6. Configure **Library Access**.
7. Select **Create User**.

Select a username on the **Users** list to edit its username, optional email, role, and library access.

## Password Changes and Forced Resets

Any signed-in user can open the user menu, select **Preferences**, and use **Change Password** under **Password**. A normal self-service change requires the current password.

A user with Users Write permission can open another account under **Administration > Users**, select **Reset Password**, and optionally select **Require user to reset password on next login**. A user marked for forced reset must choose a new password before continuing in Shisho.

## Devices

A Device is one reading app on one piece of hardware, such as Kobo's own reader on a Kobo Libra, KOReader on that same Kobo, or the web browser on a Kindle. [Kobo Sync](./kobo-sync.md), the [eReader Browser](./ereader-browser.md), and the [OPDS Catalog](./opds.md#sign-in-as-a-device) sign in as a Device. Any signed-in user can manage their own Devices from the user menu under **Devices**, with no role permission.

### Device Types

Choose the type when adding a Device. It decides which connections the Device can use, which file types it can receive, and the [Delivery Rules](#delivery-rules-of-a-device) it starts with:

| Type | Connections | Can receive | Starts with |
|------|-------------|-------------|-------------|
| **Kobo** | Kobo sync and the eReader browser | KePub, EPUB | Its preset: KePub as KePub, then EPUB as KePub, then CBZ as KePub, then AZW3 as KePub, then MOBI as KePub |
| **Kindle** | eReader browser | AZW3, MOBI | Its preset: AZW3 as AZW3, then MOBI as MOBI |
| **OPDS app** | OPDS, for reading apps such as KOReader, Thorium, Panels, etc. | Any file type | Your default Delivery Rules |
| **Browser** | eReader browser, for any other e-reader's web browser | Any file type | Your default Delivery Rules |

The type alone decides how a Device connects, and the Device's page shows setup instructions for each of its connections. A Device used through a connection its type does not have, such as a Browser Device's credential in a Kobo sync URL, gets a forbidden response; an OPDS client given a credential from a Device without OPDS gets `401 Unauthorized`.

**Change Type** on a Device's page switches its type and keeps its credential and scope. Its connections change to the new type's, and its Delivery Rules go back to what the new type starts with, so a custom list is discarded.

### Credential

Each Device has one credential that all its connections use. It is 16 lowercase letters with no `l`, `i`, or `o`, so it can be typed on an e-ink keyboard. Shisho accepts it only exactly as shown: no spaces, hyphens, or capital letters. The Device's page always shows the credential, with a copy button.

:::warning[Regenerating a Credential]
**Regenerate** on a Device's page replaces its credential immediately, and cannot be undone. Every Kobo sync URL and eReader browser bookmark with the old credential stops working, and any unexpired setup URL stops redirecting. Set up each connection again with the new credential. Regenerate when a credential may have leaked; delete the Device when the hardware is lost for good.
:::

Treat the credential, and every URL that contains it, like a password.

### Scope

A Device's scope decides which books it reaches, through every connection:

| Scope | Books |
|-------|-------|
| **All Books** | Books in every library the Device's owner can access |
| **Library** | Books in one library the owner can access |
| **List** | Books on one list the owner owns or that is shared with them, in libraries the owner can access |

A list shared with you can hold books from libraries you cannot access; those books stay out of the scope. If the scope's library or list is deleted, or the list stops being shared with you, the Device reaches no books until you choose another scope. A scope change applies on the Device's next request, without changing anything on the device itself.

### Delivery Rules of a Device

Each Device has its own [Delivery Rules](./supported-formats.md#delivery-rules), which decide what its eReader browser and OPDS catalog offer and what [Kobo Sync](./kobo-sync.md#what-syncs) sends. Under **Delivery Rules** on its page, choose one:

- **My default**: your default rules from **Preferences > Delivery Rules**, without any rule that delivers a file type the Device can't receive. A later edit to your default reaches the Device too. A Kindle following a default that has no AZW3 or MOBI rule receives nothing, and its page says so.
- **Kobo preset** or **Kindle preset**: the type's preset from the table above, for Kobo and Kindle Devices. A Shisho release that changes a preset changes it for every Device following it.
- **Custom**: the Device's own list, which starts as a copy of what it followed and is saved with **Save**. It needs at least one rule, and a rule can only deliver a file type the Device can receive, so a Kindle can't have a rule that sends EPUB. Choosing **My default** or the preset again discards the list. While a Device follows your default, **Edit your default** under its rules opens **Preferences > Delivery Rules**.

Under the rules, the Device's page previews how many books in its scope the Device receives, and how many it receives nothing of, counted by the file types those books have. A book with files of two types counts under both. The preview updates while you edit a custom list. For a Kobo, the preview also says how many books on it the rules would move to a different edition, and Shisho asks before saving a change that switches books on it between KePub and EPUB; see [Kobo Sync](./kobo-sync.md#moving-to-a-different-edition).

The **Devices** tab on each book's page has a row for each of your Devices: the file type each of the book's files goes out as, such as EPUB to KePub, or why it receives nothing, such as no rule for the book's file types or the book being outside its scope. For a Kobo, the row also warns when its next sync would move the book to a different edition or switch it between KePub and EPUB.

### Delete a Device

Deleting a Device under **Devices** cannot be undone. Every connection stops working immediately, and Shisho forgets what it has synced to the Device. To use the hardware again, add a new Device and set it up again.

### Manage Every User's Devices

A user with Users Read can open **Administration > Devices** to see every user's Devices: their names, types, connections, owners, and when each was last used. The list never shows credentials, so an administrator cannot sign in as another user's Device. A user with Users Write can **Revoke** a Device, which deletes it as above, for example to cut off a lost device for someone.

## Deactivate Users

:::warning[Verify the Account Before Deactivation]
Deactivation immediately prevents that user from logging in. An [OPDS](./opds.md) client that signed in during the last minute may keep working for up to a minute, because OPDS remembers a successful sign-in for that long. It does not delete the account, but the web interface has no control to reactivate it. You cannot deactivate your own account.
:::

A user with Users Write permission can select another active account and choose **Deactivate User**. The account and its historical records remain stored.

Deactivation also stops every [Share Link](./sharing.md#when-a-link-stops-working) the user created. The links stay listed in each book's Share dialog, where anyone with Shares Write can revoke or delete them. It disables the user's [Devices](#devices) too, so their [Kobo Sync](./kobo-sync.md#how-long-the-url-works) and [eReader Browser](./ereader-browser.md#how-long-the-url-works) URLs stop working.

## Sessions

Shisho uses one server-wide session duration with a fixed default of 30 days. There is no per-user duration or remember-me setting. Administrators can change the global `SESSION_DURATION_DAYS` setting; existing tokens remain governed by how they were issued. See [Configuration](./configuration.md#authentication).

The [OPDS Catalog](./opds.md) also supports HTTP Basic Auth for clients that do not use Shisho's browser session. OPDS requires Books Read, and catalog contents follow the user's library access.
