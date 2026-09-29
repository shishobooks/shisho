# Users and Permissions

Shisho combines role permissions with a per-user library access list. Both checks must allow an action.

Open **Settings > Users** to manage accounts and roles. Access to this page and its actions depends on the current user's Users permissions.

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

- A list's books are book data, so reading them requires Books Read, even for the list's owner. Creating a list and renaming it need no role permission. Any owner or Manager of a list can manage its sharing without Users Read. See [Lists](./lists.md#sharing).
- Global search returns Series results only to roles with Series Read and People results only to roles with People Read. Books results need Books Read, like the rest of search.
- The review criteria can be read with Books Read, for the review panel on book pages, or with Config Read, for **Settings > Review Criteria**. Changing them requires Config Write.
- The plugin manager under **Settings > Plugins** can be viewed with Config Read, though its plugin order also needs Books Read. Installing, configuring, and removing plugins requires Config Write. See [Plugins](./plugins/overview.md).

Share Links are managed with Shares permissions. Granting Shares Write lets a user expose books to people outside your server, so review [Sharing](./sharing.md) before adding it to a role. Shares permissions and library access are all the server checks; they do not depend on Books Read. The **Share** entry lives on the book page, though, and opening a book page needs Books Read.

The server log under **Settings > Logs** requires Config Read. Live log updates follow the same rule, so users without Config Read never receive server log lines.

## What a Read-Only Role Sees

A user whose role has Read but not Write for a resource does not see the controls that would change it. The server still rejects any unauthorized request; hiding the controls only keeps the interface honest about what the user can do.

With the built-in **viewer** role, or any role without Books Write, Series Write, or People Write:

- Book tiles in the gallery show no actions menu (rescan, identify, delete). **Add to list** stays available.
- The book page shows no edit, rescan, identify, merge, or delete actions, no per-file actions menu, and no file selection for moving files. The review panel shows the current state without a toggle. Reading, listening, downloading, and **Add to list** remain.
- The file page shows no **Edit** or **Delete** buttons, and the chapters tab offers no chapter editing.
- Series, person, genre, tag, and publisher pages show no **Edit**, **Merge**, or **Delete** buttons.
- Selection mode still works for adding books to lists, and for downloads when the role has the [bulk download permissions](./browsing-search-bulk-actions.md#bulk-download-permissions). **Merge**, **Delete**, and the review actions are hidden.

Each control follows the resource its request needs, not the page it appears on. A role with Books Write but not Series Write sees the edit controls on book, genre, tag, and publisher pages and not on series pages.

Lists are independent of these role permissions. Any signed-in user can create a personal list and add books to a list they own or that has been shared with them with editor or manager access, even without Books Write. See [Lists](./lists.md).

## What a Role Sees Without Read Permissions

Shisho hides pages and links that a role cannot read, rather than showing an error. Opening such a page by its address shows **Access Denied**.

- **Books Read.** Library pages, book and file pages, and search are unavailable, and the library picker is hidden. The home page opens **Lists**. A list still opens, but its books are hidden, since they are book data.
- **Series Read.** The library navigation has no **Series** entry, series names on book pages are plain text, and search shows no series. The edit dialogs suggest no existing series, but you can still type a name.
- **People Read.** The library navigation has no **People** entry, author and narrator names are plain text, and search shows no people. The edit dialogs suggest no existing authors or narrators, but you can still type a name.
- **Libraries Read.** Library settings and **Settings > Libraries** are unavailable. Every signed-in user still gets the name and display settings (cover aspect ratio, download format, and whether files are organized) of each library in their [library access](#library-access) list, never its folders. The library picker, breadcrumbs, cover shapes, and the merge and move dialogs use these. A role with Users Write can list libraries without Libraries Read, so it can assign library access when creating or editing a user.
- **Users Read.** **Settings > Users** is unavailable. List owners and Managers still pick whom to share a list with from a directory of usernames.
- **Jobs Read.** **Settings > Jobs** is unavailable, and so are the library rescan button and **Recompute now** on **Settings > Review Criteria**, which also need Jobs Write.
- **Config Read.** The server, review criteria, sharing, plugin, cache, and log settings pages are unavailable. With Config Read but not Config Write, the review criteria are shown without a **Save** button.

The **Global Settings** button opens the first settings page the role can view, and is hidden when there is none. The home page opens the first library the user can access when the role has Books Read. With no libraries yet, it opens **Settings > Libraries** for a role with Libraries Read and **Lists** otherwise.

## Custom Roles

On **Settings > Users**, select **Add Role** to create a named role with a custom permission matrix. Select an existing role in the **Roles** section to edit it. Non-system roles can also be renamed or deleted, but you must reassign every user before deleting an assigned role.

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

1. Open **Settings > Users**.
2. Select **Add User**.
3. Enter the account information.
4. Optionally select **Require password reset on first login**.
5. Select a role.
6. Configure **Library Access**.
7. Select **Create User**.

Select a username on the **Users** list to edit its username, optional email, role, and library access.

## Password Changes and Forced Resets

Any signed-in user can open the user menu, select **Security**, and use **Change Password**. A normal self-service change requires the current password.

A user with Users Write permission can open another account under **Settings > Users**, select **Reset Password**, and optionally select **Require user to reset password on next login**. A user marked for forced reset must choose a new password before continuing in Shisho.

## Deactivate Users

:::warning[Verify the Account Before Deactivation]
Deactivation immediately prevents that user from logging in. It does not delete the account, but Shisho currently has no reactivation control. You cannot deactivate your own account.
:::

A user with Users Write permission can select another active account and choose **Deactivate User**. The account and its historical records remain stored.

Deactivation also stops every [Share Link](./sharing.md#when-a-link-stops-working) the user created. The links stay listed in each book's Share dialog, where anyone with Shares Write can revoke or delete them. It disables the user's API keys too, so their [Kobo Sync](./kobo-sync.md) and [eReader Browser](./ereader-browser.md) URLs stop working, the same as when their role loses Books Read.

## Sessions

Shisho uses one server-wide session duration with a fixed default of 30 days. There is no per-user duration or remember-me setting. Administrators can change the global `SESSION_DURATION_DAYS` setting; existing tokens remain governed by how they were issued. See [Configuration](./configuration.md#authentication).

The [OPDS Catalog](./opds.md) also supports HTTP Basic Auth for clients that do not use Shisho's browser session. OPDS catalog contents follow the user's library access.
