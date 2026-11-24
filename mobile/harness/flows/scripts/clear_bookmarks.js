// Deletes the Bookmarks in the Book titled TITLE on the server, through the
// harness's control endpoint, for a flow that checks the ones it makes.
const response = http.post(
  CONTROL + "/bookmarks/clear?title=" + encodeURIComponent(TITLE),
  { body: "" },
);
if (response.status !== 204) {
  throw new Error(
    "the harness could not clear the Bookmarks: " + response.status,
  );
}
