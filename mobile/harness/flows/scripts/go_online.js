// Brings the Shisho server back online through the harness's control
// endpoint, on the same address and data. Flows that need the server call it
// first, in case an earlier flow left it offline.
const response = http.post(CONTROL + "/online", { body: "" });
if (response.status !== 204) {
  throw new Error(
    "the harness could not bring the server online: " + response.status,
  );
}
