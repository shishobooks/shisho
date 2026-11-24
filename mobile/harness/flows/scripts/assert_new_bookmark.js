// The server has more Bookmarks in the Book titled TITLE than
// note_bookmarks.js noted. It polls until it does or 25 s pass, since the
// sync can land late on a stalled CI simulator, and records "ok", or what's
// still wrong, in output.serverCheck for the flow's assertTrue.
let count;
const deadline = Date.now() + 25000;
for (;;) {
  const response = http.get(
    CONTROL + "/bookmarks?title=" + encodeURIComponent(TITLE),
  );
  if (response.status !== 200) {
    throw new Error(
      "the harness could not read the server: " + response.status,
    );
  }
  count = json(response.body).length;
  output.serverCheck =
    count > output.bookmarksBefore
      ? "ok"
      : "the server still has " + count + " Bookmarks";
  if (output.serverCheck === "ok" || Date.now() > deadline) break;
  http.get(CONTROL + "/wait?ms=500");
}
console.log("server check: " + output.serverCheck);
