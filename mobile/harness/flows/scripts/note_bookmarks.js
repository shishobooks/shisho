// Notes how many Bookmarks the server has in the Book titled TITLE, in
// output.bookmarksBefore, for assert_new_bookmark.js to compare with.
const response = http.get(
  CONTROL + "/bookmarks?title=" + encodeURIComponent(TITLE),
);
if (response.status !== 200) {
  throw new Error("the harness could not read the server: " + response.status);
}
output.bookmarksBefore = json(response.body).length;
console.log("bookmarks before: " + output.bookmarksBefore);
