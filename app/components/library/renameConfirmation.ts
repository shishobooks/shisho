/**
 * The toast a rename dialog shows once its save succeeds. A genre, tag, or
 * publisher renamed onto an existing name is merged into that one, which the
 * server answers with the other resource's id.
 */
export const renameConfirmation = (
  oldName: string,
  newName: string,
  entityId: number | undefined,
  saved: unknown,
): string => {
  const name = newName.trim();
  if (name === oldName.trim()) return "Changes saved";
  const savedId =
    saved !== null && typeof saved === "object" && "id" in saved
      ? saved.id
      : undefined;
  if (
    entityId !== undefined &&
    typeof savedId === "number" &&
    savedId !== entityId
  ) {
    return `Merged into "${name}"`;
  }
  return `Renamed to "${name}"`;
};
