// Puts a fresh server behind the app's address through the harness's control
// endpoint: an empty database, as a server set up again from scratch, with
// the harness account made again as its first admin. go_online.js brings
// the seeded server back.
const response = http.post(CONTROL + "/fresh-server", { body: "" });
if (response.status !== 204) {
  throw new Error(
    "the harness could not start a fresh server: " + response.status,
  );
}
