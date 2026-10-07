# Demo Mode in the frontend

Read this before changing anything that reports a write failure, hides a control in Demo Mode, or stores preferences. Backend behavior is in `docs/agents/backend/demo-mode.md`; the backend remains the write boundary.

## The flag

`useAuth()` exposes `demoMode`, sourced from the unauthenticated `GET /auth/status`. Branch on this flag, never on hostname or username.

## Rejections are reported once, by `checkStatus`

On a `403` with code `demo_mode`, `ShishoAPI.checkStatus` shows the toast `This action is unavailable in the demo.` (id `demo-mode`, so concurrent rejections collapse into one) and then rejects. Callers still receive the rejection, so dialogs stay open and keep their drafts, but they must not report it again:

- `toastRequestError` stays silent when `isDemoModeError(error)`.
- Inline error UI (the `BookEditDialog` and `FileEditDialog` banners, the `MetadataEditDialog` and `PublisherEditDialog` server errors) skips a rejection where `isDemoModeError(error)` and renders every other error as before.

Testing: a caller that toasts goes through the real `API` with a `demo_mode` 403 and a real `<Toaster />`, and the test counts the visible messages; a mocked rejection never reaches `checkStatus`. A dialog that only renders an injected `onSave` rejection inline can be tested with a rejected `ShishoAPIError` directly.

## What Demo Mode hides

On top of role-based hiding, Demo Mode hides only: the file download button and format popover, the supplement download button, the bulk download action, the admin gear and mobile drawer admin entries, the Security settings route and its `UserMenu` entry. Anything else the role may use stays visible and relies on the backend rejection plus the toast. `useCan` and `can` reflect role permissions only; they know nothing about Demo Mode.

Routes the server leaves unregistered in Demo Mode (plugins, `/users/directory`) have query hooks that are off there (`usePluginRouteEnabled`, `useUserDirectory`); `ShareListDialog` shows a notice instead of the user picker.

## Preferences stay in the browser

User settings use the `shisho-demo-user-settings` local storage key; per-library settings use `shisho-demo-library-settings-{libraryId}`. The query hooks fetch server defaults first, merge stored values over them, and write Demo Mode mutations to local storage and the TanStack Query cache without sending a request.
