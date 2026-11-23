// The Bookmark the app made offline is on the server, with its note, placed
// when it was made rather than when it synced.
// It polls the server until the check passes or 25 s pass, since the sync
// can land late on a stalled CI simulator, and records "ok", or what's still
// wrong, in output.serverCheck for the flow's assertTrue.
function get(path) {
  const response = http.get(CONTROL + path);
  if (response.status !== 200) {
    throw new Error(
      "the harness could not read the server: " + response.status,
    );
  }
  return json(response.body);
}
let last;
function check() {
  const query = "?title=" + encodeURIComponent(TITLE);
  const bookmarks = get("/bookmarks" + query);
  const reading = get("/reading" + query);
  last = bookmarks;
  const made = bookmarks.filter((b) => b.note === NOTE);
  if (made.length !== 1) {
    throw new Error(
      "the server has " +
        made.length +
        " Bookmarks noted " +
        JSON.stringify(NOTE),
    );
  }
  if (made[0].created_at > reading.online_at) {
    throw new Error(
      "the Bookmark was made at " +
        made[0].created_at +
        ", after the server came back at " +
        reading.online_at +
        ": it was placed when it synced",
    );
  }
  if (made[0].created_at < reading.offline_at - 5) {
    throw new Error(
      "the Bookmark was made at " +
        made[0].created_at +
        ", before the server went offline at " +
        reading.offline_at,
    );
  }
}

const deadline = Date.now() + 25000;
for (;;) {
  try {
    check();
    output.serverCheck = "ok";
  } catch (e) {
    output.serverCheck = String(e.message || e);
  }
  if (output.serverCheck === "ok" || Date.now() > deadline) break;
  http.get(CONTROL + "/wait?ms=500");
}
console.log(JSON.stringify(last));
console.log("server check: " + output.serverCheck);
