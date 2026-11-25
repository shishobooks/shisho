// Takes the Books titled TITLES (comma-separated) back to unread for the
// harness user, off To Read, and deletes their Lists named LISTS, through
// the harness's control endpoint, so a flow that checks what it changes
// starts from the same place whatever an earlier flow left.
const response = http.post(
  CONTROL +
    "/record/reset?titles=" +
    encodeURIComponent(TITLES) +
    "&lists=" +
    encodeURIComponent(typeof LISTS === "undefined" ? "" : LISTS),
  { body: "" },
);
if (response.status !== 204) {
  throw new Error(
    "the harness could not reset the reading record: " + response.status,
  );
}
