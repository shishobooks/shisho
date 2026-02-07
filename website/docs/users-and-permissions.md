---
sidebar_position: 11
---

# Users and Permissions

Shisho has a role-based access control system that lets you manage who can view and edit your libraries.

## Roles

There are three built-in roles:

| Role | Description |
|------|-------------|
| **Admin** | Full read and write access to everything, including server configuration, user management, and job monitoring |
| **Editor** | Read and write access to libraries, books, series, and people. Cannot manage users, server config, or jobs |
| **Viewer** | Read-only access to libraries, books, series, and people |

The built-in roles cannot be renamed or deleted.

### Permissions Reference

Each role is defined by a set of resource/operation pairs:

| Resource | Admin | Editor | Viewer |
|----------|-------|--------|--------|
| Libraries | Read, Write | Read, Write | Read |
| Books | Read, Write | Read, Write | Read |
| Series | Read, Write | Read, Write | Read |
| People | Read, Write | Read, Write | Read |
| Users | Read, Write | | |
| Jobs | Read, Write | | |
| Config | Read, Write | | |

## Library Access

In addition to role-based permissions, each user has a **library access list** that controls which libraries they can see. This works independently of their role:

- **All libraries**: The user can access every library, including ones created in the future
- **Specific libraries**: The user can only access the libraries you select

For example, you might give a family member the Viewer role with access only to your shared fiction library, while keeping your reference library private.

## Managing Users

### Creating a User

1. Go to **Admin > Users**
2. Click **Add User**
3. Set the username, optional email, password, and role
4. (Optional) Enable **Require password reset on first login** to force the user to choose a new password after they sign in
5. Choose which libraries the user can access
6. Save

Usernames and email addresses must be unique (case-insensitive).

### Deactivating a User

You can deactivate a user to revoke their access without deleting their account. Deactivated users cannot log in.

### Changing Passwords

Admins can reset any user's password from the user management page. When resetting another user's password, admins can also enable **Require user to reset password on next login**.

Users can change their own password from their user security settings. If a user's account is marked as requiring a password reset, Shisho redirects them to security settings until they set a new password, and that forced-reset form only asks for the new password.

## Authentication

Shisho uses JWT-based authentication. Sessions last 7 days before requiring a new login.

[OPDS](./opds) endpoints also support HTTP Basic Auth for compatibility with e-reader apps that don't handle cookie-based authentication.
