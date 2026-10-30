# Lists

Lists are virtual collections of books from one or more libraries. Use them for favorites, rankings, or any collection that should not change the filesystem. For your own queue of books to read next, use [To Read](./reading-tracking.md#to-read), which is pinned at the top of **Lists** and of **Add to list**. It isn't a list, so nothing on this page applies to it.

## Creating Lists

Create a list from **Lists**. Set its name and optional description, and choose whether it is ordered:

- With **Ordered list** selected, the list has a manual sequence.
- With **Ordered list** clear, the list uses an automatic sort. This page calls these unordered lists.

The **Favorites** template creates an unordered collection. It sets initial values only, and you can edit the resulting list normally.

If creating or editing a list fails, an error notification appears and the dialog keeps your name, description, and ordering choice. Correct the problem and retry without re-entering them.

## Adding Books

Add books in either of these ways:

- On a book's page, open the **⋯** menu and choose **Add to list**.
- In a library gallery, click **Select**, select one or more books, then choose **Add** and pick a list.

The list detail page has no add-books picker. A list can contain books from different libraries, subject to each viewer's library access.

## Sorting and Reordering

Ordered lists default to manual order. Drag and drop is available only on page 1 when all list items fit on that page. If the list spans pages, you cannot drag books between pages or move a book to a numbered position.

Unordered lists can sort by:

- **Recently Added** or **Oldest Added**
- **Title (A-Z)** or **Title (Z-A)**
- **Author (A-Z)** or **Author (Z-A)**
- **Rating (high to low)** or **Rating (low to high)**, and **Recently Finished** or **Earliest Finished**, by each viewer's own [reading](./reading-tracking.md#finding-books-by-status-and-rating)

Any list can also be filtered by reading status and rating with **Filter**. A filtered ordered list can't be reordered until the filters are cleared.

A chosen sort is kept in the page URL, so it survives a reload and can be bookmarked or shared. Save it as the list default when you want other visits to open in that order.

## Copying to To Read

**Copy to To Read** on a list's page adds its books to the end of your [To Read](./reading-tracking.md#to-read) queue, in the list's order: its manual order for an ordered list, its default sort otherwise. Books already on To Read, books you're reading, and books in libraries you can't access are skipped. It needs only view access to the list, including a list shared with you as **Viewer**, and it changes your own queue, not the list.

## Converting List Modes

You can convert an existing list at any time:

- Switching to ordered assigns positions by when books were added, oldest first, and changes the sort to manual.
- Switching to unordered clears manual positions and changes the sort to **Recently Added**.

Conversion changes ordering behavior, not list membership.

## Sharing

The owner has full control, including deletion. Shares use these list-specific roles:

| Role | View | Add or Remove Books | Edit List | Manage Sharing |
|---|---:|---:|---:|---:|
| **Viewer** | Yes | No | No | No |
| **Editor** | Yes | Yes | No | No |
| **Manager** | Yes | Yes | Yes | Yes |

The owner or a **Manager** of a list can manage its sharing whatever their role, without the global Users Read permission. The **Share** dialog lets them pick from every active username. In the [demo](./demo.md), the dialog shows a notice instead of the user picker. Lists identify other users by username only: a list's owner, the people it is shared with, and who added each book never show an email address or role. A list can be shared only with an active user.

Shared lists record who added each book.

List sharing only reaches users who sign in to Shisho, and it never grants library access. To send one book to someone without an account or without access to its library, use a [Share Link](./sharing.md) instead.

## Permissions

Lists follow their own sharing roles rather than role permissions. Any signed-in user can create a list, and the owner or a **Manager** can rename it. Adding books to a list you own, or to one shared with you as **Editor** or **Manager**, does not need Books Write.

The one exception is Books Read. A list's books are book data, so seeing them requires Books Read, even for the list's owner. Without it, a list still opens but its books are hidden.

## Library Access Filtering

Each viewer sees only list books from libraries they can access. Hidden books remain members of the list and reappear if the viewer later gains access. List sharing never grants access to a library.

## Reading Apps

Lists you own or that are shared with you also appear in the [OPDS catalog](./opds.md#catalog-structure), so a reading app can browse and download a list's books. The same library access filtering applies there.

See [Users and Permissions](./users-and-permissions.md) for global roles and library grants, and [Browsing, Search, and Bulk Actions](./browsing-search-bulk-actions.md) for gallery selection and size controls.
