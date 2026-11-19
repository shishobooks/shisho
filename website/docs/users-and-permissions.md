# Users and Permissions

Shisho combines role permissions with a per-user library access list. Both checks must allow an action.

Open **Administration > Users** to manage accounts on its **Users** tab and roles on its **Roles** tab. Access to this page and its actions depends on the current user's Users permissions.

## Built-In Roles

Shisho creates three system roles with these default permissions:

| Role | Default Permissions |
|------|---------------------|
| **admin** | Read and Write for Libraries, Books, Series, People, Users, Jobs, Config, and Shares |
| **editor** | Read and Write for Libraries, Books, Series, and People |
| **viewer** | Read for Books, Series, and People |

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

- Genres, tags, and publishers are edited with Books Write, along with book and file metadata, covers, chapters, review status, Identify, rescans, merges, file moves, and deletion.
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
- **Libraries Read.** Library settings and **Administration > Libraries** are unavailable. Every signed-in user still gets the name and display settings (cover aspect ratio, whether files are organized, and [tag namespaces](./metadata.md#tag-namespaces)) of each library in their [library access](#library-access) list, never its folders. The library picker, breadcrumbs, cover shapes, and the merge and move dialogs use these. A role with Users Write can list libraries without Libraries Read, so it can assign library access when creating or editing a user.
- **Users Read.** **Administration > Users** is unavailable. [List sharing](./lists.md#sharing) still works.
- **Jobs Read.** **Administration > Jobs** is unavailable, and so are the library rescan button and **Recompute now** on **Administration > Review Criteria**, which also need Jobs Write.
- **Config Read.** The server settings, Server Stats, review criteria, sharing, plugin, cache, and log pages are unavailable, and the sidebar never says when a newer Shisho release is out. With Config Read but not Config Write, the review criteria are shown without a **Save** button.

**Administration** in the user menu opens the first administration page the role can view, and is hidden when there is none. When the role has Books Read, the home page opens the user's default library, which each user can choose under **Library** in **Preferences** once they can access two or more libraries. Without a default, or after losing access to it, the home page opens the first library the user can access. With no libraries it can open, it goes to **Administration > Libraries** for a user with Libraries Read and access to every library. Anyone else sees a page saying they don't have access to any libraries yet and should ask their server admin for it; **Lists** stays in the user menu. Give the user [library access](#library-access) and the home page opens that library on their next visit.

## Custom Roles

On the **Roles** tab of **Administration > Users**, select **Add Role** to create a named role with a custom permission matrix. Select an existing role there to edit it. Non-system roles can also be renamed or deleted, but you must reassign every user before deleting an assigned role.

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

- Username is required and must contain 3 to 50 characters. It can hold letters and numbers in any language, plus `.`, `_`, `-`, and `@`, but no spaces or other symbols, so it works for signing in to [OPDS](./opds.md) and [KOReader](./koreader.md) clients. An existing username that doesn't follow these rules still signs in, and has to follow them only when it's changed.
- Password is required and must contain at least 8 characters.
- Email is optional.
- Usernames and email addresses must be unique without regard to letter case.

The initial setup screen creates the first user with the built-in admin role, then continues into [Setup](./getting-started.md#set-up-shisho). That admin starts with the [Getting Started](#getting-started) pill in the top nav.

## Add Users

Open **Administration > Users** and select **Add User**. It offers two ways to add someone:

- **Invite link**, the default: you choose the role and library access, and Shisho gives you a link. The person opens it and chooses their own username, password, and optional email. See [Invite Links](#invite-links).
- **Set a password**: you create the account yourself, including its password, and pass the details on.

Both forms warn when **Access to all libraries** is cleared and no library is selected. Such a user can sign in but sees no books until you give them access to a library.

### Invite Links

An Invite Link adds one user. To create one:

1. Under **Add User**, keep **Invite link** selected.
2. Select a role and configure **Library Access**.
3. Choose how long the link works under **Link expires after**: 1, 7, or 30 days. The default is 7 days.

:::warning[Anyone With the Link Can Create the Account]
Until it's used, an invite link works like a password: whoever opens it first gets an account with the role and library access you chose. Send it only to the person it's for.
:::

4. Select **Create invite link**, then copy the link.

Shisho never sends the link. Pass it on however you like, such as by text or chat. The link uses the address you have Shisho open at, so create it from an address the other person can reach, not `localhost`. At such an address, the link and the pending invites on the Users page carry a warning.

Opening the link shows a welcome page naming who sent it. The person chooses a username, a password, and optionally an email address. Accepting creates the account with the invite's role and library access, signs them in, and opens their [Getting Started](#getting-started) panel.

Pending invites are listed at the top of the **Users** tab, marked **Pending**, with their role, library access, and expiry. Select the copy button on one to copy its link again, or the revoke button to stop it.

An invite works once. After that, and once it expires or is revoked, the link shows **This invite link is no longer valid** and asks the person to request a new one, without saying which happened. An invite also stops working when the user who created it is deleted, deactivated, or loses Users Write. Library access in a pending invite follows the libraries: deleting a library removes it from the invite.

Someone who opens an invite while already signed in to Shisho is asked to sign out first, so accepting never switches accounts by accident.

### Set a Password

1. Under **Add User**, select **Set a password**.
2. Enter the account information. **Require password reset on first login** is selected by default, so the password you choose is temporary. Clear it to let them keep that password.
3. Select a role and configure **Library Access**.
4. Select **Create User**.

Shisho then shows **Send these to them**: the address you're using Shisho at and the new username, each with a copy button. Give them those and the password you set. If you reach Shisho at an address other devices can't use, such as `localhost`, send the address they should use instead.

The first time the new user signs in, after choosing a new password if a reset was required, their [Getting Started](#getting-started) panel opens on its own.

## Edit Users

Select a username on the **Users** list to edit its username, optional email, role, and library access.

## Password Changes and Forced Resets

Any signed-in user can open the user menu, select **Preferences**, and use **Change Password** under **Password**. A normal self-service change requires the current password.

A user with Users Write permission can open another account under **Administration > Users**, select **Reset Password**, and optionally select **Require user to reset password on next login**. A user marked for forced reset must choose a new password before continuing in Shisho.

## Getting Started

Getting Started is each user's short list of first steps, behind the **Getting Started** pill in the top nav. The ring in the pill shows how many are done. Each step links to where it gets done and ticks itself off once what it asks for exists, however it was done. A user sees only the steps their role allows:

| Step | Shown to roles with | Done when |
|------|---------------------|-----------|
| **Set up your first library** | Libraries Write | Any library exists |
| **Add metadata plugins** | Config Write, once a library exists | An installed plugin provides a metadata enricher |
| **Invite someone** | Users Write | Another active user exists or an [Invite Link](#invite-links) is pending |
| **Connect a reading device** | Books Read | The user has a [Device](#devices) |
| **Add a book to To Read** | Books Read, once they have access to a library | The user's To Read holds a book, or held one they then started |
| **Start reading something** | Books Read, once they have access to a library | The user has a place saved in any book, or any [reading history](./reading-tracking.md) |

- **Not for me** on a step takes it out of the count, and **Undo** puts it back.
- **Dismiss this list** hides the pill without asking. **Getting Started** in the user menu brings it back. A hidden list also comes back on its own when a step that didn't apply when it was hidden starts to apply, such as **Add a book to To Read** once the user has access to a library, and stays until it's hidden again.
- When every step is done or marked **Not for me**, the panel shows **You're all set**, and closing it hides the list the same way.
- **Set up your first library** opens [Setup](./getting-started.md#set-up-shisho) at its library step, so metadata plugins are chosen before the first scan.
- The panel opens on its own the first time a user added with **Add User** signs in, whether through an invite or a password you set. The first admin gets the pill, but the panel stays closed.
- A user it isn't on for can turn it on with **Getting Started** in the user menu.
- Getting Started never appears in [Demo Mode](./configuration.md#demo-mode).

### Show Getting Started

A user with Users Write can open an account under **Administration > Users** and select **Show Getting Started**. It brings the list back for that user, clears their **Not for me** choices, and opens the panel on their next visit. The admin never sees which of the user's steps are done.

## Devices

A Device is one reading app on one piece of hardware, such as Kobo's own reader on a Kobo Libra, KOReader on that same Kobo, or the web browser on a Kindle. [Kobo Sync](./kobo-sync.md), [KOReader](./koreader.md), the [eReader Browser](./ereader-browser.md), and the [OPDS Catalog](./opds.md#sign-in-as-a-device) sign in as a Device. Any signed-in user can manage their own Devices from the user menu under **Devices**, with no role permission.

**Connect Device** on that page adds one in three steps. It asks what you read on, shows the setup steps for that one way of connecting, and then waits, switching to **Connected!** the first time the device reaches Shisho. A Device's page leads with its **Status**, which says whether it has connected and when it was last used, and its **Setup** steps. Its password, which books it reaches, and its Delivery Rules are under **Advanced**.

Set a device up from Shisho opened at the address your devices use, not `localhost` or another address only the server's own machine reaches. The setup steps warn when the page is open at such an address, because the URLs they give would not work on the device.

### Device Types

**Connect Device** picks the type from what you read on: **A Kobo** adds a Kobo Device, **KOReader** a KOReader Device (on any hardware, a Kobo included), **A Kindle's browser** a Kindle Device, **A reading app on a phone or tablet** an OPDS app Device, and **Another e-reader's browser** a Browser Device. The type decides which connections the Device can use, which file types it can receive, and the [Delivery Rules](#delivery-rules-of-a-device) it starts with:

| Type | Connections | Can receive | Starts with |
|------|-------------|-------------|-------------|
| **Kobo** | Kobo sync and the eReader browser | KePub, EPUB | Its preset: KePub as KePub, then EPUB as KePub, then CBZ as KePub, then AZW3 as KePub, then MOBI as KePub |
| **Kindle** | eReader browser | AZW3, MOBI | Its preset: AZW3 as AZW3, then MOBI as MOBI, then EPUB as AZW3, then KePub as AZW3, then CBZ as AZW3 |
| **KOReader** | The [KOReader plugin](./koreader.md#install-the-koreader-plugin), OPDS, and KOReader's progress sync | Any file type except M4B, since KOReader can't play audiobooks | Your default Delivery Rules |
| **OPDS app** | OPDS, for other reading apps such as Thorium, Panels, etc. | Any file type | Your default Delivery Rules |
| **Browser** | eReader browser, for any other e-reader's web browser | Any file type | Your default Delivery Rules |

The type alone decides how a Device connects, and the Device's page shows setup steps for each of its connections under **Setup**. A Device used through a connection its type does not have, such as a Browser Device's credential in a Kobo sync URL, gets a forbidden response; an OPDS client given a credential from a Device without OPDS, or KOReader's progress sync given one from a Device that isn't a KOReader Device, gets `401 Unauthorized`.

**Change Type** on a Device's page switches its type and keeps its credential and scope. Its connections change to the new type's, and its Delivery Rules go back to what the new type starts with, so a custom list is discarded.

### Device Password

Each Device has one password, its credential, that all its connections use. It is 16 lowercase letters with no `l`, `i`, or `o`, so it can be typed on an e-ink keyboard. Shisho accepts it only exactly as shown: no spaces, hyphens, or capital letters. The Device's page shows it as **Device password** under **Advanced**, with a copy button.

:::warning[Regenerating a Device Password]
**Regenerate** on a Device's page replaces its password immediately, and cannot be undone. Every Kobo sync URL and eReader browser bookmark with the old credential stops working, KOReader's progress sync, OPDS, and the KOReader plugin stop signing in with it, and any unexpired setup URL stops redirecting. Set up each connection again with the new password. Regenerate when a password may have leaked; delete the Device when the hardware is lost for good.
:::

Treat the Device password, and every URL that contains it, like your account password.

### Which Books

A Device's scope, under **Advanced > Which books** on its page, decides which books it reaches, through every connection:

| Scope | Books |
|-------|-------|
| **All Books** | Books in every library the Device's owner can access |
| **Library** | Books in one library the owner can access |
| **List** | Books on one list the owner owns or that is shared with them, in libraries the owner can access |

A list shared with you can hold books from libraries you cannot access; those books stay out of the scope. If the scope's library or list is deleted, or the list stops being shared with you, the Device reaches no books until you choose another scope. A scope change applies on the Device's next request, without changing anything on the device itself.

### Delivery Rules of a Device

Each Device has its own [Delivery Rules](./supported-formats.md#delivery-rules), which decide what its eReader browser and OPDS catalog offer and what [Kobo Sync](./kobo-sync.md#what-syncs) sends. Under **Advanced > Delivery Rules** on its page, choose one:

- **My default**: your default rules from **Preferences > Delivery Rules**, without any rule that delivers a file type the Device can't receive. A later edit to your default reaches the Device too. A Kindle following a default that has no rule delivering AZW3 or MOBI receives nothing, and its page says so.
- **Kobo preset** or **Kindle preset**: the type's preset from the table above, for Kobo and Kindle Devices. A Shisho release that changes a preset changes it for every Device following it.
- **Custom**: the Device's own list, which starts as a copy of what it followed and is saved with **Save**. It needs at least one rule, and a rule can only deliver a file type the Device can receive, so a Kindle can't have a rule that sends EPUB, though one can send EPUB as AZW3. Choosing **My default** or the preset again discards the list. While a Device follows your default, **Edit your default** under its rules opens **Preferences > Delivery Rules**.

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
