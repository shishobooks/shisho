// What the app changed offline reached the server as it was left: READ is
// the title of a Book marked read, TO_READ the titles on To Read in order
// (comma-separated), and LIST and LIST_BOOK a List made with a Book in it.
// It polls the server until the check passes or 25 s pass, since the sync
// can land late on a stalled CI simulator, and records "ok", or what's still
// wrong, in output.serverCheck for the flow's assertTrue.
let last;
function check() {
  const response = http.get(
    CONTROL + "/record?titles=" + encodeURIComponent(READ),
  );
  if (response.status !== 200) {
    throw new Error(
      "the harness could not read the server: " + response.status,
    );
  }
  last = json(response.body);
  if (last.outcomes[READ] !== "finished") {
    throw new Error(READ + " is " + (last.outcomes[READ] || "not started"));
  }
  const queued = last.to_read.join(",");
  if (queued !== TO_READ) {
    throw new Error("To Read is " + JSON.stringify(queued));
  }
  const books = last.lists[LIST];
  if (!books) throw new Error("the server has no List named " + LIST);
  if (books.indexOf(LIST_BOOK) < 0) {
    throw new Error(LIST + " holds " + JSON.stringify(books));
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
