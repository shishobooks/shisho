# Users and Permissions

Shisho combines role permissions with a per-user library access list. Both checks must allow an action.

Open **Settings > Users** to manage accounts and roles. Access to this page and its actions depends on the current user's Users permissions.

## Built-In Roles

Shisho creates three system roles with these default permissions:

| Role | Default Permissions |
|------|---------------------|
| **admin** | Read and Write for Libraries, Books, Series, People, Users, Jobs, and Config |
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

Write permissions permit the create, edit, or delete operations associated with that resource. Read access does not imply Write access.

## Custom Roles

On **Settings > Users**, select **Add Role** to create a named role with a custom permission matrix. Select an existing role in the **Roles** section to edit it. Non-system roles can also be renamed or deleted, but you must reassign every user before deleting an assigned role.

A user has one role. Assign the narrowest permissions needed for that person's work.

## Library Access

Role permissions and library access intersect:

1. The assigned role must grant the required resource operation.
2. The user must also have access to the library containing the requested data.

For example, a user with Books Write but access to only one library can edit books only in that library. Library access does not add permissions that the role lacks.

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

## Sessions

Shisho uses one server-wide session duration with a fixed default of 30 days. There is no per-user duration or remember-me setting. Administrators can change the global `SESSION_DURATION_DAYS` setting; existing tokens remain governed by how they were issued. See [Configuration](./configuration.md#authentication).

The [OPDS Catalog](./opds.md) also supports HTTP Basic Auth for clients that do not use Shisho's browser session. OPDS catalog contents follow the user's library access.
