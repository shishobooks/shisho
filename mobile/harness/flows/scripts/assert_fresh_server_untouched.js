// The fresh server the harness put behind the app's address got no journal
// requests: the changes queued for the server it replaced were never sent
// to it. The flow runs this once the new account's first sync has finished,
// and that account has nothing to send, so the count can't change and this
// checks once rather than polling.
const response = http.get(CONTROL + "/fresh-server/journal-requests");
if (response.status !== 200) {
  throw new Error(
    "the harness could not read the fresh server's log: " + response.status,
  );
}
const count = json(response.body).count;
output.serverCheck =
  count === 0 ? "ok" : "the fresh server got " + count + " journal requests";
console.log("server check: " + output.serverCheck);
