# Lists

Lists are virtual collections of books from one or more libraries. Use them for a reading queue, favorites, rankings, or any collection that should not change the filesystem.

## Creating Lists and Using Templates

Create a list from **Lists**. Set its name, optional description, and mode:

- **Ordered** lists have a manual sequence.
- **Unordered** lists use an automatic sort.

Built-in templates provide a quick starting point:

- **To Be Read** creates an ordered reading queue.
- **Favorites** creates an unordered collection.

Templates set initial values only. You can edit the resulting list normally.

If creating or editing a list fails, an error notification appears and the dialog keeps your name, description, and ordering choice. Correct the problem and retry without re-entering them.

## Adding Books

Add books using either current workflow:

- On a book detail page, choose **Add to List**.
- In a library gallery, click **Select**, select one or more books, then choose **Add** and **Add to List**.

The list detail page does not currently provide an add-books picker. A list can contain books from different libraries, subject to each viewer's library access.

## Sorting and Reordering

Ordered lists default to manual order. Drag and drop is available only on page 1 when all list items fit on that page. If the list spans pages, the current interface does not provide cross-page drag reordering or a **Move to Position** command.

Unordered lists can sort by:

- **Recently added** or **Oldest added**
- **Title A–Z** or **Title Z–A**
- **Author A–Z** or **Author Z–A**

Save the chosen sort as the list default when you want other visits to open in that order.

## Converting List Modes

You can convert an existing list at any time:

- Switching to ordered assigns positions by when books were added, oldest first, and changes the sort to manual.
- Switching to unordered clears manual positions and changes the sort to recently added.

Conversion changes ordering behavior, not list membership.

## Sharing

The owner has full control, including deletion. Shares use these list-specific roles:

| Role | View | Add or Remove Books | Edit List | Manage Sharing |
|---|---:|---:|---:|---:|
| **Viewer** | Yes | No | No | No |
| **Editor** | Yes | Yes | No | No |
| **Manager** | Yes | Yes | Yes | Yes |

The API lets the owner or a **Manager** of a list manage its sharing whatever their role, without the global Users Read permission, and offers every signed-in user a directory of active usernames to share with. The app still asks for Users Read before it lists people to share with; it enables sharing for other roles in a following release. Lists identify other users by username only: a list's owner, the people it is shared with, and who added each book never show an email address or role. A list can be shared only with an active user.

Shared lists record who added each book.

List sharing only reaches users who sign in to Shisho, and it never grants library access. To send one book to someone without an account or without access to its library, use a [Share Link](./sharing.md) instead.

## Library Access Filtering

Each viewer sees only list books from libraries they can access. Hidden books remain members of the list and reappear if the viewer later gains access. List sharing never grants access to a library.

See [Users and Permissions](./users-and-permissions.md) for global roles and library grants, and [Browsing, Search, and Bulk Actions](./browsing-search-bulk-actions.md) for gallery selection and size controls.
