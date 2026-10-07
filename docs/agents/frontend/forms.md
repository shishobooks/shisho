# Forms and unsaved changes protection

Read this before building or changing a form that creates or updates data, in a dialog or on a page. Which forms need protection is a review rule in `docs/agents/standards/frontend.md`; this is how to wire it.

## The shared shape

Every protected form keeps an `initialValues` snapshot taken when the form loads, computes `hasChanges` by comparing current state to it (`fast-deep-equal` for arrays and objects), and sets `initialValues` to the saved values after a successful save so `hasChanges` drops to false.

## Dialog forms: `FormDialog`

Use `FormDialog` (`@/components/ui/form-dialog`) in place of `Dialog`, passing `hasChanges`. It intercepts close attempts with `UnsavedChangesDialog` and adds a `beforeunload` handler. Close after a save with `requestClose` from `useFormDialogClose(open, onOpenChange, hasChanges)`, which waits for `hasChanges` to update; calling `onOpenChange(false)` directly right after `setInitialValues` hits the stale `hasChanges` and prompts. Initialize fields in an effect keyed on `open`. Examples: `CreateListDialog`, `MetadataEditDialog`.

A confirmation (delete, revoke) launched from a form dialog renders beside the `FormDialog`, not inside it (`RoleDialog`).

## Page forms: `useUnsavedChanges`

`useUnsavedChanges(hasChanges)` (`@/hooks/useUnsavedChanges`) blocks SPA navigation through react-router's `useBlocker` and adds `beforeunload`. It returns `showBlockerDialog`, `proceedNavigation`, and `cancelNavigation`; render `<UnsavedChangesDialog open={showBlockerDialog} onDiscard={proceedNavigation} onStay={cancelNavigation} />`. Initialize once per entity with an `isInitialized` flag reset when the entity id changes, so a background refetch does not overwrite the user's edits. Examples: `UserDetail`, `LibrarySettings`.

## Child editors with their own Save

A child with its own unsaved state takes `onHasChangesChange?: (hasChanges: boolean) => void` and reports through an effect; the parent ORs it into the `hasChanges` it hands `useUnsavedChanges` (`LibrarySettings` with its plugins tab).

## Tabs with inline editing

Intercept `onValueChange`: while `hasChanges`, store the target in `pendingTabChange` and show `UnsavedChangesDialog`; on discard, leave edit mode and navigate to the stored tab.
